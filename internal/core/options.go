package core

import "time"

// TraceOptions 是单次 Trace 调用的有效选项集合。
//
// 用户通过 root 包的 TraceOption 构造，实现包在调用前读取这里的字段。
type TraceOptions struct {
	// Config 覆盖客户端默认的 TraceConfig；nil 表示沿用默认。
	Config *TraceConfig
	// Timeout 是单目标超时；<=0 表示不额外施加超时（仅受 ctx 约束）。
	Timeout time.Duration
	// Retry 是单目标的可重试错误重试次数；0 表示不重试。
	Retry int
	// Metadata 是用户透传的附加信息。
	Metadata Metadata
}

// TraceOption 是单次 Trace 的配置项。
//
// 由于底层参数类型属于 internal，用户无法自行实现该函数类型，只能使用 root
// 包导出的构造函数（WithTraceConfig、WithTimeout 等）。
type TraceOption func(*TraceOptions)

// ApplyTrace 按顺序把 option 应用到 options 上。
func ApplyTrace(o *TraceOptions, options []TraceOption) {
	for _, opt := range options {
		if opt != nil {
			opt(o)
		}
	}
}

// BatchOptions 是 TraceBatch 调用的有效选项集合。
type BatchOptions struct {
	// Concurrency 是并发上限；<=0 表示使用实现包的默认值。
	Concurrency int
	// Timeout 是单目标超时；<=0 表示不额外施加超时。
	Timeout time.Duration
	// Retry 是单目标的可重试错误重试次数；0 表示不重试。
	Retry int
	// Config 覆盖客户端默认的 TraceConfig；nil 表示沿用默认。
	Config *TraceConfig
	// Metadata 是用户透传的附加信息。
	Metadata Metadata
	// StopOnError 为 true 时，首个不可重试错误会中止剩余任务。
	StopOnError bool
	// Progress 在结果落地时回调；可能被并发调用，实现需线程安全。
	Progress func(done, total int)
	// QueueSize 是调度队列容量；实际容量为 max(QueueSize, concurrency)。
	QueueSize int
	// DropOnBackpressure 为 true 时队列满会快速失败而不是阻塞提交。
	DropOnBackpressure bool
	// Metrics 是可选指标接收器。
	Metrics MetricsSink
}

// BatchOption 是 TraceBatch 的配置项。
type BatchOption func(*BatchOptions)

// ApplyBatch 按顺序把 option 应用到 options 上。
func ApplyBatch(o *BatchOptions, options []BatchOption) {
	for _, opt := range options {
		if opt != nil {
			opt(o)
		}
	}
}

// ClientOption 是客户端构造选项。
type ClientOption func(*ClientOptions)

// ApplyClient 按顺序把 option 应用到 options 上。
func ApplyClient(o *ClientOptions, options []ClientOption) {
	for _, opt := range options {
		if opt != nil {
			opt(o)
		}
	}
}
