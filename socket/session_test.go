package socket

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kanechoo/scamper-client-go/internal/core"
)

// fakeServer 是用 Unix domain socket 模拟 scamper daemon 的测试服务端。
type fakeServer struct {
	t    *testing.T
	path string
	ln   net.Listener

	attachERR        string
	traceERR         string
	haltERR          bool
	holdHaltERR      bool
	extraNoID        []byte
	rawTraceResponse string
	closeAfterData   bool
	handleTrace      func(target string, id uint64) []byte
	delay            func(target string) time.Duration

	haltCh chan uint64
}

func newFakeServer(t *testing.T, configure func(*fakeServer)) *fakeServer {
	t.Helper()
	dir, err := os.MkdirTemp("", "scamper")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	path := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		_ = os.RemoveAll(dir)
		t.Fatalf("listen unix: %v", err)
	}
	s := &fakeServer{t: t, path: path, ln: ln, haltCh: make(chan uint64, 256)}
	if configure != nil {
		configure(s)
	}
	go s.acceptLoop()
	t.Cleanup(func() {
		_ = ln.Close()
		_ = os.RemoveAll(dir)
	})
	return s
}

func (s *fakeServer) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *fakeServer) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	br := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	var wmu sync.Mutex

	// 与真实 scamper 一致：DATA 负载后不追加换行，下一帧头紧随其后。
	writeNoID := func(p []byte) {
		wmu.Lock()
		defer wmu.Unlock()
		fmt.Fprintf(w, "DATA %d\n", len(p))
		_, _ = w.Write(p)
		_ = w.Flush()
	}
	writeData := func(id uint64, p []byte) {
		wmu.Lock()
		defer wmu.Unlock()
		fmt.Fprintf(w, "DATA %d id-%d\n", len(p), id)
		_, _ = w.Write(p)
		_ = w.Flush()
	}

	line, err := br.ReadString('\n')
	if err != nil {
		return
	}
	if strings.TrimSpace(line) != "attach format json" {
		s.t.Errorf("unexpected attach line: %q", line)
		return
	}
	if s.attachERR != "" {
		fmt.Fprintf(w, "ERR %s\n", s.attachERR)
		_ = w.Flush()
		return
	}
	fmt.Fprint(w, "MORE\n")
	fmt.Fprint(w, "OK\n")
	_ = w.Flush()

	var nextID uint64
	var pendingHaltERR uint64
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		fields := strings.Fields(line)

		switch fields[0] {
		case "halt":
			if len(fields) >= 2 {
				var id uint64
				_, _ = fmt.Sscanf(fields[1], "%d", &id)
				select {
				case s.haltCh <- id:
				default:
				}
				switch {
				case s.haltERR && s.holdHaltERR:
					// 延迟到下一个 trace 命令前再回复，构造 ERR 先于 trace OK
					// 到达的交错场景。
					pendingHaltERR = id
				case s.haltERR:
					fmt.Fprintf(w, "ERR no task id-%d\n", id)
					_ = w.Flush()
				default:
					fmt.Fprintf(w, "OK halted %d\n", id)
					_ = w.Flush()
				}
			}
		case "trace":
			target := fields[len(fields)-1]
			if s.traceERR != "" {
				fmt.Fprintf(w, "ERR %s\n", s.traceERR)
				_ = w.Flush()
				continue
			}
			if pendingHaltERR != 0 {
				fmt.Fprintf(w, "ERR no task id-%d\n", pendingHaltERR)
				pendingHaltERR = 0
			}
			nextID++
			id := nextID
			fmt.Fprintf(w, "OK id-%d\n", id)
			_ = w.Flush()

			if len(s.extraNoID) > 0 {
				writeNoID(s.extraNoID)
			}
			if s.rawTraceResponse != "" {
				wmu.Lock()
				_, _ = w.WriteString(s.rawTraceResponse)
				_ = w.Flush()
				wmu.Unlock()
				continue
			}
			if s.handleTrace != nil {
				payload := s.handleTrace(target, id)
				if payload != nil {
					d := time.Duration(0)
					if s.delay != nil {
						d = s.delay(target)
					}
					if d > 0 {
						go func(id uint64, p []byte, d time.Duration) {
							time.Sleep(d)
							writeData(id, p)
						}(id, payload, d)
					} else {
						writeData(id, payload)
					}
				}
			}
			if s.closeAfterData {
				return
			}
		}
	}
}

func tracePayload(target string, ttl int) []byte {
	return []byte(fmt.Sprintf(
		`{"type":"trace","version":"0.1","method":"udp","dst":%q,"stop_reason":"COMPLETED","hop_count":1,"probe_count":1,"hops":[{"addr":%q,"rtt":1.5,"reply_ttl":64,"probe_ttl":%d}]}`,
		target, target, ttl))
}

func withSocketPath(p string) core.ClientOption {
	return func(o *core.ClientOptions) { o.SocketPath = p }
}

func withDialTimeout(d time.Duration) core.ClientOption {
	return func(o *core.ClientOptions) { o.DialTimeout = d }
}

func withReconnect(p core.ReconnectPolicy) core.ClientOption {
	return func(o *core.ClientOptions) { o.Reconnect = p }
}

func withTraceTimeout(d time.Duration) core.TraceOption {
	return func(o *core.TraceOptions) { o.Timeout = d }
}

func withTraceRetry(n int) core.TraceOption {
	return func(o *core.TraceOptions) { o.Retry = n }
}

func withTraceConfig(c *core.TraceConfig) core.TraceOption {
	return func(o *core.TraceOptions) { o.Config = c }
}

func withConcurrency(n int) core.BatchOption {
	return func(o *core.BatchOptions) { o.Concurrency = n }
}

func TestSocketTrace(t *testing.T) {
	srv := newFakeServer(t, func(s *fakeServer) {
		s.handleTrace = func(target string, id uint64) []byte { return tracePayload(target, 1) }
	})
	c, err := New(withSocketPath(srv.path))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	res, err := c.Trace(context.Background(), "1.1.1.1")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if res.Address != "1.1.1.1" || res.Method != "udp" || res.StopReason != "COMPLETED" {
		t.Fatalf("res = %+v", res)
	}
	if len(res.Hops) != 1 || res.Hops[0].Address != "1.1.1.1" {
		t.Fatalf("Hops = %+v", res.Hops)
	}
}

func TestSocketIgnoresNoIDData(t *testing.T) {
	srv := newFakeServer(t, func(s *fakeServer) {
		s.extraNoID = []byte(`{"type":"cycle-start"}`)
		s.handleTrace = func(target string, id uint64) []byte { return tracePayload(target, 1) }
	})
	c, err := New(withSocketPath(srv.path))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	res, err := c.Trace(context.Background(), "1.1.1.1")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if res.StopReason != "COMPLETED" {
		t.Fatalf("res = %+v", res)
	}
}

func TestSocketCommandError(t *testing.T) {
	srv := newFakeServer(t, func(s *fakeServer) {
		s.traceERR = "invalid target"
	})
	c, err := New(withSocketPath(srv.path))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Trace(context.Background(), "1.1.1.1")
	if !errors.Is(err, core.ErrCommand) {
		t.Fatalf("Trace() error = %v, want ErrCommand", err)
	}
	if core.IsRetryable(err) {
		t.Fatal("CommandError should not be retryable")
	}
}

func TestSocketAttachError(t *testing.T) {
	srv := newFakeServer(t, func(s *fakeServer) {
		s.attachERR = "unsupported format"
	})
	_, err := New(withSocketPath(srv.path))
	if !errors.Is(err, core.ErrCommand) {
		t.Fatalf("New() error = %v, want ErrCommand", err)
	}
}

func TestSocketDialFailure(t *testing.T) {
	_, err := New(
		withSocketPath("/nonexistent/scamper/dir/s.sock"),
		withDialTimeout(500*time.Millisecond),
	)
	if !errors.Is(err, core.ErrDaemonUnavailable) {
		t.Fatalf("New() error = %v, want ErrDaemonUnavailable", err)
	}
}

func TestSocketProtocolErrorIsRetryable(t *testing.T) {
	srv := newFakeServer(t, func(s *fakeServer) {
		s.rawTraceResponse = "DATA notanumber id-1\n"
	})
	c, err := New(withSocketPath(srv.path))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Trace(context.Background(), "1.1.1.1")
	if !errors.Is(err, core.ErrProtocol) {
		t.Fatalf("Trace() error = %v, want ErrProtocol", err)
	}
	if !core.IsRetryable(err) {
		t.Fatal("protocol error should be retryable")
	}
}

func TestSocketTimeoutSendsHalt(t *testing.T) {
	srv := newFakeServer(t, func(s *fakeServer) {
		s.handleTrace = nil // 永不返回结果
	})
	c, err := New(withSocketPath(srv.path))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Trace(context.Background(), "1.1.1.1", withTraceTimeout(60*time.Millisecond))
	var te *core.TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("Trace() error = %v, want *TimeoutError", err)
	}

	select {
	case id := <-srv.haltCh:
		if id != 1 {
			t.Fatalf("halt id = %d, want 1", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not receive halt")
	}
}

// TestSocketHaltErrorDoesNotCorruptPendingTrace 验证 halt 被 daemon 拒绝（ERR）
// 时，不会把这个 ERR 误配给随后提交的 trace 任务。
func TestSocketHaltErrorDoesNotCorruptPendingTrace(t *testing.T) {
	srv := newFakeServer(t, func(s *fakeServer) {
		s.haltERR = true
		s.holdHaltERR = true
		s.handleTrace = func(target string, id uint64) []byte {
			if target == "8.8.8.8" {
				return tracePayload(target, 1)
			}
			return nil // 1.1.1.1 永不响应，触发超时与 halt
		}
	})
	c, err := New(withSocketPath(srv.path))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Trace(context.Background(), "1.1.1.1", withTraceTimeout(50*time.Millisecond))
	var te *core.TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("first Trace() error = %v, want *TimeoutError", err)
	}
	select {
	case <-srv.haltCh:
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not receive halt")
	}

	res, err := c.Trace(context.Background(), "8.8.8.8", withTraceTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("second Trace() error = %v, want success", err)
	}
	if res.Address != "8.8.8.8" {
		t.Fatalf("second Trace() address = %q", res.Address)
	}
}

func TestSocketOutOfOrderResults(t *testing.T) {
	const n = 20
	targets := make([]string, n)
	for i := range targets {
		targets[i] = fmt.Sprintf("10.0.0.%d", i+1)
	}
	// 越靠后的命令延迟越小，使结果乱序返回。
	srv := newFakeServer(t, func(s *fakeServer) {
		s.handleTrace = func(target string, id uint64) []byte { return tracePayload(target, 1) }
		s.delay = func(target string) time.Duration {
			var last int
			_, _ = fmt.Sscanf(target, "10.0.0.%d", &last)
			return time.Duration(n-last) * 5 * time.Millisecond
		}
	})
	c, err := New(withSocketPath(srv.path))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	results, err := c.TraceBatch(context.Background(), targets, withConcurrency(n))
	if err != nil {
		t.Fatalf("TraceBatch() error = %v", err)
	}
	if len(results) != n {
		t.Fatalf("len(results) = %d, want %d", len(results), n)
	}
	for i, r := range results {
		if r.Target != targets[i] {
			t.Fatalf("results[%d].Target = %q, want %q", i, r.Target, targets[i])
		}
		if r.Error != nil {
			t.Fatalf("results[%d].Error = %v", i, r.Error)
		}
	}
}

func TestSocketBatch500(t *testing.T) {
	const n = 500
	targets := make([]string, n)
	for i := range targets {
		targets[i] = fmt.Sprintf("10.%d.%d.%d", i/65536%256, i/256%256, i%256)
	}
	srv := newFakeServer(t, func(s *fakeServer) {
		s.handleTrace = func(target string, id uint64) []byte { return tracePayload(target, 1) }
	})
	c, err := New(withSocketPath(srv.path))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	results, err := c.TraceBatch(context.Background(), targets, withConcurrency(64))
	if err != nil {
		t.Fatalf("TraceBatch() error = %v", err)
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
			t.Fatalf("duplicate result %q", r.Target)
		}
		seen[r.Target] = true
	}
}

func TestSocketReconnect(t *testing.T) {
	srv := newFakeServer(t, func(s *fakeServer) {
		s.handleTrace = func(target string, id uint64) []byte { return tracePayload(target, 1) }
		s.closeAfterData = true
	})
	c, err := New(
		withSocketPath(srv.path),
		withReconnect(core.ReconnectPolicy{MinBackoff: 10 * time.Millisecond, MaxBackoff: 50 * time.Millisecond, Multiplier: 1}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if _, err := c.Trace(context.Background(), "1.1.1.1"); err != nil {
		t.Fatalf("first Trace() error = %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		res, err := c.Trace(context.Background(), "8.8.8.8", withTraceRetry(3))
		if err == nil && res.Address == "8.8.8.8" {
			return
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("second Trace() did not recover after reconnect: %v", lastErr)
}

func TestSocketInvalidConfig(t *testing.T) {
	srv := newFakeServer(t, func(s *fakeServer) {
		s.handleTrace = func(target string, id uint64) []byte { return tracePayload(target, 1) }
	})
	c, err := New(withSocketPath(srv.path))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Trace(context.Background(), "1.1.1.1", withTraceConfig(core.NewTraceConfig().FirstHop(0)))
	if !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("Trace() error = %v, want ErrInvalidConfig", err)
	}
}

func TestSocketClose(t *testing.T) {
	srv := newFakeServer(t, nil)
	c, err := New(withSocketPath(srv.path))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = c.Trace(context.Background(), "1.1.1.1")
	if !errors.Is(err, core.ErrClosed) {
		t.Fatalf("Trace() after Close error = %v, want ErrClosed", err)
	}
}
