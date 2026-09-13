package legacy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kanechoo/scamper-client-go/internal/core"
)

const fakeTraceJSON = `{"type":"trace","version":"0.1","method":"udp","src":"127.0.0.1","dst":"1.1.1.1","stop_reason":"COMPLETED","hop_count":1,"probe_count":1,"hops":[{"addr":"1.1.1.1","rtt":0.5,"reply_ttl":64,"probe_ttl":1}]}`

func withBinary(path string) core.ClientOption {
	return func(o *core.ClientOptions) { o.Binary = path }
}

func withTraceConfig(c *core.TraceConfig) core.TraceOption {
	return func(o *core.TraceOptions) { o.Config = c }
}

func withConcurrency(n int) core.BatchOption {
	return func(o *core.BatchOptions) { o.Concurrency = n }
}

// writeFakeScamper 在临时目录写入一个可执行的假 scamper 脚本。
func writeFakeScamper(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "scamper")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake scamper: %v", err)
	}
	return path
}

func TestNewMissingBinary(t *testing.T) {
	_, err := New(withBinary("/nonexistent/scamper"))
	if !errors.Is(err, core.ErrNoScamperBinary) {
		t.Fatalf("New() error = %v, want ErrNoScamperBinary", err)
	}
}

func TestTraceAssemblesArgvAndParses(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"$ARGS_FILE\"\n" +
		"printf '%s\\n' '" + fakeTraceJSON + "'\n"
	bin := writeFakeScamper(t, script)
	t.Setenv("ARGS_FILE", argsFile)

	c, err := New(withBinary(bin))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cfg := core.NewTraceConfig().Protocol(core.ProtocolUDP).MaxHops(5)
	res, err := c.Trace(context.Background(), "1.1.1.1", withTraceConfig(cfg))
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if res.Address != "1.1.1.1" || len(res.Hops) != 1 {
		t.Fatalf("res = %+v", res)
	}

	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	got := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	want := []string{"-O", "json", "-I", "trace -P udp -m 5 1.1.1.1"}
	if len(got) != len(want) {
		t.Fatalf("args = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q (all: %q)", i, got[i], want[i], got)
		}
	}
}

func TestTraceCommandError(t *testing.T) {
	script := "#!/bin/sh\necho 'ERR command not accepted' >&2\nexit 1\n"
	bin := writeFakeScamper(t, script)

	c, err := New(withBinary(bin))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = c.Trace(context.Background(), "1.1.1.1")
	if !errors.Is(err, core.ErrCommand) {
		t.Fatalf("Trace() error = %v, want ErrCommand", err)
	}
	var ce *core.CommandError
	if !errors.As(err, &ce) || !strings.Contains(ce.Message, "command not accepted") {
		t.Fatalf("CommandError = %+v", ce)
	}
}

func TestTraceInvalidConfig(t *testing.T) {
	script := "#!/bin/sh\nprintf '%s\\n' '" + fakeTraceJSON + "'\n"
	bin := writeFakeScamper(t, script)
	c, _ := New(withBinary(bin))

	_, err := c.Trace(context.Background(), "1.1.1.1", withTraceConfig(core.NewTraceConfig().FirstHop(0)))
	if !errors.Is(err, core.ErrInvalidConfig) {
		t.Fatalf("Trace() error = %v, want ErrInvalidConfig", err)
	}
}

func TestTraceBatch(t *testing.T) {
	script := "#!/bin/sh\nprintf '%s\\n' '" + fakeTraceJSON + "'\n"
	bin := writeFakeScamper(t, script)
	c, _ := New(withBinary(bin))

	targets := []string{"1.1.1.1", "8.8.8.8", "9.9.9.9"}
	results, err := c.TraceBatch(context.Background(), targets, withConcurrency(2))
	if err != nil {
		t.Fatalf("TraceBatch() error = %v", err)
	}
	if len(results) != len(targets) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(targets))
	}
	for i, r := range results {
		if r.Target != targets[i] {
			t.Errorf("results[%d].Target = %q, want %q", i, r.Target, targets[i])
		}
		if r.Error != nil {
			t.Errorf("results[%d].Error = %v", i, r.Error)
		}
	}
}

func TestClose(t *testing.T) {
	script := "#!/bin/sh\nprintf '%s\\n' '" + fakeTraceJSON + "'\n"
	bin := writeFakeScamper(t, script)
	c, _ := New(withBinary(bin))
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := c.Trace(context.Background(), "1.1.1.1")
	if !errors.Is(err, core.ErrClosed) {
		t.Fatalf("Trace() after Close error = %v, want ErrClosed", err)
	}
}
