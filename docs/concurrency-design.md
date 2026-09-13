# 并发模型设计

## 1. 适用范围

并发调度**只在 Socket 模式有意义**。

- Socket：一个连接即可多路复用成百上千条 traceroute，SDK 内部提供 worker pool、有界队列、背压、id 映射、超时、重试。
- Legacy：每条 traceroute 需要一个 `scamper` 进程；`TraceBatch` 仍可用，但内部是一个**受限进程池**（默认并发远小于 socket，例如 8），文档需明确进程开销。

不要求用户必须使用 SDK 内部并发：`Trace` 单目标接口始终可用，让高级用户自行控制 goroutine。

## 2. 三个层次

```
┌────────────────────────────── TraceBatch（高级 API）──────────────────────────────┐
│  调度层 Scheduler：worker pool + bounded queue + backpressure + retry + progress   │
├────────────────────────────────────────────────────────────────────────────────────┤
│  传输层 Session：单连接、单 writer goroutine、单 reader goroutine、id 映射           │
├────────────────────────────────────────────────────────────────────────────────────┤
│  Trace（基础 API）：调用方自己管理 goroutine，SDK 只保证单次提交/解析正确            │
└────────────────────────────────────────────────────────────────────────────────────┘
```

- 用户可只用最底层 `Trace`（socket 模式仍复用同一个 Session，线程安全）。
- `TraceBatch` 是 `Trace` 之上的调度器糖。

## 3. 模型与背压

### 3.1 组成

- **jobs 有界队列**：`chan job`，容量 = `max(queueSize, concurrency)`。
- **worker pool**：`concurrency` 个 goroutine，从队列取目标并调用 `Session.Submit`。
- **inflight 信号量**：容量 = `maxInflight`，限制同时在途的任务数（防止把 daemon 窗口挤爆）。
- **结果收集**：每个 job 带自己的 `*TraceResult` 槽位，保证输出顺序与输入一致。

### 3.2 背压来源

1. jobs 队列满 → 提交者阻塞（或 `WithDropOnBackpressure` 时快速失败）。
2. inflight 信号量满 → worker 阻塞。
3. Session 写通道满 → worker 阻塞。

三者都施加上界，内存不会随目标数线性增长。

### 3.3 伪代码

```go
func (s *scheduler) run(ctx context.Context, targets []string) []TraceResult {
    results := make([]TraceResult, len(targets))
    jobs := make(chan int, s.queueSize)

    var wg sync.WaitGroup
    for w := 0; w < s.concurrency; w++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for i := range jobs {
                s.inflight <- struct{}{}
                results[i] = s.session.trace(ctx, targets[i], s.traceOpts)
                <-s.inflight
                s.report(i)
            }
        }()
    }

    for i := range targets {
        select {
        case jobs <- i:
        case <-ctx.Done():
            close(jobs); wg.Wait(); return results
        }
    }
    close(jobs)
    wg.Wait()
    return results
}
```

> 真正的实现里 `Session.trace` 会把任务交给单 writer goroutine，reader 按 id 回填结果，详见 `socket-design.md`。

## 4. task id 与 result 映射

scamper attach 协议里：

- 命令发出后，daemon 回 `OK id-N`；
- 完成时回 `DATA <len> id-N` + JSON。

SDK 的映射策略：

1. 每个 job 生成一个单调递增的本地序号 `seq`。
2. 单 writer 顺序写出命令，并把 `seq → job` 压入 FIFO `acceptQueue`。
3. reader 收到 `OK id-N`：弹出 `acceptQueue` 队首，记录 `id→job`。
4. reader 收到 `DATA ... id-N`：查 `id→job`，解析 JSON，投递结果，删除映射。
5. reader 收到 `ERR`：弹出 `acceptQueue` 队首，填 `CommandError`。
6. 连接断开：`acceptQueue` 与 `id→job` 中所有在途 job 统一置为可重试错误。

由于**只有一个 writer**且命令顺序与 daemon 接受顺序一致，`OK` 与 `acceptQueue` 严格按序对应，无需在命令里自定义 id。

## 5. 超时

三层超时：

| 层 | 来源 | 行为 |
|----|------|------|
| 整批 | `TraceBatch` 的 `ctx` | 取消后停止提交，在途任务收到 `halt` |
| 单目标 | `WithTimeout` / `WithBatchTimeout` | 超时对该任务发 `halt <id>`，返回 `TimeoutError` |
| 连接 | `WithDialTimeout` | 建连超时，返回 `ErrDaemonUnavailable` |

超时不是"丢弃"：SDK 尽力向 daemon 发 `halt <id>`，避免占着 daemon 窗口。

## 6. 重试

- 默认 **不重试**（`WithRetry(0)`），保证语义可预期。
- 可重试错误：`ErrDaemonUnavailable`、`TimeoutError`、`ErrProtocol`。
- 不可重试：`CommandError`（参数错误，重试无意义）、`ErrInvalidConfig`。
- 重试有独立退避（指数 + 抖动），且受单目标 `ctx` 约束。
- `WithBatchRetry(n)` 对每个目标独立重试，不影响其它目标。

## 7. 进度与顺序

- `WithProgress(func(done, total int))` 在结果落地时回调；回调需线程安全（可能并发触发）。
- 返回切片与输入 `targets` **顺序一致**（内部按索引回填）。
- `WithStopOnError(true)` 时，首个不可重试错误触发取消；默认 false，保证部分成功可用。

## 8. 与 daemon 窗口的关系（重要）

- scmaper daemon 的 `-w`（active task 窗口）与 `-p`（PPS）由**用户**配置，SDK 不管理。
- SDK 的 `WithConcurrency` / `WithMaxInflight` 只是本地提交上限；若远大于 daemon 窗口，任务会在 daemon 侧排队，不会报错但也不会更快。
- 文档建议：`WithConcurrency` ≈ daemon 窗口；过高并发反而可能因本地/网络丢包降低测量质量。
- 去重：scamper 对相同任务签名会合并，SDK 不做去重，调用方需注意批量中出现等价目标的场景。

## 9. 指标（可选）

`WithMetrics(sink MetricsSink)` 暴露：提交数、完成数、失败数、平均时延、重连次数、队列深度。默认 no-op。

## 10. 测试要点

- **并发正确性**：500 个目标、并发 64，断言 500 个结果无丢失、无重复、顺序正确。
- **背压**：无限流式提交时内存有界。
- **超时**：注入慢目标，验证 `halt` 被发送、结果带 `TimeoutError`。
- **重连**：运行中关闭连接，断言在途任务收到可重试错误、连接恢复后新任务成功。
- **竞态**：`go test -race` 全绿。
