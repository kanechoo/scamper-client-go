# Public API 设计

## 1. 包划分与命名

| 包 | import path | 职责 | 稳定性 |
|----|-------------|------|--------|
| `scamper` | `github.com/kanechoo/scamper-client-go` | 接口、公共类型、工厂、错误 | 稳定（v1 API 面） |
| `legacy` | `.../legacy` | `exec scamper` 实现 | 稳定但用户一般不用 |
| `socket` | `.../socket` | daemon control socket 实现 | 稳定但用户一般不用 |
| `internal/parser` | 内部 | JSON → struct | 不对外 |

用户推荐只 import root 包：

```go
import scamper "github.com/kanechoo/scamper-client-go"
```

根包通过类型别名/工厂导出两种实现，避免用户关心子包：

```go
c, err := scamper.NewSocketClient(scamper.WithSocketPath("/var/run/scamper/scamper.sock"))
c, err := scamper.NewLegacyClient(scamper.WithBinary("/usr/local/bin/scamper"))
```

> 子包 `socket`/`legacy` 仍然导出，供需要直接控制具体实现的高级用户使用。

## 2. 核心接口

```go
package scamper

type Client interface {
    // Trace 对单个目标做一次 traceroute。
    // options 覆盖 Client 默认配置（例如 TraceConfig、超时、重试）。
    Trace(ctx context.Context, target string, options ...TraceOption) (*TraceResult, error)

    // TraceBatch 并发测量多个目标。
    // socket 模式：内部 worker pool + 有界队列 + 背压 + id 映射。
    // legacy 模式：受限的进程池（每个目标一个 scamper 进程）。
    // 返回切片与 targets 等长且顺序一致；单目标失败不影响其它目标。
    TraceBatch(ctx context.Context, targets []string, options ...BatchOption) ([]TraceResult, error)

    // Close 释放连接/进程等资源。Close 之后不应再调用 Trace。
    Close() error
}
```

`TraceBatch` 返回 `[]TraceResult`（值切片，保证顺序与 targets 对应）；每个元素自带 `Error`，整批只有在初始化/参数级失败时才返回非 nil error。

## 3. functional options

分三层，职责清晰：

### 3.1 Client 构造选项（`ClientOption`）

```go
func NewSocketClient(opts ...ClientOption) (Client, error)
func NewLegacyClient(opts ...ClientOption) (Client, error)

func WithSocketPath(path string) ClientOption   // socket 模式；默认 /var/run/scamper/scamper.sock
func WithBinary(path string) ClientOption       // legacy 模式；默认在 PATH 中找 scamper
func WithDialTimeout(d time.Duration) ClientOption
func WithReconnectPolicy(p ReconnectPolicy) ClientOption
func WithDefaultTraceConfig(c *TraceConfig) ClientOption
func WithLogger(l *slog.Logger) ClientOption
func WithMaxInflight(n int) ClientOption        // socket 模式在途上限
```

### 3.2 单次调用选项（`TraceOption`）

```go
func WithTraceConfig(c *TraceConfig) TraceOption
func WithTimeout(d time.Duration) TraceOption   // 覆盖默认单任务超时
func WithRetry(n int) TraceOption               // 单目标重试次数（仅可重试错误）
func WithMetadata(m Metadata) TraceOption       // 透传给结果，便于调用方关联业务
```

### 3.3 批量选项（`BatchOption`）

```go
func WithConcurrency(n int) BatchOption         // 并发上限（socket 模式生效）
func WithBatchTimeout(d time.Duration) BatchOption
func WithBatchRetry(n int) BatchOption
func WithTraceConfig(c *TraceConfig) BatchOption
func WithMetadata(m Metadata) BatchOption
func WithStopOnError(stop bool) BatchOption     // 默认 false：不中断整批
func WithProgress(fn func(done, total int)) BatchOption
```

> 传 `...TraceOption` 和 `...BatchOption` 的类型是分开的，避免误用；`BatchOption` 内部可包含 `TraceOption`。

## 4. 结果类型

```go
type TraceResult struct {
    Target   string        // 请求时传入的原始目标
    Address  string        // scamper 实际使用的目标地址
    Method   string        // udp / icmp / tcp / tcp-ack / udp-paris / icmp-paris
    Hops     []Hop
    StopReason string      // COMPLETED / HOPLIMIT / GAPLIMIT / LOOP / ...
    Duration time.Duration // 端到端耗时
    Error    error         // 单目标错误；err == nil 表示测量成功
    Metadata Metadata      // 用户透传信息 + SDK 附加信息
}

type Hop struct {
    TTL       int           // 探测该跳所用的 TTL（probe_ttl）
    Address   string        // 该跳应答地址
    RTT       time.Duration // 往返时延
    Name      string        // 反向 DNS（-O ptr 时）
    ReplyTTL  int           // 应答包 TTL
    ProbeSize int
}

type Metadata map[string]any  // 便捷 JSON 友好类型
```

设计要点：

- **Legacy 与 Socket 返回完全一致的结构**，由 `internal/parser` 统一产出。
- `Duration` 由 SDK 计时（不依赖 scamper 内部时间戳），便于两种模式对齐。
- `StopReason` 保留 scamper 原始语义，供高级用户决策；`Hops` 是稳定字段。

## 5. 错误模型

```go
var (
    ErrNoScamperBinary = errors.New("scamper: binary not found")
    ErrDaemonUnavailable = errors.New("scamper: daemon unavailable")
    ErrProtocol = errors.New("scamper: protocol error")
    ErrParse = errors.New("scamper: result parse error")
    ErrCommand = errors.New("scamper: command rejected")
)

// 结构化错误
type CommandError struct { Args []string; Message string }   // scamper 返回 ERR
type TimeoutError struct { Target string; Timeout time.Duration }
type ClientClosedError struct{}
```

- 所有错误可用 `errors.Is/As` 判定。
- `TraceBatch` 把单目标错误放进 `TraceResult.Error`；只有整个批次无法启动时才返回顶层 error。
- 可重试判定：`IsRetryable(err)` 辅助函数（daemon 不可用、超时、协议错误返回 true；命令错误返回 false）。

## 6. context 语义

- `ctx` 取消 / 超时会中止该目标（socket 模式向 daemon 发 `halt <id>`）。
- `WithTimeout` 提供单目标超时；`TraceBatch` 的 `ctx` 约束整批。
- Client 自身持有长生命周期 ctx（由 `New*` 时传入或内部 `context.Background()` + `Close()`）。

## 7. 使用示例

### 7.1 Socket 单目标

```go
c, err := scamper.NewSocketClient(
    scamper.WithSocketPath("/var/run/scamper/scamper.sock"),
)
if err != nil { log.Fatal(err) }
defer c.Close()

cfg := scamper.NewTraceConfig().
    Protocol(scamper.ProtocolUDP).
    FirstHop(9).MaxHops(16).
    Wait(time.Second).ProbesPerHop(1).
    Source("192.168.6.254")

res, err := c.Trace(ctx, "38.60.89.121", scamper.WithTraceConfig(cfg))
fmt.Println(len(res.Hops), res.StopReason, res.Duration)
```

### 7.2 批量并发

```go
results, err := c.TraceBatch(ctx, targets,
    scamper.WithConcurrency(64),
    scamper.WithTraceConfig(cfg),
    scamper.WithBatchRetry(1),
    scamper.WithProgress(func(done, total int) { log.Printf("%d/%d", done, total) }),
)
for _, r := range results {
    _ = r
}
```

### 7.3 Legacy

```go
c, _ := scamper.NewLegacyClient()
defer c.Close()
res, _ := c.Trace(context.Background(), "1.1.1.1")
```

## 8. 版本与兼容策略

- 首个版本 `v0.x`；`v1.0` 冻结 `Client` 接口与 `TraceResult` 字段。
- 采用 functional options，新增能力不破坏旧调用。
- `ExtraArgs` 保证 scamper 新增参数时无需等 SDK 发版。
