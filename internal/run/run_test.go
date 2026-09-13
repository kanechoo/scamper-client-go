package run

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kanechoo/scamper-client-go/internal/core"
)

func TestTraceSuccessMergesMetadata(t *testing.T) {
	once := func(_ context.Context, target string, _ core.TraceOptions) (*core.TraceResult, error) {
		return &core.TraceResult{
			Target:   target,
			Address:  target,
			Metadata: core.Metadata{"sdk": true},
		}, nil
	}
	o := core.TraceOptions{Metadata: core.Metadata{"user": 42}}
	res, err := Trace(context.Background(), "1.1.1.1", o, once, nil)
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if res.Target != "1.1.1.1" || res.Address != "1.1.1.1" {
		t.Fatalf("res = %+v", res)
	}
	if res.Metadata["sdk"] != true || res.Metadata["user"] != 42 {
		t.Fatalf("Metadata = %v", res.Metadata)
	}
	if res.Duration <= 0 {
		t.Fatalf("Duration = %v, want > 0", res.Duration)
	}
}

func TestTraceRetriesRetryable(t *testing.T) {
	var calls int32
	once := func(_ context.Context, target string, _ core.TraceOptions) (*core.TraceResult, error) {
		if atomic.AddInt32(&calls, 1) < 3 {
			return nil, core.ErrDaemonUnavailable
		}
		return &core.TraceResult{Target: target}, nil
	}
	res, err := Trace(context.Background(), "1.1.1.1", core.TraceOptions{Retry: 2}, once, nil)
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if res.Error != nil {
		t.Fatalf("res.Error = %v", res.Error)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("calls = %d, want 3", got)
	}
}

func TestTraceDoesNotRetryCommandError(t *testing.T) {
	var calls int32
	once := func(_ context.Context, _ string, _ core.TraceOptions) (*core.TraceResult, error) {
		atomic.AddInt32(&calls, 1)
		return nil, &core.CommandError{Message: "bad args"}
	}
	res, err := Trace(context.Background(), "1.1.1.1", core.TraceOptions{Retry: 5}, once, nil)
	if !errors.Is(err, core.ErrCommand) {
		t.Fatalf("error = %v, want ErrCommand", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
	if res == nil || res.Error == nil {
		t.Fatalf("res = %+v, want non-nil Error", res)
	}
	if core.IsRetryable(err) {
		t.Fatal("CommandError should not be retryable")
	}
}

func TestTraceTimeout(t *testing.T) {
	once := func(ctx context.Context, _ string, _ core.TraceOptions) (*core.TraceResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	_, err := Trace(context.Background(), "1.1.1.1", core.TraceOptions{Timeout: 40 * time.Millisecond}, once, nil)
	var te *core.TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("error = %v, want *TimeoutError", err)
	}
	if te.Timeout != 40*time.Millisecond {
		t.Fatalf("te.Timeout = %v", te.Timeout)
	}
	if !core.IsRetryable(err) {
		t.Fatal("TimeoutError should be retryable")
	}
}

func TestTraceElevatesResultError(t *testing.T) {
	once := func(_ context.Context, target string, _ core.TraceOptions) (*core.TraceResult, error) {
		return &core.TraceResult{Target: target, Error: errors.New("measurement failed")}, nil
	}
	_, err := Trace(context.Background(), "1.1.1.1", core.TraceOptions{}, once, nil)
	if err == nil || err.Error() != "measurement failed" {
		t.Fatalf("error = %v", err)
	}
}

func TestBatchPreservesOrderAndCount(t *testing.T) {
	const n = 500
	targets := make([]string, n)
	for i := range targets {
		targets[i] = string(rune('a'+i%26)) + "-" + itoa(i)
	}
	once := func(_ context.Context, target string, _ core.TraceOptions) (*core.TraceResult, error) {
		return &core.TraceResult{Target: target, Address: target}, nil
	}
	results, err := Batch(context.Background(), targets, core.BatchOptions{Concurrency: 64}, core.TraceOptions{}, once, 8)
	if err != nil {
		t.Fatalf("Batch() error = %v", err)
	}
	if len(results) != n {
		t.Fatalf("len(results) = %d, want %d", len(results), n)
	}
	seen := make(map[string]bool, n)
	for i, r := range results {
		if r.Target != targets[i] {
			t.Fatalf("results[%d].Target = %q, want %q", i, r.Target, targets[i])
		}
		if r.Error != nil {
			t.Fatalf("results[%d].Error = %v", i, r.Error)
		}
		if seen[r.Target] {
			t.Fatalf("duplicate result for %q", r.Target)
		}
		seen[r.Target] = true
	}
}

func TestBatchProgress(t *testing.T) {
	const n = 100
	targets := make([]string, n)
	for i := range targets {
		targets[i] = itoa(i)
	}
	once := func(_ context.Context, target string, _ core.TraceOptions) (*core.TraceResult, error) {
		return &core.TraceResult{Target: target}, nil
	}
	var mu sync.Mutex
	seen := make(map[int]bool, n)
	totalSeen := 0
	progress := func(done, total int) {
		mu.Lock()
		defer mu.Unlock()
		if total != n {
			t.Errorf("progress total = %d, want %d", total, n)
		}
		if done < 1 || done > n {
			t.Errorf("progress done = %d, out of range 1..%d", done, n)
		}
		if seen[done] {
			t.Errorf("duplicate progress done = %d", done)
		}
		seen[done] = true
		totalSeen++
	}
	_, err := Batch(context.Background(), targets, core.BatchOptions{Concurrency: 8, Progress: progress}, core.TraceOptions{}, once, 8)
	if err != nil {
		t.Fatal(err)
	}
	if totalSeen != n || len(seen) != n {
		t.Fatalf("progress calls = %d, unique = %d, want %d", totalSeen, len(seen), n)
	}
}

func TestBatchStopOnError(t *testing.T) {
	const n = 200
	targets := make([]string, n)
	for i := range targets {
		targets[i] = itoa(i)
	}
	once := func(_ context.Context, _ string, _ core.TraceOptions) (*core.TraceResult, error) {
		return nil, &core.CommandError{Message: "rejected"}
	}
	results, err := Batch(context.Background(), targets,
		core.BatchOptions{Concurrency: 4, StopOnError: true}, core.TraceOptions{}, once, 4)
	if err != nil {
		t.Fatalf("Batch() error = %v", err)
	}
	if len(results) != n {
		t.Fatalf("len(results) = %d, want %d", len(results), n)
	}
	for i, r := range results {
		if r.Error == nil {
			t.Fatalf("results[%d].Error = nil, want error", i)
		}
		if !errors.Is(r.Error, core.ErrCommand) && !errors.Is(r.Error, core.ErrBatchStopped) {
			t.Fatalf("results[%d].Error = %v", i, r.Error)
		}
	}
}

func TestBatchInvalidConfigReturnsTopLevelError(t *testing.T) {
	once := func(_ context.Context, target string, _ core.TraceOptions) (*core.TraceResult, error) {
		return &core.TraceResult{Target: target}, nil
	}
	_, err := Batch(context.Background(), []string{"1.1.1.1"},
		core.BatchOptions{Config: core.NewTraceConfig().FirstHop(0)}, core.TraceOptions{}, once, 1)
	if !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("Batch() error = %v, want ErrInvalidConfig", err)
	}
}

func TestBatchDropOnBackpressure(t *testing.T) {
	const n = 50
	targets := make([]string, n)
	for i := range targets {
		targets[i] = itoa(i)
	}

	release := make(chan struct{})
	started := make(chan struct{})
	var onceCalls int32
	once := func(_ context.Context, target string, _ core.TraceOptions) (*core.TraceResult, error) {
		if atomic.AddInt32(&onceCalls, 1) == 1 {
			close(started)
		}
		<-release
		return &core.TraceResult{Target: target}, nil
	}

	resultCh := make(chan []core.TraceResult, 1)
	go func() {
		results, _ := Batch(context.Background(), targets,
			core.BatchOptions{Concurrency: 1, QueueSize: 1, DropOnBackpressure: true},
			core.TraceOptions{}, once, 1)
		resultCh <- results
	}()

	<-started
	time.Sleep(30 * time.Millisecond)
	close(release)
	results := <-resultCh

	if len(results) != n {
		t.Fatalf("len(results) = %d, want %d", len(results), n)
	}
	dropped := 0
	for _, r := range results {
		if errors.Is(r.Error, core.ErrBatchStopped) {
			dropped++
		}
	}
	if dropped == 0 {
		t.Fatal("expected at least one dropped result under backpressure")
	}
}

func TestBatchEmpty(t *testing.T) {
	results, err := Batch(context.Background(), nil, core.BatchOptions{}, core.TraceOptions{}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("len(results) = %d, want 0", len(results))
	}
}

func TestTraceNegativeRetryRunsOnce(t *testing.T) {
	var calls int32
	once := func(_ context.Context, target string, _ core.TraceOptions) (*core.TraceResult, error) {
		atomic.AddInt32(&calls, 1)
		return &core.TraceResult{Target: target}, nil
	}
	res, err := Trace(context.Background(), "1.1.1.1", core.TraceOptions{Retry: -3}, once, nil)
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if res == nil || res.Error != nil {
		t.Fatalf("res = %+v, want success", res)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
}

// stubMetrics 记录指标回调次数，用于验证 MetricsSink 贯通到批量调度。
type stubMetrics struct {
	submits    int32
	results    int32
	reconnects int32
	depths     int32
}

func (m *stubMetrics) OnSubmit(string)                       { atomic.AddInt32(&m.submits, 1) }
func (m *stubMetrics) OnResult(string, time.Duration, error) { atomic.AddInt32(&m.results, 1) }
func (m *stubMetrics) OnReconnect()                          { atomic.AddInt32(&m.reconnects, 1) }
func (m *stubMetrics) OnQueueDepth(int)                      { atomic.AddInt32(&m.depths, 1) }

func TestBatchMetricsPropagated(t *testing.T) {
	const n = 20
	targets := make([]string, n)
	for i := range targets {
		targets[i] = itoa(i)
	}
	once := func(_ context.Context, target string, _ core.TraceOptions) (*core.TraceResult, error) {
		return &core.TraceResult{Target: target}, nil
	}
	m := &stubMetrics{}
	_, err := Batch(context.Background(), targets,
		core.BatchOptions{Concurrency: 4, Metrics: m}, core.TraceOptions{}, once, 4)
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&m.submits); got != n {
		t.Fatalf("OnSubmit = %d, want %d", got, n)
	}
	if got := atomic.LoadInt32(&m.results); got != n {
		t.Fatalf("OnResult = %d, want %d", got, n)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
