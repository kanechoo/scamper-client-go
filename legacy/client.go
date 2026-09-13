// Package legacy 提供通过本地 scamper CLI（exec）执行 traceroute 的 Client 实现。
//
// 它不需要 daemon：每次 Trace 启动一个 scamper 进程，以
// `scamper -O json -I "trace ..."` 运行并从 stdout 解析 JSON。批量测量使用一个
// 受限进程池（默认并发 8），因为每个目标都会启动一个进程。
//
// 普通用户应通过 root 包使用 NewLegacyClient，而非直接 import 本包。
package legacy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"

	"github.com/kanechoo/scamper-client-go/internal/core"
	"github.com/kanechoo/scamper-client-go/internal/parser"
	"github.com/kanechoo/scamper-client-go/internal/run"
)

// defaultConcurrency 是 legacy 批量测量的默认进程池大小。
//
// 每个在途目标对应一个 scamper 进程，开销远高于 socket 模式，因此默认值较小。
const defaultConcurrency = 8

// Client 是基于本地 scamper CLI 的 Client 实现。
type Client struct {
	bin    string
	opts   core.ClientOptions
	log    *slog.Logger
	closed atomic.Bool
}

// New 构造一个 legacy Client。
//
// 未设置 WithBinary 时会在 PATH 中查找 scamper；找不到或指定的路径不可执行时
// 返回包装了 ErrNoScamperBinary 的错误。
func New(options ...core.ClientOption) (*Client, error) {
	opts := core.DefaultClientOptions()
	core.ApplyClient(&opts, options)

	bin := opts.Binary
	if bin == "" {
		found, err := exec.LookPath("scamper")
		if err != nil {
			return nil, fmt.Errorf("%w: %v", core.ErrNoScamperBinary, err)
		}
		bin = found
	} else if err := checkExecutable(bin); err != nil {
		return nil, fmt.Errorf("%w: %v", core.ErrNoScamperBinary, err)
	}

	log := opts.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	log.Debug("scamper: legacy client initialized", "binary", bin)
	return &Client{bin: bin, opts: opts, log: log}, nil
}

// Trace 对单个目标做一次 traceroute。详见 root 包 Client 接口。
func (c *Client) Trace(ctx context.Context, target string, options ...core.TraceOption) (*core.TraceResult, error) {
	if c.closed.Load() {
		return nil, &core.ClientClosedError{}
	}
	o := core.TraceOptions{Config: c.opts.DefaultConfig}
	core.ApplyTrace(&o, options)
	return run.Trace(ctx, target, o, c.once, c.opts.Metrics)
}

// TraceBatch 并发测量多个目标（受限进程池）。详见 root 包 Client 接口。
func (c *Client) TraceBatch(ctx context.Context, targets []string, options ...core.BatchOption) ([]core.TraceResult, error) {
	if c.closed.Load() {
		return nil, &core.ClientClosedError{}
	}
	var b core.BatchOptions
	core.ApplyBatch(&b, options)
	if b.Metrics == nil {
		b.Metrics = c.opts.Metrics
	}
	base := core.TraceOptions{Config: c.opts.DefaultConfig}
	return run.Batch(ctx, targets, b, base, c.once, defaultConcurrency)
}

// Close 释放资源。legacy Client 无长连接，仅标记关闭。
func (c *Client) Close() error {
	c.closed.Store(true)
	return nil
}

// once 启动一次 scamper 进程并解析其 stdout。
func (c *Client) once(ctx context.Context, target string, o core.TraceOptions) (*core.TraceResult, error) {
	cfg := o.Config
	if cfg == nil {
		cfg = c.opts.DefaultConfig
	}
	argv, err := core.BuildArgv(cfg, target)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, c.bin, "-O", "json", "-I", strings.Join(argv, " "))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if cerr := ctx.Err(); cerr != nil {
		return nil, cerr
	}
	if runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = runErr.Error()
		}
		return nil, &core.CommandError{Args: argv, Message: msg}
	}

	res, err := parser.ParseTrace(stdout.Bytes(), target)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// checkExecutable 校验路径存在且具有可执行权限。
func checkExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%s is not executable", path)
	}
	return nil
}
