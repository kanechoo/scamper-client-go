package scamper

import (
	"time"

	"github.com/kanechoo/scamper-client-go/internal/core"
)

// TraceOption 是单次 Trace 调用的配置项。
type TraceOption = core.TraceOption

// BatchOption 是 TraceBatch 调用的配置项。
type BatchOption = core.BatchOption

// WithTraceConfig 为单次 Trace 设置 TraceConfig，覆盖客户端默认配置。
func WithTraceConfig(c *TraceConfig) TraceOption {
	return func(o *core.TraceOptions) {
		o.Config = c
	}
}

// WithTimeout 为单次 Trace 设置单目标超时；超时会中止该目标并返回 TimeoutError。
func WithTimeout(d time.Duration) TraceOption {
	return func(o *core.TraceOptions) {
		o.Timeout = d
	}
}

// WithRetry 为单次 Trace 设置可重试错误的重试次数；0 表示不重试。
func WithRetry(n int) TraceOption {
	return func(o *core.TraceOptions) {
		o.Retry = n
	}
}

// WithMetadata 为单次 Trace 透传附加信息，便于调用方关联业务。
//
// 用户键会合并进结果的 Metadata，冲突时优先于 SDK 附加的键。
func WithMetadata(m Metadata) TraceOption {
	return func(o *core.TraceOptions) {
		o.Metadata = m
	}
}

// WithConcurrency 设置批量测量的并发上限（socket 模式生效；legacy 为进程池大小）。
func WithConcurrency(n int) BatchOption {
	return func(o *core.BatchOptions) {
		o.Concurrency = n
	}
}

// WithBatchTimeout 设置批量中每个目标的超时。
func WithBatchTimeout(d time.Duration) BatchOption {
	return func(o *core.BatchOptions) {
		o.Timeout = d
	}
}

// WithBatchRetry 设置批量中每个目标独立的重试次数。
func WithBatchRetry(n int) BatchOption {
	return func(o *core.BatchOptions) {
		o.Retry = n
	}
}

// WithBatchTraceConfig 为批量测量设置 TraceConfig，覆盖客户端默认配置。
func WithBatchTraceConfig(c *TraceConfig) BatchOption {
	return func(o *core.BatchOptions) {
		o.Config = c
	}
}

// WithBatchMetadata 为批量测量透传附加信息。
func WithBatchMetadata(m Metadata) BatchOption {
	return func(o *core.BatchOptions) {
		o.Metadata = m
	}
}

// WithStopOnError 设置首个不可重试错误是否中止剩余任务；默认 false。
func WithStopOnError(stop bool) BatchOption {
	return func(o *core.BatchOptions) {
		o.StopOnError = stop
	}
}

// WithProgress 设置结果落地时的进度回调；可能被并发调用。
func WithProgress(fn func(done, total int)) BatchOption {
	return func(o *core.BatchOptions) {
		o.Progress = fn
	}
}

// WithDropOnBackpressure 设置队列满时快速失败（对应元素返回 ErrBatchStopped）
// 而不是阻塞提交；默认 false（阻塞，形成背压）。
func WithDropOnBackpressure(drop bool) BatchOption {
	return func(o *core.BatchOptions) {
		o.DropOnBackpressure = drop
	}
}

// WithQueueSize 设置调度队列容量；实际容量为 max(n, concurrency)。
func WithQueueSize(n int) BatchOption {
	return func(o *core.BatchOptions) {
		o.QueueSize = n
	}
}
