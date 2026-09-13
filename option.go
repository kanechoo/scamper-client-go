package scamper

import (
	"log/slog"
	"time"

	"github.com/kanechoo/scamper-client-go/internal/core"
)

// ClientOption 是客户端构造选项。
type ClientOption = core.ClientOption

// ReconnectPolicy 描述 socket 模式断线重连的指数退避参数。
type ReconnectPolicy = core.ReconnectPolicy

// MetricsSink 接收调度层与连接层的可选指标回调。
//
// 所有方法都可能被并发调用，实现需保证线程安全；默认（不设置）为 no-op。
type MetricsSink = core.MetricsSink

// WithSocketPath 设置 daemon control socket 路径（socket 模式）。
//
// 默认 core.DefaultSocketPath（/var/run/scamper/scamper.sock）。
func WithSocketPath(path string) ClientOption {
	return func(o *core.ClientOptions) {
		o.SocketPath = path
	}
}

// WithBinary 设置 scamper 可执行文件路径（legacy 模式）。
//
// 未设置时在 PATH 中查找 scamper。
func WithBinary(path string) ClientOption {
	return func(o *core.ClientOptions) {
		o.Binary = path
	}
}

// WithDialTimeout 设置建连与 attach 握手超时（socket 模式）。
func WithDialTimeout(d time.Duration) ClientOption {
	return func(o *core.ClientOptions) {
		o.DialTimeout = d
	}
}

// WithReconnectPolicy 设置断线重连的指数退避策略（socket 模式）。
func WithReconnectPolicy(p ReconnectPolicy) ClientOption {
	return func(o *core.ClientOptions) {
		o.Reconnect = p
	}
}

// WithDefaultTraceConfig 设置客户端默认的 TraceConfig。
//
// 单次调用的 WithTraceConfig / WithBatchTraceConfig 会覆盖它。
func WithDefaultTraceConfig(c *TraceConfig) ClientOption {
	return func(o *core.ClientOptions) {
		o.DefaultConfig = c
	}
}

// WithLogger 设置结构化日志器；默认静默。
func WithLogger(l *slog.Logger) ClientOption {
	return func(o *core.ClientOptions) {
		o.Logger = l
	}
}

// WithMaxInflight 设置在途任务上限（socket 模式信号量容量）；<=0 表示不限制。
func WithMaxInflight(n int) ClientOption {
	return func(o *core.ClientOptions) {
		o.MaxInflight = n
	}
}

// WithHealthCheck 设置主动健康检查间隔（socket 模式）；0 表示关闭。
//
// 注意：scamper 的 attach 模式不支持 get/set 等交互式命令，无法安全地做主动探测，
// 因此当前实现仅依赖读写错误做被动检测。该选项按设计文档保留，暂不改变行为。
func WithHealthCheck(d time.Duration) ClientOption {
	return func(o *core.ClientOptions) {
		o.HealthCheck = d
	}
}

// WithMetrics 设置指标接收器；默认 no-op。
func WithMetrics(sink MetricsSink) ClientOption {
	return func(o *core.ClientOptions) {
		o.Metrics = sink
	}
}
