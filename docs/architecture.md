# scamper-client-go 架构设计

## 1. 目标与范围

`scamper-client-go` 是一个**通用 Go SDK**，用于驱动 [scamper](https://www.caida.org/catalog/software/scamper/)
做 traceroute 测量。它不是某个业务项目的封装，只抽离可复用的通用能力：

**SDK 负责**

- 连接（连接 scamper daemon 的 control socket，或调用本地 scamper CLI）
- 提交任务（把 `TraceConfig` 转成 scamper trace 命令）
- 接收结果（读取 JSON 输出）
- 解析结果（scamper JSON → 统一的 Go struct）
- 并发调度（socket 模式下的 worker pool / 有界队列 / 背压 / 重试）

**SDK 不负责**

- 启动 / 停止 scamper daemon
- 配置 daemon 的窗口、PPS、权限
- 管理系统环境、安装 scamper

daemon 的 `-D`、`-w`、`-p`、socket 权限等一律由用户自行负责；SDK 只在给定 socket 路径上 connect/submit/receive/parse。

## 2. 两种运行模式

| 模式 | 落点 | 依赖 | 适用 |
|------|------|------|------|
| Legacy | `exec.Command` 调本地 `scamper` | 系统装了 scamper，无需 daemon | 简单使用、开发测试、无 daemon 环境 |
| Socket | Unix Domain Socket 连常驻 daemon | 需要 daemon（由用户启动） | 高并发、批量测量、生产 |

两种模式对用户**完全透明**，返回相同的 `*TraceResult`。

```
Legacy:  Go ──exec──▶ scamper CLI ──stdout(JSON)──▶ Parser ──▶ TraceResult
Socket:  Go ──UDS───▶ scamper daemon ──DATA(JSON)──▶ Parser ──▶ TraceResult
```

## 3. 分层与包结构

```
scamper-client-go/
├── go.mod
├── README.md
├── doc.go                     # 包级文档
├── client.go                  # Client 接口 + 公共类型别名 + 工厂
├── option.go                  # 通用 functional options
├── trace.go                   # TraceOption / BatchOption
├── result.go                  # TraceResult / Hop / Metadata
├── errors.go                  # 统一错误类型与哨兵
├── config.go                  # TraceConfig（builder）
│
├── legacy/
│   └── client.go              # Legacy 实现（exec scamper）
│
├── socket/
│   ├── client.go              # Socket 实现（attach 协议 + 并发调度）
│   ├── protocol.go            # attach 行协议：OK/MORE/DATA/ERR
│   ├── session.go             # 单连接的读/写 goroutine 与 id 映射
│   ├── reconnect.go           # 重连与连接健康
│   └── scheduler.go           # worker pool + 有界队列 + 背压
│
├── internal/
│   └── parser/
│       └── json.go            # scamper JSON → TraceResult（两种模式共用）
│
├── examples/
│   ├── legacy/main.go
│   ├── socket/main.go
│   ├── batch/main.go
│   └── custom-args/main.go
│
└── docs/…
```

### 依赖方向

```
examples ─┐
          ├─▶ scamper (root) ──▶ legacy/ socket/
          │                         │        │
          │                         └───┬────┘
          │                             ▼
          └────────────────────▶ internal/parser
```

- root 包定义接口与公共类型，是唯一的稳定 API 面。
- `legacy/` 与 `socket/` 是两种实现，互不依赖。
- `internal/parser` 被两种实现共用，返回 root 的 `*TraceResult`。
- 用户既可以依赖 root 接口（推荐），也可以直接用具体实现包。

## 4. 统一抽象

```go
// root package: scamper
type Client interface {
    Trace(ctx context.Context, target string, options ...TraceOption) (*TraceResult, error)
    TraceBatch(ctx context.Context, targets []string, options ...BatchOption) ([]TraceResult, error)
    Close() error
}
```

- `Trace`：单目标，高级用户自行控制 goroutine。
- `TraceBatch`：批量，SDK 内部负责并发、背压、id 映射、超时、重试（socket 模式为多路复用；legacy 模式为受限进程池）。
- 构造：

```go
scamper.NewLegacyClient(...)  // 返回 scamper.Client
scamper.NewSocketClient(...)  // 返回 scamper.Client
```

## 5. 统一结果结构

两种模式都返回相同的结构，用户无法（也不需要）感知结果来源：

```go
type TraceResult struct {
    Target   string
    Address  string        // 目标解析后的地址（如 scamper 给出）
    Hops     []Hop
    Duration time.Duration
    Error    error         // 单目标错误；批量时按元素返回，不中断整批
    Metadata Metadata      // 原始信息：stop_reason、method、probe 数等
}

type Hop struct {
    TTL      int
    Address  string
    RTT      time.Duration
    Name     string        // 可选（-O ptr 时）
    ReplyTTL int
    // 其余可扩展字段
}
```

## 6. 参数配置

主配置方式是 **Go API builder**（`scamper.NewTraceConfig()`），覆盖 scamper trace 命令的全部常用参数；不使用 cmd-args 字符串作为主入口。为向前兼容保留 `ExtraArgs(...string)` 透传未来新增参数。详见 `trace-config-design.md`。

## 7. 输出格式

- **仅支持 JSON**。
- Legacy：以 `-O json -I "<trace command>"` 调用，逐行解析 JSON。
- Socket：`attach format json`，解析 `DATA` 帧中的 JSON。
- 不实现 WARTS 及其 parser；`internal/parser` 只做 JSON → struct。
- 用户永远只看到 `TraceResult`。

## 8. 并发与可靠性

- 并发只在 socket 模式有意义；`TraceBatch` + `WithConcurrency` 触发内部调度。
- 单连接、单 writer、单 reader，按 scamper 分配的 `id` 做 task/result 映射。
- 断线指数退避重连；在途任务判为可重试错误，由调用方决定重跑或由 SDK 内建重试。
- 详见 `concurrency-design.md`、`socket-design.md`。

## 9. 平台与兼容性

- 目标平台：macOS + Linux（Unix Domain Socket 与 `exec` 均为 POSIX）。
- 纯 Go 标准库实现，尽量零第三方依赖（不引入 WARTS 解析库）。
- Go 版本：`go 1.22+`（使用标准库 `os/exec`、`net`、`encoding/json`、`sync`）。

## 10. 非目标（Non-goals）

- 不做 WARTS / warts.gz 支持。
- 不管理 daemon 生命周期与权限。
- 不实现 traceroute 本身（那是 scamper 的职责）。
- 不绑定任何业务数据结构（如 CN2 判定）。
