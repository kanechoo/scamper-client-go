# Socket 通信设计

## 1. 概述

Socket 模式通过 scamper daemon 的 **control socket**（Unix Domain Socket）做高并发测量。

```
Go Client ── Unix Domain Socket ──▶ scamper daemon
   │  attach format json
   │  trace <args> <target>\n
   │◀─ OK / MORE / DATA <len> [id-N] / ERR
```

SDK 只处理 **JSON** 格式；不实现 WARTS。

## 2. Control socket 协议（基于 `man scamper` CONTROL SOCKET / ATTACH）

### 2.1 进入 attach 模式

连接后发送一行：

```
attach format json\n
```

daemon 回 `OK`（中途可能夹杂 `MORE`）。收到 `OK` 视为 attach 成功。

### 2.2 提交命令

attach 模式下，直接发送 scamper 命令（与 `-I` 的命令串一致）：

```
trace -P udp -f 9 -m 16 -w 1 -q 1 -S 192.168.6.254 38.60.89.121\n
```

### 2.3 应答帧

daemon 逐行应答，SDK 需要按帧解析：

| 行 | 含义 | SDK 行为 |
|----|------|----------|
| `OK` | attach 成功 / 无 id 的确认 | 握手用；运行期忽略 |
| `OK id-N` | 命令被接受，分配了 id N | 把 N 绑定到 `acceptQueue` 队首任务 |
| `MORE` | 有并行容量 | 忽略（前向兼容） |
| `DATA <len> [id-N]` | 后随 `len` 字节数据（JSON） | 读满 len 字节；有 id 则解析并投递 |
| `ERR <msg>` | 命令被拒绝 | 给 `acceptQueue` 队首任务填 `CommandError` |

- `len` 是字符数；JSON 输出对非 ASCII 做转义，实际按字节读取即可（SDK 用 `io.ReadFull`）。
- 无 `id-` 的 `DATA`（如 `cycle-start` / `cycle-stop`）直接丢弃。
- 只解析 `{"type":"trace",...}`；其余类型忽略。

### 2.4 取消

超时/取消时发送：

```
halt <id>\n
```

## 3. 单连接、单读、单写（硬性要求）

**禁止**多个 goroutine 直接读写同一个 socket。结构如下：

```
                 ┌──────────────── Session ────────────────┐
Submit ──job──▶ outCh ──▶ [writer goroutine] ──▶ conn.Write
                                                       │
conn ──▶ [reader goroutine] ──▶ parse line ──▶ id→job ──▶ job.result
```

- **writer goroutine**：唯一写者。串行写出命令与 `halt`；写前登记到 `acceptQueue`。
- **reader goroutine**：唯一读者。逐行读，解析帧，按 id 投递结果。
- Submit 只与 channel 交互，不直接碰 `net.Conn`。

这天然解决"500 个 goroutine 争抢 socket"的问题，也是背压的落点（outCh 满则 Submit 阻塞）。

## 4. 任务/结果映射

见 `concurrency-design.md` §4。要点：

- 单 writer 保证命令顺序 = daemon 接受顺序，因此 `OK id-N` 与 FIFO `acceptQueue` 一一对应。
- `DATA ... id-N` 按 `id→job` 精确投递，结果可乱序到达。
- 连接断开时，`acceptQueue` 与 `id→job` 全部在途任务置为可重试错误（`ErrDaemonUnavailable`）。

## 5. 连接生命周期

```
NewClient ──▶ Dial ──▶ attach(json) ──▶ ready
                                        │
                         ┌──────────────┴──────────────┐
                         ▼                             ▼
                  正常 submit/read              读写错误/EOF
                         │                             │
                         └──────── failInflight ───────┘
                                        │
                                   backoff
                                        │
                                   reconnect ──▶ 重新 attach ──▶ ready
```

- SDK 在 `NewSocketClient` 时建立首个连接以 **fail-fast**；失败返回 `ErrDaemonUnavailable`。
- 之后由后台 supervisor 维持连接：断开 → 指数退避重连（可配置上下限与抖动）。
- `Close()` 取消 supervisor、关闭连接、唤醒所有等待者。

## 6. 断线重连与 daemon 重启恢复

- **在途任务**：断线瞬间不可完成，统一返回可重试错误；由调用方或 `WithBatchRetry` 决定重跑。
- **已提交未返回**：同上，不做"断点续传"（scamper 不持久化任务结果到 control 连接）。
- **daemon 重启**：socket 可能被删除/替换。重连时重新 `Dial`；若 socket 尚未就绪，保持退避重试直到成功。
- **socket 文件残留**：SDK 不负责清理残留 socket（属于 daemon 管理范围）；连接失败会如实返回错误。
- **generation 计数**：每次成功重连递增 generation，防止旧连接的 reader 回调污染新连接的映射表。

## 7. 连接健康检查

- 被动检测：任何 `Read`/`Write` 错误或 `EOF` 即判连接不可用。
- 可选主动探测：`WithHealthCheck(interval)`，定期发送轻量命令（如 `get window`）验证往返；失败触发重连。
- 默认关闭主动探测，仅依赖读写错误，避免额外流量。

## 8. 配置项

```go
scamper.NewSocketClient(
    scamper.WithSocketPath("/var/run/scamper/scamper.sock"), // 默认
    scamper.WithDialTimeout(5*time.Second),
    scamper.WithReconnectPolicy(scamper.ReconnectPolicy{
        MinBackoff: time.Second,
        MaxBackoff: 30*time.Second,
        Multiplier: 2,
        Jitter:     true,
    }),
    scamper.WithMaxInflight(64),
    scamper.WithHealthCheck(0), // 0=关闭
)
```

- 默认 socket 路径常量：`DefaultSocketPath = "/var/run/scamper/scamper.sock"`（用户可用 `WithSocketPath` 覆盖；daemon 实际路径由用户 `scamper -U` 决定）。
- SDK **不启动** daemon、**不修改** daemon `-w/-p`、**不管理**权限。

## 9. 数据格式

- 只走 `attach format json`；`DATA` 内容是 JSON 文本。
- `internal/parser` 将 JSON 映射为 `TraceResult`：

```
{"type":"trace","method":"udp","dst":"38.60.89.121","stop_reason":"HOPLIMIT",
 "hops":[{"addr":"59.43.250.50","probe_ttl":11,"rtt":15.309,...}, ...]}
        │
        ▼
TraceResult{ Target, Address, Method, StopReason, Hops:[{TTL,Address,RTT,...}], Duration, Metadata }
```

- SDK 忽略 caller 不关心的字段，但把原始关键字段放入 `Metadata`（如 `stop_data`、`probe_count`）。
- 不实现 WARTS 解析；未来如需其他格式，通过 `Format` 扩展点接入，当前仅 json。

## 10. 错误分类

| 场景 | 错误 |
|------|------|
| dial 失败 / 断线 | `ErrDaemonUnavailable`（可重试） |
| attach 被 `ERR` | `CommandError`（不可重试） |
| trace 命令 `ERR` | `CommandError`（不可重试） |
| `DATA` 无法解析 | `ErrParse`（可重试） |
| 帧长度非法 / 读不满 | `ErrProtocol`（可重试） |
| ctx 超时 | `TimeoutError`（可重试） |
| Client 已 Close | `ClientClosedError` |

## 11. 时序示例（并发 N 条）

```
Client                        Session(writer)         daemon            Session(reader)
  │ submit A,B,C ─────────────────▶ │                    │                     │
  │                                  │ trace A ─────────▶ │                     │
  │                                  │ trace B ─────────▶ │                     │
  │                                  │ trace C ─────────▶ │                     │
  │                                  │◀──── OK id-1 ──────│                     │
  │                                  │◀──── OK id-2 ──────│                     │
  │                                  │◀──── OK id-3 ──────│                     │
  │                                  │                    │─ DATA ... id-2 ────▶│─▶ result B
  │                                  │                    │─ DATA ... id-1 ────▶│─▶ result A
  │                                  │                    │─ DATA ... id-3 ────▶│─▶ result C
  │ ◀──────────── 结果按输入索引回填 ─┴────────────────────┴─────────────────────┘
```

## 12. 测试要点

- **协议单测**：用 fake unix socket 服务端模拟 `OK/MORE/DATA/ERR`，覆盖乱序、无 id、超大 len、半包。
- **重连**：服务端主动断开后，新任务仍能成功。
- **halt**：ctx 超时后，fake 端能收到 `halt <id>`。
- **真实集成测试**：需要环境提供 scamper daemon 与 socket 路径，通过 `SCAMPER_SOCKET` 环境变量启用；否则 `t.Skip`。
