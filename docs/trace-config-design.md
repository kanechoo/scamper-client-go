# TraceConfig 设计

## 1. 目标

用 **Go API builder** 完整表达 scamper `trace` 命令的参数，替代字符串 `cmd-args`。
覆盖 `man scamper` 中 trace 命令的全部选项；未被覆盖或未来新增的参数通过
`ExtraArgs` 原样透传。

参见权威来源：`man 1 scamper` 的 `TRACEROUTE OPTIONS` 一节。

## 2. Builder API

```go
cfg := scamper.NewTraceConfig().
    Protocol(scamper.ProtocolUDP).   // -P
    FirstHop(9).                     // -f
    MaxHops(16).                     // -m
    Wait(time.Second).               // -w
    ProbesPerHop(1).                 // -q
    Source("192.168.6.254").         // -S
    GapLimit(12).                    // -g
    GapAction(scamper.GapActionHalt).// -G
    DstPort(33434).                  // -d
    SrcPort(0).                      // -s
    TOS(0).                          // -t
    Squeries(1).                     // -N
    Loops(1).                        // -l
    WaitProbe(2 * time.Second).      // -W（步进 10ms）
    WaitProbeHop(time.Second).       // -H（上限 2s）
    PayloadHex("00").                // -p
    RouterAddr("").                  // -r
    UserID(0).                       // -U
    Stream(0).                       // -y
    PMTUD(true).                     // -M
    AllProbes(false).                // -Q
    TTLExceededNotDest(false).       // -T
    Option(scamper.OptionPTR).       // -O ptr（可多次）
    ExtraArgs("--future-flag", "x")  // 兜底透传
```

### 设计原则

1. **只输出被显式设置的参数**：未调用对应方法就完全交给 scamper 默认值，避免 SDK 与 scamper 默认值漂移。
2. **链式、可克隆**：`TraceConfig` 用指针接收者返回自身，支持链式；提供 `Clone()` 防止跨调用共享。
3. **不拼接 shell 字符串**：`Build()` 直接产出 `[]string` argv，两种模式分别组装，杜绝 shell 注入。
4. **可校验**：`Validate()` 在提交前发现非法组合（如 `FirstHop > MaxHops`）。

## 3. 方法 ↔ scamper 选项对照

| scamper | 类型/范围 | Builder 方法 | 说明 |
|---------|-----------|--------------|------|
| `-P method` | enum | `Protocol(Method)` | udp/icmp/udp-paris/icmp-paris/tcp/tcp-ack（大小写不敏感） |
| `-f firsthop` | 1..255 | `FirstHop(int)` | 起始 TTL |
| `-m maxttl` | 1..255 | `MaxHops(int)` | 最大 TTL |
| `-w wait` | 秒 | `Wait(time.Duration)` | 每个 probe 等待秒数（默认 scamper 5s） |
| `-q attempts` | ≥1 | `ProbesPerHop(int)` | 每跳最大尝试次数 |
| `-S srcaddr` | IPv4/IPv6 | `Source(string)` | 源地址（不可伪造） |
| `-d dport` | 0..65535 | `DstPort(int)` | UDP/TCP 目的端口基址；ICMP-Paris 下为校验和 |
| `-s sport` | 0..65535 | `SrcPort(int)` | 源端口 / ICMP ID；0=由 OS 分配 |
| `-t tos` | 0..255 | `TOS(int)` | IP ToS/DSCP+ECN |
| `-c confidence` | 95 或 99 | `Confidence(Confidence)` | 逐跳高置信度探测 |
| `-g gaplimit` | ≥0 | `GapLimit(int)` | 连续无响应跳数上限（默认 5，0=禁用） |
| `-G gapaction` | 1/2 | `GapAction(GapAction)` | 1=halt，2=last-ditch probes |
| `-H wait-probe-hop` | 0..2s | `WaitProbeHop(time.Duration)` | 同 TTL 连续 probe 最小间隔 |
| `-l loops` | ≥0 | `Loops(int)` | 允许的环路数（默认 1，0=禁用检测） |
| `-N squeries` | ≥1 | `Squeries(int)` | 允许同时在途的跳数（须 < gaplimit） |
| `-o offset` | 0..8191 | `Offset(int)` | 分片偏移 |
| `-p payload` | hex | `PayloadHex(string)` | probe 负载基址（hex） |
| `-r rtraddr` | IP | `RouterAddr(string)` | 指定路由器地址 |
| `-U userid` | uint32 | `UserID(uint32)` | 用户自定义标识，随数据返回 |
| `-w`/`-W`/`-y` | — | 见上表 | — |
| `-y stream` | ≥0 | `Stream(int)` | 每 N 包上报一次中间结果 |
| `-M` | bool | `PMTUD(bool)` | 探测后做 PMTU 发现 |
| `-Q` | bool | `AllProbes(bool)` | 无论收到多少响应都发完所有 probe |
| `-T` | bool | `TTLExceededNotDest(bool)` | 目的地的 time-exceeded 不算到达 |
| `-O option` | set | `Option(Option)` | back / const-payload / dl / dtree-noback / ptr / raw |

> 双树/datatree 专用参数（`-z gss-entry`、`-Z lss-name`）在 v0.1 **不提升为稳定的
> Go API**（不是因为不重要，而是暂不冻结其签名），需要时统一用 `ExtraArgs` 透传；
> 待设计稳定后再补专用方法。

### 枚举

```go
type Method string
const (
    ProtocolUDP       Method = "udp"
    ProtocolICMP      Method = "icmp"
    ProtocolUDPParis  Method = "udp-paris"
    ProtocolICMPParis Method = "icmp-paris"
    ProtocolTCP       Method = "tcp"
    ProtocolTCPAck    Method = "tcp-ack"
)

type GapAction int
const (
    GapActionHalt      GapAction = 1
    GapActionLastDitch GapAction = 2
)

type Confidence int
const (
    Confidence95 Confidence = 95
    Confidence99 Confidence = 99
)

type Option string
const (
    OptionBack        Option = "back"
    OptionConstPayload Option = "const-payload"
    OptionDL          Option = "dl"
    OptionDTreeNoBack Option = "dtree-noback"
    OptionPTR         Option = "ptr"
    OptionRaw         Option = "raw"
)
```

## 4. ExtraArgs（向前兼容）

```go
func (c *TraceConfig) ExtraArgs(args ...string) *TraceConfig
```

- 原样追加到 argv 尾部，未来 scamper 新增参数无需改 SDK。
- 与专用方法冲突时**以 ExtraArgs 为准**（后写覆盖前写由 scamper 自行决定，SDK 不干预）。
- 安全约束：拒绝包含 `\n`/`\r`/`\x00` 的 token（会破坏 socket 行协议）；`Source`/`RouterAddr` 等在进入 argv 前也做同样检查。

## 5. Build 与校验

```go
// Build 产出 argv（不含 "trace" 和 target，也不含 --）。
// 输出仅包含被显式设置的选项。
func (c *TraceConfig) Build() ([]string, error)
```

- `Validate()` 规则示例：
  - `1 <= FirstHop <= MaxHops <= 255`
  - `Confidence ∈ {95, 99}`、`GapAction ∈ {1,2}`
  - `Method` 合法
  - `PayloadHex` 为偶数长度合法 hex
  - `Wait/ProbesPerHop/Squeries/Loops` 非负
  - `Squeries < GapLimit`（scamper 要求）
- 非法配置返回 `ErrInvalidConfig`（`errors.Is` 可判定）。

## 6. 与两种模式的组装

同一份 `TraceConfig` 生成统一的命令 token 序列，再按模式包装：

```
argv = ["trace", <cfg.Build()...>, <target>]

Legacy:  scamper -O json -I "<join(argv,' ')>"       // 单条命令，无 shell
Socket:  连接后 attach format json，发送 "<join(argv,' ')>"
```

由于 argv 由 SDK 校验（无空白/换行 token），两种模式的 join 都安全；不经过任何 shell。

## 7. 默认值

| 项 | SDK 默认 | 说明 |
|----|----------|------|
| Protocol | 不设置（scamper 默认 udp-paris） | 需要确定协议时显式设置 |
| FirstHop | 不设置（scamper 默认 1） | |
| MaxHops | 不设置（scamper 默认无限制） | 建议生产环境显式限制 |
| Wait | 不设置（scamper 默认 5s） | |
| ProbesPerHop | 不设置（scamper 默认 2） | |
| Source | 不设置 | 需要绕过隧道时显式指定 |
| 其它 | 不设置 | 一律交给 scamper |

> SDK 不替用户猜默认值，避免"看起来一样、实际不同"的隐性行为。

## 8. 测试要点

- **Build 单测**：每个方法 → 期望 argv；组合顺序；ExtraArgs 追加；非法值报错。
- **等价性测试**：同一配置分别走 Legacy/Socket，断言两者结果结构一致（`Hops`、`StopReason`）。
- **参数边界**：hex 负载、IPv6 source、Confidence/GapAction 非法值。
