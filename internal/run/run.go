// Package run 是 legacy 与 socket 共用的执行层。
//
// 它负责单目标的超时与重试、以及批量的 worker pool、有界队列、背压、顺序回填、
// 进度回调与 stop-on-error。两种模式只需注入各自的 OnceFunc（一次尝试）。
package run

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kanechoo/scamper-client-go/internal/core"
)

// OnceFunc 执行一次测量尝试，不做超时/重试/元数据合并。
//
// 实现应尊重 ctx：ctx 取消或超时时尽快返回，并返回 ctx.Err() 以便上层判定。
type OnceFunc func(ctx context.Context, target string, o core.TraceOptions) (*core.TraceResult, error)

// Trace 执行单目标测量，含超时、重试与元数据合并。
//
// 返回的 TraceResult 始终非 nil；失败时 result.Error 与返回的 error 相同。
func Trace(parent context.Context, target string, o core.TraceOptions, once OnceFunc, sink core.MetricsSink) (*core.TraceResult, error) {
	attempts := o.Retry + 1
	if o.Retry < 0 || attempts < 1 {
		// 负数或溢出的重试次数一律按“只尝试一次”处理，避免循环被跳过而
		// 返回一个看似成功、实则从未执行的空结果。
		attempts = 1
	}

	var lastErr error
	var lastRes *core.TraceResult

	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 && !sleepCtx(parent, retryBackoff(attempt)) {
			break
		}

		callCtx := parent
		var cancel context.CancelFunc
		if o.Timeout > 0 {
			callCtx, cancel = context.WithTimeout(parent, o.Timeout)
		}

		start := time.Now()
		res, err := once(callCtx, target, o)
		if cancel != nil {
			cancel()
		}

		if res == nil {
			res = &core.TraceResult{Target: target}
		}
		if res.Target == "" {
			res.Target = target
		}
		if err == nil && res.Error != nil {
			err = res.Error
		}
		res.Duration = time.Since(start)
		res.Metadata = core.MergeMetadata(res.Metadata, o.Metadata)

		if err == nil {
			if sink != nil {
				sink.OnResult(target, res.Duration, nil)
			}
			return res, nil
		}

		res.Error = err
		lastErr, lastRes = err, res

		if !core.IsRetryable(err) || parent.Err() != nil {
			break
		}
	}

	if errors.Is(lastErr, context.DeadlineExceeded) && o.Timeout > 0 {
		lastErr = &core.TimeoutError{Target: target, Timeout: o.Timeout}
	}
	if lastRes == nil {
		lastRes = &core.TraceResult{Target: target}
	}
	lastRes.Error = lastErr
	if sink != nil {
		sink.OnResult(target, lastRes.Duration, lastErr)
	}
	return lastRes, lastErr
}

// Batch 并发测量多个目标。
//
// 返回切片与 targets 等长且顺序一致；单目标错误写入对应元素的 Error，不影响
// 其它目标。仅当共享配置非法时才返回非 nil 的顶层 error。
func Batch(ctx context.Context, targets []string, b core.BatchOptions, base core.TraceOptions, once OnceFunc, defaultConcurrency int) ([]core.TraceResult, error) {
	results := make([]core.TraceResult, len(targets))
	for i := range results {
		results[i] = core.TraceResult{Target: targets[i], Error: core.ErrBatchStopped}
	}
	if len(targets) == 0 {
		return results, nil
	}

	conc := b.Concurrency
	if conc <= 0 {
		conc = defaultConcurrency
	}
	if conc <= 0 {
		conc = 1
	}
	if conc > len(targets) {
		conc = len(targets)
	}
	// 队列容量 = max(QueueSize, concurrency)，保证提交端与消费端不互相饿死。
	qsize := b.QueueSize
	if qsize < conc {
		qsize = conc
	}

	to := base
	if b.Config != nil {
		to.Config = b.Config
	}
	if b.Timeout > 0 {
		to.Timeout = b.Timeout
	}
	to.Retry = b.Retry
	to.Metadata = core.MergeMetadata(base.Metadata, b.Metadata)

	if to.Config != nil {
		if err := to.Config.Validate(); err != nil {
			return nil, err
		}
	}

	batchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan int, qsize)
	var wg sync.WaitGroup
	var completed atomic.Int64
	var stopOnce sync.Once
	total := len(targets)

	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				res, err := Trace(batchCtx, targets[idx], to, once, b.Metrics)
				if res == nil {
					res = &core.TraceResult{Target: targets[idx]}
				}
				if err != nil {
					res.Error = err
				}
				results[idx] = *res

				n := int(completed.Add(1))
				if b.Progress != nil {
					b.Progress(n, total)
				}
				if err != nil && b.StopOnError && !core.IsRetryable(err) {
					stopOnce.Do(cancel)
				}
			}
		}()
	}

submit:
	for i := range targets {
		if b.Metrics != nil {
			b.Metrics.OnSubmit(targets[i])
		}
		if b.DropOnBackpressure {
			select {
			case jobs <- i:
			case <-batchCtx.Done():
				break submit
			default:
				results[i] = core.TraceResult{Target: targets[i], Error: core.ErrBatchStopped}
				n := int(completed.Add(1))
				if b.Progress != nil {
					b.Progress(n, total)
				}
			}
			if b.Metrics != nil {
				b.Metrics.OnQueueDepth(len(jobs))
			}
			continue
		}
		select {
		case jobs <- i:
			if b.Metrics != nil {
				b.Metrics.OnQueueDepth(len(jobs))
			}
		case <-batchCtx.Done():
			break submit
		}
	}
	close(jobs)
	wg.Wait()

	for i := range results {
		if errors.Is(results[i].Error, core.ErrBatchStopped) && ctx.Err() != nil {
			results[i].Error = ctx.Err()
		}
	}
	return results, nil
}

// sleepCtx 睡眠 d，可被 ctx 取消；返回 false 表示 ctx 已结束。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// retryBackoff 计算第 attempt 次重试前的退避时长（指数 + ±20% 抖动，5s 封顶）。
func retryBackoff(attempt int) time.Duration {
	const base = 200 * time.Millisecond
	d := base << (attempt - 1)
	if d <= 0 || d > 5*time.Second {
		d = 5 * time.Second
	}
	factor := 80 + rand.IntN(41) // 80..120
	return d * time.Duration(factor) / 100
}
