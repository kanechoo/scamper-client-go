# scamper-client-go

驱动 [scamper](https://www.caida.org/catalog/software/scamper/) 做 traceroute 测量的通用 Go SDK。

纯 Go 标准库实现，零第三方依赖，支持 macOS 与 Linux，Go 1.22+。

## 职责边界

**SDK 负责**

- 连接：本地 `scamper` CLI（legacy）或 daemon 的 control socket（socket）
- 提交任务：把 `TraceConfig` 转成 scamper trace 命令
- 接收结果：读取并解析 JSON 输出为统一的 `*TraceResult`
- 并发调度：worker pool、有界队列、背压、id 映射、超时、重试

**SDK 不负责**

- 启动 / 停止 scamper daemon
- 配置 daemon 的窗口（`-w`）、PPS（`-p`）、权限与 socket 路径
- 实现 traceroute 协议本身，也不做 WARTS 解析

daemon 相关的 `scamper -D -U <socket>`、socket 权限等一律由用户自行负责。

## 安装

```bash
go get github.com/kanechoo/scamper-client-go
```

## 两种模式

| 模式 | 落点 | 依赖 | 适用 |
|------|------|------|------|
| Legacy | `exec` 本地 `scamper` | 系统装了 scamper，无需 daemon | 简单使用、开发测试 |
| Socket | Unix domain socket 连常驻 daemon | 用户已启动 daemon | 高并发、批量、生产 |

两种模式对用户完全透明，返回相同的 `*TraceResult`。

## 快速开始

### Legacy

```go
c, err := scamper.NewLegacyClient() // 默认在 PATH 中找 scamper
if err != nil {
    log.Fatal(err)
}
defer c.Close()

cfg := scamper.NewTraceConfig().
    Protocol(scamper.ProtocolUDP).
    FirstHop(1).MaxHops(16).
    Wait(time.Second).ProbesPerHop(1)

res, err := c.Trace(context.Background(), "1.1.1.1", scamper.WithTraceConfig(cfg))
if err != nil {
    log.Fatal(err)
}
fmt.Println(len(res.Hops), res.StopReason, res.Duration)
```

### Socket

```go
c, err := scamper.NewSocketClient(
    scamper.WithSocketPath("/var/run/scamper/scamper.sock"),
)
if err != nil {
    log.Fatal(err) // 连接/attach 失败会 fail-fast
}
defer c.Close()
```

### 批量并发

```go
results, err := c.TraceBatch(ctx, targets,
    scamper.WithConcurrency(64),
    scamper.WithBatchTraceConfig(cfg),
    scamper.WithBatchTimeout(20*time.Second),
    scamper.WithBatchRetry(1),
    scamper.WithProgress(func(done, total int) { log.Printf("%d/%d", done, total) }),
)
for _, r := range results {
    if r.Error != nil {
        log.Printf("%s: %v", r.Target, r.Error)
        continue
    }
    _ = r
}
```

返回切片与 `targets` **等长且顺序一致**；单目标失败只写入该元素的 `Error`。

## 统一结果

```go
type TraceResult struct {
    Target     string        // 请求时传入的原始目标
    Address    string        // scamper 实际使用的地址
    Method     string        // udp / icmp / tcp / tcp-ack / udp-paris / icmp-paris
    Hops       []Hop
    StopReason string        // COMPLETED / HOPLIMIT / GAPLIMIT / LOOP / ...
    Duration   time.Duration // 由 SDK 计时，两种模式对齐
    Error      error         // 单目标错误
    Metadata   Metadata      // 用户透传 + SDK 附加信息
}

type Hop struct {
    TTL       int
    Address   string
    RTT       time.Duration
    Name      string // -O ptr 时
    ReplyTTL  int
    ProbeSize int
}
```

## TraceConfig 速查

所有方法只输出**被显式设置**的参数，未设置的一律交给 scamper 默认值。

| 方法 | scamper | 说明 |
|------|---------|------|
| `Protocol(Method)` | `-P` | udp / icmp / udp-paris / icmp-paris / tcp / tcp-ack |
| `FirstHop(int)` | `-f` | 起始 TTL，1..255 |
| `MaxHops(int)` | `-m` | 最大 TTL，1..255 |
| `Wait(time.Duration)` | `-w` | 每 probe 等待秒数 |
| `ProbesPerHop(int)` | `-q` | 每跳尝试次数，>=1 |
| `Source(string)` | `-S` | 源地址（IPv4/IPv6） |
| `DstPort(int)` / `SrcPort(int)` | `-d` / `-s` | 目的/源端口，0..65535 |
| `TOS(int)` | `-t` | ToS/DSCP+ECN，0..255 |
| `Confidence(Confidence)` | `-c` | 95 或 99 |
| `GapLimit(int)` | `-g` | 连续无响应上限，0 禁用 |
| `GapAction(GapAction)` | `-G` | 1=halt，2=last-ditch |
| `WaitProbe(time.Duration)` | `-W` | 相邻 probe 间隔（10ms 粒度） |
| `WaitProbeHop(time.Duration)` | `-H` | 同 TTL 间隔（秒，<=2s） |
| `Loops(int)` | `-l` | 环路数，0 禁用检测 |
| `Squeries(int)` | `-N` | 在途跳数，须 < GapLimit |
| `Offset(int)` | `-o` | 分片偏移，0..8191 |
| `PayloadHex(string)` | `-p` | probe 负载（hex） |
| `RouterAddr(string)` | `-r` | 指定路由器 IP |
| `UserID(uint32)` | `-U` | 用户自定义标识 |
| `Stream(int)` | `-y` | 每 N 包上报中间结果 |
| `PMTUD(bool)` / `AllProbes(bool)` / `TTLExceededNotDest(bool)` | `-M` / `-Q` / `-T` | 开关 |
| `Option(Option)` | `-O` | back / const-payload / dl / dtree-noback / ptr / raw |
| `ExtraArgs(...string)` | — | 透传未来新增参数 |

`Build()` 返回 `[]string` argv（不含 `trace` 与 target）；`Validate()` 在提交前发现
非法组合，返回可被 `errors.Is(err, scamper.ErrInvalidConfig)` 判定的错误。

## 配置选项

API 采用 functional options，分三层且类型相互独立，避免误用：`Trace` 只接受
`TraceOption`，`TraceBatch` 只接受 `BatchOption`。

### 客户端构造（`ClientOption`）

| 选项 | 作用 | 默认 |
|------|------|------|
| `WithSocketPath(path)` | socket 模式：daemon control socket 路径 | `DefaultSocketPath`（`/var/run/scamper/scamper.sock`） |
| `WithBinary(path)` | legacy 模式：scamper 可执行文件路径 | 在 `PATH` 中查找 `scamper` |
| `WithDialTimeout(d)` | socket 模式：建连与 attach 握手的超时 | 5s |
| `WithReconnectPolicy(p)` | socket 模式：断线重连指数退避（`MinBackoff`/`MaxBackoff`/`Multiplier`/`Jitter`） | 1s→30s、×2、开启抖动 |
| `WithDefaultTraceConfig(c)` | 客户端默认 `TraceConfig`，可被单次调用覆盖 | 不设置 |
| `WithLogger(l)` | 结构化日志（`*slog.Logger`） | 静默 |
| `WithMaxInflight(n)` | socket 模式：同一 Client 上同时在途的任务上限（信号量）；`<=0` 表示不限制 | 64 |
| `WithMetrics(sink)` | 指标回调（提交/完成/失败/重连/队列深度） | no-op |
| `WithHealthCheck(d)` | 预留项：scamper attach 模式不支持交互式命令，无法安全地主动探测，当前不改变行为 | 0 |

### 单次调用（`TraceOption`）

| 选项 | 作用 | 默认 |
|------|------|------|
| `WithTraceConfig(c)` | 覆盖客户端默认 `TraceConfig` | 客户端默认 |
| `WithTimeout(d)` | 单目标超时；超时会中止该目标并返回 `*TimeoutError` | 仅受 `ctx` 约束 |
| `WithRetry(n)` | 可重试错误的重试次数（由 `IsRetryable` 判定） | 0（不重试） |
| `WithMetadata(m)` | 透传业务信息，合并进结果的 `Metadata`（同名键用户优先） | 无 |

### 批量调用（`BatchOption`）

| 选项 | 作用 | 默认 |
|------|------|------|
| `WithConcurrency(n)` | worker 并发上限 | socket 16 / legacy 8 |
| `WithBatchTimeout(d)` | 每个目标的超时 | 仅受 `ctx` 约束 |
| `WithBatchRetry(n)` | 每个目标独立的重试次数 | 0（不重试） |
| `WithBatchTraceConfig(c)` | 批量覆盖 `TraceConfig` | 客户端默认 |
| `WithBatchMetadata(m)` | 批量透传元数据 | 无 |
| `WithStopOnError(b)` | 首个不可重试错误是否中止剩余任务 | `false`（跑完整批） |
| `WithProgress(fn)` | 结果落地回调 `fn(done, total int)`；可能被并发调用 | 无 |
| `WithQueueSize(n)` | 调度队列容量；实际容量为 `max(n, concurrency)` | 等于并发数 |
| `WithDropOnBackpressure(b)` | 队列满时快速失败（对应目标返回 `ErrBatchStopped`），而非阻塞提交 | `false`（阻塞形成背压） |

## 并发模型与 daemon 窗口

`TraceBatch` 的每个目标依次经过：提交端 → **有界队列** → **worker** → 单 writer/单连接 → daemon。

本地有三个含义不同的闸门：

| 参数 | 控制什么 | 说明 |
|------|----------|------|
| `WithConcurrency` | 同时执行任务的 goroutine 数 | worker pool 大小 |
| `WithMaxInflight` | 同一 Client 上同时“在途”的任务数 | 已提交 daemon、等待结果；跨多次并发调用共享 |
| `WithQueueSize` | 待处理任务缓冲 | 满时默认阻塞提交（背压），可改为快速失败 |

daemon 侧由用户配置：`-w` 是可同时进行的 active task 窗口，`-p` 是全局发包速率。
因此**实际并发 ≈ min(WithConcurrency, WithMaxInflight, daemon -w)**：

- 把本地参数设得高于 daemon 窗口不会更快，多余任务只会在 daemon 侧排队。
- 高并发突发还可能因上游对 ICMP/UDP 的限速而降低响应捕获率（测量质量下降）；
  实测 64–128 并发在吞吐与保真度之间更均衡。
- 建议 `WithConcurrency` 取接近 daemon 窗口（本仓库附带 daemon 为 `-w 100`，常用 64–100），
  并保证 `WithMaxInflight ≥ WithConcurrency`。
- 超时/取消时 SDK 会尽力向 daemon 发送 `halt <id>`，避免长期占用窗口。
- legacy 模式没有 daemon 窗口：每个目标一个进程，默认进程池为 8，开销明显高于 socket。

## 错误模型

```go
errors.Is(err, scamper.ErrDaemonUnavailable) // 可重试
errors.Is(err, scamper.ErrProtocol)          // 可重试
errors.Is(err, scamper.ErrParse)             // 可重试
errors.Is(err, scamper.ErrCommand)           // 不可重试
errors.Is(err, scamper.ErrInvalidConfig)     // 不可重试
errors.Is(err, scamper.ErrClosed)

var te *scamper.TimeoutError
errors.As(err, &te)

scamper.IsRetryable(err) // 辅助判定
```

## 目录结构

```
scamper-client-go/
├── client.go / option.go / trace.go / config.go / result.go / errors.go / doc.go
├── legacy/client.go
├── socket/{client,protocol,session,reconnect,scheduler}.go
├── internal/core        # 共享公共类型（root 以别名导出）
├── internal/parser      # JSON -> TraceResult
├── internal/run         # 超时/重试/批量调度
├── examples/{legacy,socket,batch,custom-args}
└── docs/
```

## 运行示例

```bash
# legacy（需本机安装 scamper）
go run ./examples/legacy 1.1.1.1

# socket（需先启动 daemon：scamper -D -U /var/run/scamper/scamper.sock）
go run ./examples/socket -socket /var/run/scamper/scamper.sock 1.1.1.1

# 批量
go run ./examples/batch -socket /var/run/scamper/scamper.sock 1.1.1.1 8.8.8.8

# 自定义参数 / ExtraArgs
go run ./examples/custom-args -socket /var/run/scamper/scamper.sock 1.1.1.1
```

## 常驻 daemon（macOS launchd / Linux systemd）

本仓库提供随开机启动的 daemon 配置（socket 使用默认路径
`/var/run/scamper/scamper.sock`，窗口 `-w 100`、速率 `-p 10000`）。

**macOS（launchd）**

```bash
sudo sh scripts/scamperd/install.sh     # 安装并启动
sudo sh scripts/scamperd/uninstall.sh   # 卸载
launchctl print system/com.scamper-client-go.scamperd
tail -f /var/log/scamper-client-go.scamperd.log
```

**Linux（systemd）**

```bash
sudo sh scripts/systemd/install.sh     # 安装并启动
sudo sh scripts/systemd/uninstall.sh   # 卸载
systemctl status scamper-client-go-scamperd
journalctl -u scamper-client-go-scamperd -f
```

安装后非 root 的 Go 进程即可通过默认路径连接（socket 以 0777 创建）。
Linux 上该路径为 `/run/scamper/scamper.sock`（`/var/run` 是指向 `/run` 的符号
链接，与 SDK 默认值等价）。`install.sh` 会自动探测 `scamper` 二进制路径。

> `-w 100` 是 daemon 的 active task 窗口，与 trace 命令里的 `-w`（每 probe
> 等待秒数）无关；客户端的 `WithConcurrency` 建议取接近该窗口的值。

## 测试

```bash
go test ./...
go test -race ./...
go vet ./...
```

默认只跑单元测试与假服务端测试。真实集成测试通过环境变量门控：

- `SCAMPER_SOCKET`：指向真实 daemon socket 时启用 socket 集成测试，否则 `t.Skip`。
- legacy 集成测试在本机存在 `scamper` 时运行，否则 `t.Skip`。

## License

[MIT](LICENSE)
