package socket

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/kanechoo/scamper-client-go/internal/core"
	"github.com/kanechoo/scamper-client-go/internal/run"
)

// Client 是通过 scamper daemon control socket 执行测量的 Client 实现。
//
// 构造时（New）会立即建立首个连接以 fail-fast；之后由后台 supervisor 维持连接，
// 断线后按 ReconnectPolicy 自动重连。Client 可被多个 goroutine 并发使用。
type Client struct {
	sess *session
	opts core.ClientOptions
}

// New 构造一个 socket Client。
//
// 未设置 WithSocketPath 时使用 core.DefaultSocketPath。构造时会同步 Dial 并
// 完成 attach 握手；失败返回包装了 ErrDaemonUnavailable 的错误（attach 被 ERR
// 时返回 *CommandError）。
func New(options ...core.ClientOption) (*Client, error) {
	opts := core.DefaultClientOptions()
	core.ApplyClient(&opts, options)
	if opts.SocketPath == "" {
		opts.SocketPath = core.DefaultSocketPath
	}
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = 5 * time.Second
	}
	opts.Reconnect = opts.Reconnect.Normalized()
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	sess, err := newSession(opts)
	if err != nil {
		return nil, err
	}
	return &Client{sess: sess, opts: opts}, nil
}

// Trace 对单个目标做一次 traceroute。详见 root 包 Client 接口。
func (c *Client) Trace(ctx context.Context, target string, options ...core.TraceOption) (*core.TraceResult, error) {
	o := core.TraceOptions{Config: c.opts.DefaultConfig}
	core.ApplyTrace(&o, options)
	return run.Trace(ctx, target, o, c.once, c.opts.Metrics)
}

// Close 取消 supervisor、关闭连接并唤醒所有等待者。Close 后不应再调用 Trace。
func (c *Client) Close() error {
	return c.sess.Close()
}

// once 把任务交给当前连接；连接断开或超时由 session/run 处理。
func (c *Client) once(ctx context.Context, target string, o core.TraceOptions) (*core.TraceResult, error) {
	cfg := o.Config
	if cfg == nil {
		cfg = c.opts.DefaultConfig
	}
	argv, err := core.BuildArgv(cfg, target)
	if err != nil {
		return nil, err
	}
	return c.sess.trace(ctx, target, argv)
}
