package scamper

import (
	"context"

	"github.com/kanechoo/scamper-client-go/internal/core"
	"github.com/kanechoo/scamper-client-go/legacy"
	"github.com/kanechoo/scamper-client-go/socket"
)

// DefaultSocketPath 是 scamper daemon control socket 的默认路径。
//
// daemon 实际监听的路径由用户启动 scamper 时的 -U 决定，可用 WithSocketPath
// 覆盖。
const DefaultSocketPath = core.DefaultSocketPath

// Client 是 scamper-client-go 的统一抽象，两种运行模式都实现该接口。
//
// 实现可被多个 goroutine 并发使用。Close 之后不应再调用 Trace/TraceBatch。
type Client interface {
	// Trace 对单个目标做一次 traceroute。
	//
	// options 覆盖 Client 默认配置（TraceConfig、超时、重试、元数据）。
	// 返回的 *TraceResult 始终非 nil；发生错误时 result.Error 与返回的 error
	// 相同，且保留已解析到的 Hops。
	Trace(ctx context.Context, target string, options ...TraceOption) (*TraceResult, error)

	// TraceBatch 并发测量多个目标。
	//
	// socket 模式内部使用 worker pool + 有界队列 + 背压 + id 映射；legacy 模式
	// 使用受限进程池。返回切片与 targets 等长且顺序一致；单目标失败写入对应
	// 元素的 Error，不影响其它目标。仅当配置非法等参数级失败时才返回非 nil 的
	// 顶层 error。
	TraceBatch(ctx context.Context, targets []string, options ...BatchOption) ([]TraceResult, error)

	// Close 释放连接/进程等资源。
	Close() error
}

// NewLegacyClient 构造一个通过本地 scamper CLI 执行测量的 Client。
//
// 未设置 WithBinary 时在 PATH 中查找 scamper；找不到返回 ErrNoScamperBinary。
func NewLegacyClient(options ...ClientOption) (Client, error) {
	c, err := legacy.New(options...)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// NewSocketClient 构造一个通过 scamper daemon control socket 执行测量的 Client。
//
// 构造时会同步完成首次 Dial 与 attach（fail-fast）。失败返回
// ErrDaemonUnavailable；attach 被 daemon 拒绝时返回 *CommandError。
func NewSocketClient(options ...ClientOption) (Client, error) {
	c, err := socket.New(options...)
	if err != nil {
		return nil, err
	}
	return c, nil
}
