package scamper

import (
	"errors"
	"testing"
	"time"

	"github.com/kanechoo/scamper-client-go/legacy"
	"github.com/kanechoo/scamper-client-go/socket"
)

// 编译期断言：两个实现都满足 root 包导出的 Client 接口。
var (
	_ Client = (*legacy.Client)(nil)
	_ Client = (*socket.Client)(nil)
)

func TestNewLegacyClientMissingBinary(t *testing.T) {
	_, err := NewLegacyClient(WithBinary("/nonexistent/scamper"))
	if !errors.Is(err, ErrNoScamperBinary) {
		t.Fatalf("NewLegacyClient() error = %v, want ErrNoScamperBinary", err)
	}
}

func TestNewSocketClientDialFailure(t *testing.T) {
	_, err := NewSocketClient(
		WithSocketPath("/nonexistent/scamper/dir/s.sock"),
		WithDialTimeout(300*time.Millisecond),
	)
	if !errors.Is(err, ErrDaemonUnavailable) {
		t.Fatalf("NewSocketClient() error = %v, want ErrDaemonUnavailable", err)
	}
}

func TestTraceConfigEnumsAliased(t *testing.T) {
	if ProtocolUDP != "udp" || OptionPTR != "ptr" || GapActionHalt != 1 {
		t.Fatal("enum aliases are not wired to core values")
	}
	if NewTraceConfig() == nil {
		t.Fatal("NewTraceConfig() = nil")
	}
}
