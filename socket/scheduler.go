package socket

import (
	"context"

	"github.com/kanechoo/scamper-client-go/internal/core"
	"github.com/kanechoo/scamper-client-go/internal/run"
)

// defaultConcurrency 是 socket 批量测量的默认并发（worker pool 大小）。
//
// 它只是本地提交上限；真正的吞吐受 daemon 的 -w（active task 窗口）与 -p（PPS）
// 约束。建议 WithConcurrency 取接近 daemon 窗口的值，过高不会有收益。
const defaultConcurrency = 16

// TraceBatch 并发测量多个目标。
//
// socket 模式下所有任务复用一个 Session：单 writer 顺序提交、reader 按 id 回填，
// worker pool + 有界队列 + inflight 信号量共同提供背压。返回切片与 targets
// 顺序一致；单目标失败不影响其它目标。
func (c *Client) TraceBatch(ctx context.Context, targets []string, options ...core.BatchOption) ([]core.TraceResult, error) {
	var b core.BatchOptions
	core.ApplyBatch(&b, options)
	if b.Metrics == nil {
		b.Metrics = c.opts.Metrics
	}
	base := core.TraceOptions{Config: c.opts.DefaultConfig}
	return run.Batch(ctx, targets, b, base, c.once, defaultConcurrency)
}
