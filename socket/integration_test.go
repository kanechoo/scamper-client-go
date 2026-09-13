package socket

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kanechoo/scamper-client-go/internal/core"
)

// TestIntegrationBatch 需要真实 scamper daemon 的 control socket。
//
// 通过 SCAMPER_SOCKET=/path/to/scamper.sock 启用；未设置时跳过。
// 运行 500 目标并发，验证无丢失、无重复、顺序正确。
func TestIntegrationBatch(t *testing.T) {
	sock := os.Getenv("SCAMPER_SOCKET")
	if sock == "" {
		t.Skip("set SCAMPER_SOCKET to run socket integration test")
	}

	c, err := New(withSocketPath(sock), withDialTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer c.Close()

	const n = 500
	targets := make([]string, n)
	for i := range targets {
		targets[i] = fmt.Sprintf("10.%d.%d.%d", i/65536%256, i/256%256, i%256)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cfg := core.NewTraceConfig().Protocol(core.ProtocolUDP).FirstHop(1).MaxHops(16).Wait(time.Second).ProbesPerHop(1)
	results, err := c.TraceBatch(ctx, targets,
		withConcurrency(64),
		func(o *core.BatchOptions) { o.Config = cfg; o.Timeout = 30 * time.Second; o.Retry = 1 },
	)
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
		if seen[r.Target] {
			t.Fatalf("duplicate result %q", r.Target)
		}
		seen[r.Target] = true
	}
}
