// Package core 定义 scamper-client-go 的公共类型、配置、错误与选项。
//
// 它是 root 包（github.com/kanechoo/scamper-client-go）与两个实现包
// （legacy、socket）共享的内部基础：root 通过类型别名把这里的类型导出为
// 稳定的公共 API，两个实现包直接引用这里以避免 Go 的包循环依赖。
//
// core 属于 internal，不对外暴露，用户应始终 import root 包。
package core

import (
	"log/slog"
	"math/rand/v2"
	"time"
)

// DefaultSocketPath 是 scamper daemon control socket 的默认路径。
//
// daemon 实际监听的路径由用户启动 scamper 时的 -U 参数决定，可用
// WithSocketPath 覆盖。SDK 只负责 connect，不负责创建或清理该文件。
const DefaultSocketPath = "/var/run/scamper/scamper.sock"

// Metadata 是附加到结果上的 JSON 友好键值集合。
//
// 它同时承载两类信息：
//   - 用户在调用时通过 WithMetadata / WithBatchMetadata 透传的业务信息；
//   - SDK 从 scamper 原始输出中提取的关键字段（如 stop_data、probe_count）。
//
// 用户传入的键在冲突时优先，便于调用方覆盖 SDK 的默认附加信息。
type Metadata map[string]any

// MergeMetadata 按顺序合并多个 Metadata，后者覆盖前者，返回新 map。
// 当所有入参都为空时返回 nil，避免给结果附加空 map。
func MergeMetadata(ms ...Metadata) Metadata {
	var out Metadata
	for _, m := range ms {
		if len(m) == 0 {
			continue
		}
		if out == nil {
			out = make(Metadata, len(m))
		}
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// Clone 返回 Metadata 的浅拷贝；nil 接收者返回 nil。
func (m Metadata) Clone() Metadata {
	if m == nil {
		return nil
	}
	out := make(Metadata, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Hop 表示 traceroute 路径上的一跳。
type Hop struct {
	// TTL 是探测该跳所用的 TTL（scamper 字段 probe_ttl）。
	TTL int
	// Address 是该跳应答的地址；无应答时为 scamper 给出的 "*" 占位地址。
	Address string
	// RTT 是该跳的往返时延（scamper 的 rtt 字段，单位毫秒）。
	RTT time.Duration
	// Name 是反向 DNS 名称，仅当配置 -O ptr 时由 scamper 填充。
	Name string
	// ReplyTTL 是应答报文的 IP TTL（scamper 字段 reply_ttl）。
	ReplyTTL int
	// ProbeSize 是探测报文的大小（字节）。取自 scamper 的 probe_size 字段。
	ProbeSize int
}

// TraceResult 是一次 traceroute 测量的统一结果。
//
// Legacy 与 Socket 两种模式返回完全一致的结构，用户无需感知结果来源。
type TraceResult struct {
	// Target 是调用时传入的原始目标。
	Target string
	// Address 是 scamper 实际使用的目标地址（解析后的 dst）。
	Address string
	// Method 是 scamper 使用的探测方法：udp / icmp / tcp / tcp-ack /
	// udp-paris / icmp-paris 等。
	Method string
	// Hops 是路径上的跳列表，按 TTL 升序（scamper 的 hops）。
	Hops []Hop
	// StopReason 是 scamper 的停止原因原始语义：COMPLETED / HOPLIMIT /
	// GAPLIMIT / LOOP / UNREACH / ERROR / HALTED 等。
	StopReason string
	// Duration 是本次测量的端到端耗时，由 SDK 计时（不依赖 scamper 内部
	// 时间戳），便于两种模式对齐。
	Duration time.Duration
	// Error 是单目标错误；Error == nil 表示测量成功。
	//
	// 批量测量时按元素返回，不影响其它目标。
	Error error
	// Metadata 是用户透传信息与 SDK 附加信息的合集。
	Metadata Metadata
}

// ReconnectPolicy 描述 socket 模式断线重连的指数退避参数。
type ReconnectPolicy struct {
	// MinBackoff 是首次重连等待时间，同时也是退避重置后的基准。
	MinBackoff time.Duration
	// MaxBackoff 是两次重连之间的等待时间上限。
	MaxBackoff time.Duration
	// Multiplier 是每次失败后的退避倍数，必须 >= 1。
	Multiplier float64
	// Jitter 为 true 时在退避时间上叠加随机抖动，避免惊群。
	Jitter bool
}

// DefaultReconnectPolicy 返回 socket 模式默认的重连策略：
// 1s 起步、30s 封顶、2 倍递增、开启抖动。
func DefaultReconnectPolicy() ReconnectPolicy {
	return ReconnectPolicy{
		MinBackoff: time.Second,
		MaxBackoff: 30 * time.Second,
		Multiplier: 2,
		Jitter:     true,
	}
}

// Normalized 返回补齐了零值字段的策略，保证 supervisor 始终可安全工作。
func (p ReconnectPolicy) Normalized() ReconnectPolicy {
	if p.MinBackoff <= 0 {
		p.MinBackoff = time.Second
	}
	if p.MaxBackoff <= 0 {
		p.MaxBackoff = 30 * time.Second
	}
	if p.MaxBackoff < p.MinBackoff {
		p.MaxBackoff = p.MinBackoff
	}
	if p.Multiplier < 1 {
		p.Multiplier = 2
	}
	return p
}

// Next 依据策略计算下一次退避时长。
//
// 当 Jitter 为 true 时采用 equal jitter：结果落在 [next/2, next) 区间，既保留
// 指数增长的下界，又打散多个客户端的重连时刻。
func (p ReconnectPolicy) Next(cur time.Duration) time.Duration {
	p = p.Normalized()
	if cur <= 0 {
		cur = p.MinBackoff
	}
	next := time.Duration(float64(cur) * p.Multiplier)
	if next <= 0 {
		next = p.MaxBackoff
	}
	if next > p.MaxBackoff {
		next = p.MaxBackoff
	}
	if p.Jitter {
		if half := next / 2; half > 0 {
			next = half + time.Duration(rand.Int64N(int64(half)))
		}
	}
	return next
}

// ClientOptions 是两个实现包共享的客户端构造配置。
//
// 用户通过 root 包的 ClientOption（WithSocketPath、WithBinary 等）设置，
// 这里以导出字段承载，供内部实现读取。
type ClientOptions struct {
	// SocketPath 是 Unix domain socket 路径（socket 模式）。
	SocketPath string
	// Binary 是 scamper 可执行文件路径（legacy 模式）；空表示在 PATH 中查找。
	Binary string
	// DialTimeout 是建连与 attach 握手超时。
	DialTimeout time.Duration
	// Reconnect 是断线重连策略（socket 模式）。
	Reconnect ReconnectPolicy
	// DefaultConfig 是客户端默认的 TraceConfig，可被单次调用覆盖。
	DefaultConfig *TraceConfig
	// Logger 是结构化日志器；nil 表示静默。
	Logger *slog.Logger
	// MaxInflight 是 socket 模式在途任务上限（信号量容量）；<=0 表示不限制。
	MaxInflight int
	// HealthCheck 是主动健康检查间隔；当前实现为保留项，行为仍是仅依赖读写错误
	// 的被动检测（attach 模式不支持交互式命令，无法安全主动探测）。
	HealthCheck time.Duration
	// Metrics 是可选指标接收器；nil 表示 no-op。
	Metrics MetricsSink
}

// DefaultClientOptions 返回客户端的默认构造配置。
func DefaultClientOptions() ClientOptions {
	return ClientOptions{
		SocketPath:  DefaultSocketPath,
		DialTimeout: 5 * time.Second,
		Reconnect:   DefaultReconnectPolicy(),
		MaxInflight: 64,
	}
}

// MetricsSink 接收调度层与连接层的可选指标回调。
//
// 所有方法都可能在多个 goroutine 上并发调用，实现必须自身保证线程安全。
// 默认（WithMetrics 未设置时）不会调用任何方法。
type MetricsSink interface {
	// OnSubmit 在任务进入批量调度队列时调用。
	OnSubmit(target string)
	// OnResult 在单个目标完成后调用；err 为 nil 表示成功。
	OnResult(target string, d time.Duration, err error)
	// OnReconnect 在 daemon control 连接成功（重）建时调用。
	OnReconnect()
	// OnQueueDepth 在调度队列深度变化时调用。
	OnQueueDepth(depth int)
}
