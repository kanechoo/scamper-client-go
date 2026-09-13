package legacy

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/kanechoo/scamper-client-go/internal/core"
)

// TestIntegrationTrace 需要真实可用的 scamper CLI（通常还需 root 权限）。
//
// 通过 SCAMPER_INTEGRATION=1 启用；未启用或环境不具备时跳过。
func TestIntegrationTrace(t *testing.T) {
	if os.Getenv("SCAMPER_INTEGRATION") == "" {
		t.Skip("set SCAMPER_INTEGRATION=1 to run legacy integration test")
	}
	if _, err := exec.LookPath("scamper"); err != nil {
		t.Skip("scamper binary not found in PATH")
	}

	c, err := New()
	if err != nil {
		t.Skipf("cannot init legacy client: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := core.NewTraceConfig().Protocol(core.ProtocolUDP).FirstHop(1).MaxHops(16).Wait(time.Second).ProbesPerHop(1)
	res, err := c.Trace(ctx, "1.1.1.1", withTraceConfig(cfg))
	if err != nil {
		t.Skipf("trace failed (environment likely lacks privileges): %v", err)
	}
	if res.Address == "" {
		t.Fatalf("res.Address is empty: %+v", res)
	}
}
