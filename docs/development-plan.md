# 开发计划

## 1. 总体策略

严格分两阶段：**先设计、后编码**。本文件是阶段一（设计）的收尾；确认后再进入阶段二（实现）。

- 阶段一（已完成）：输出 `docs/` 六份设计文档，冻结 API 与协议细节。
- 阶段二：按里程碑实现、测试、示例、README，最后独立 `go build` + `go test` 通过。

## 2. 模块与仓库

| 项 | 值 |
|----|----|
| module path | `github.com/kanechoo/scamper-client-go`（已确认） |
| 本地路径 | `/Users/kanechu/my-project/go/scamper-client-go` |
| 目标平台 | macOS + Linux |
| Go 版本 | `go 1.22` |
| 第三方依赖 | 无（纯标准库） |
| 默认 socket 路径 | `/var/run/scamper/scamper.sock`（`DefaultSocketPath`，已确认） |
| `TraceBatch` 返回 | `[]TraceResult`（值，已确认） |
| 默认重试策略 | 不重试（`WithRetry(0)`，已确认） |
| License | 待定（建议 MIT） |

> **待你确认**：module path 的账号/组织名（示例用 `kanechoo`，最终以 `go get` 可用的真实路径为准）。

## 3. 里程碑

### M0 — 骨架（0.5 天）
- `go.mod`、`doc.go`、根包接口与公共类型（`client.go` / `options` / `trace.go` / `result.go` / `errors.go` / `config.go`）。
- `internal/parser` 的 JSON → `TraceResult`（先用固定样例测试）。
- 目标：`go build ./...` 通过，`TraceConfig.Build()` 单测通过。

### M1 — Legacy 实现（1 天）
- `legacy/client.go`：`exec.Command` 调 `scamper -O json -I "trace ..."`，解析多行 JSON，取 `type=="trace"`。
- 单目标 `Trace`、受限进程池 `TraceBatch`。
- 单测：用 PATH 中的假 `scamper` 脚本（或注入 `exec` 接口）覆盖 argv 组装与解析。
- 集成测试：若本机有 scamper，跑一个真实目标。

### M2 — Socket 实现（2–3 天）
- `socket/protocol.go`：attach 握手、`OK/MORE/DATA/ERR` 帧解析、`halt`。
- `socket/session.go`：单 writer + 单 reader + `acceptQueue` + `id→job`。
- `socket/reconnect.go`：supervisor、退避重连、generation、断线在途失败。
- `socket/scheduler.go`：worker pool + 有界队列 + inflight 背压 + 重试 + progress。
- 单测：fake unix socket 服务端，覆盖乱序、无 id 帧、半包、断线、ERR、超时 halt。
- 集成测试：`SCAMPER_SOCKET` 存在时跑 500 目标并发。

### M3 — 示例与文档（1 天）
- `examples/legacy`、`examples/socket`、`examples/batch`、`examples/custom-args`。
- `README.md`：安装、两种模式、快速开始、TraceConfig 速查、并发与 daemon 前置说明。

### M4 — 打磨与发布（1 天）
- `go vet`、`go test -race`、`golangci-lint`（可选）、覆盖率报告。
- v0.1.0 tag；`docs/` 与 README 对齐。
- 评估是否补充双树参数方法、`Format` 扩展点。

## 4. 目录交付清单

```
scamper-client-go/
├── go.mod / go.sum
├── README.md
├── doc.go
├── client.go       # Client 接口 + 工厂
├── option.go       # ClientOption
├── trace.go        # TraceOption / BatchOption
├── config.go       # TraceConfig builder
├── result.go       # TraceResult / Hop / Metadata
├── errors.go       # 错误类型
├── legacy/client.go
├── socket/{client,protocol,session,reconnect,scheduler}.go
├── internal/parser/json.go
├── examples/{legacy,socket,batch,custom-args}/main.go
└── docs/*.md
```

## 5. 测试计划

| 层级 | 方式 | 是否依赖环境 |
|------|------|--------------|
| 单元 | `TraceConfig.Build`、parser、protocol 帧解析 | 否 |
| 假服务端 | `net.Listen("unix", tmp)` 模拟 daemon 帧 | 否 |
| 假进程 | PATH 注入假 scamper 脚本 | 否 |
| 集成（legacy） | 本机 scamper CLI | 有则跑，无则 skip |
| 集成（socket） | `SCAMPER_SOCKET` 指向真实 daemon | 有则跑，无则 skip |
| 并发 | 500 目标 × 并发 64，`-race` | socket 集成时 |

CI 建议：默认只跑单元/假服务端；集成测试用环境变量门控。

## 6. 验收标准

- [ ] 独立 `go build ./...`、`go test ./...`、`go test -race ./...` 通过。
- [ ] `go vet` 无告警。
- [ ] Legacy 与 Socket 对同一 `TraceConfig`、同一目标返回结构一致的 `*TraceResult`。
- [ ] Socket 模式 500 目标批量无丢失、无重复、顺序正确。
- [ ] 断线后自动重连，在途任务返回可重试错误。
- [ ] 超时任务会向 daemon 发送 `halt`。
- [ ] 所有导出符号有中文注释；`ExtraArgs` 可透传未知参数。
- [ ] README + 四个 examples 可运行。

## 7. 风险与对策

| 风险 | 影响 | 对策 |
|------|------|------|
| `DATA <len>` 长度语义（字符 vs 字节）在含非 ASCII 时差异 | 帧错位 | 只处理 JSON（ASCII 转义）；单测覆盖；必要时按字节读并校验 |
| daemon 版本差异导致帧格式细微不同 | 解析失败 | 宽松解析 + 忽略未知行；记录原始行便于排查 |
| socket 路径/权限因环境而异 | 连不上 | 默认常量 + `WithSocketPath` 覆盖；清晰 `ErrDaemonUnavailable` |
| 高并发下的本地丢包影响测量质量 | 结果偏差 | 文档建议 `WithConcurrency ≈ daemon -w`；不盲目堆并发 |
| Legacy 进程开销大 | 批量慢 | 文档标注；legacy 进程池默认小并发 |
| RTT/时间字段因模式不同 | 结构不一致 | `Duration` 由 SDK 统一计时；保证字段语义一致 |

## 8. 非目标

- 不实现 WARTS / warts.gz 解析。
- 不管理 daemon 生命周期、权限、`-w/-p`。
- 不实现 traceroute 协议本身。
- 不包含任何业务判定逻辑（如 CN2）。

## 9. 已确认决策（2026-09）

1. **module path**：`github.com/kanechoo/scamper-client-go`。
2. **默认 socket 路径**：`/var/run/scamper/scamper.sock`（常量 `DefaultSocketPath`，可被 `WithSocketPath` 覆盖）。
3. **`TraceBatch` 返回**：`[]TraceResult`（值切片，顺序与入参一致）。
4. **默认重试策略**：不重试（`WithRetry(0)` / `WithBatchRetry(0)`），仅在显式请求时重试可重试错误。
5. **双树参数 `-z/-Z`**：v0.1 不提升为稳定的 Go API，统一通过 `ExtraArgs` 透传（v0.2 再评估专用方法）。

> 已同步至 `api-design.md`、`trace-config-design.md`、`socket-design.md`。设计冻结，进入 M0。
