package scamper

import "github.com/kanechoo/scamper-client-go/internal/core"

// 统一错误哨兵。所有错误均可用 errors.Is/errors.As 判定。
var (
	// ErrNoScamperBinary 表示找不到可执行的 scamper CLI（legacy 模式）。
	ErrNoScamperBinary = core.ErrNoScamperBinary
	// ErrDaemonUnavailable 表示无法连接或连接已断开（socket 模式，可重试）。
	ErrDaemonUnavailable = core.ErrDaemonUnavailable
	// ErrProtocol 表示 control socket 帧格式错误（可重试）。
	ErrProtocol = core.ErrProtocol
	// ErrParse 表示 scamper JSON 输出无法解析（可重试）。
	ErrParse = core.ErrParse
	// ErrCommand 表示 scamper 拒绝命令（不可重试）。
	ErrCommand = core.ErrCommand
	// ErrInvalidConfig 表示 TraceConfig 非法（不可重试）。
	ErrInvalidConfig = core.ErrInvalidConfig
	// ErrClosed 表示客户端已关闭。
	ErrClosed = core.ErrClosed
	// ErrTimeout 是超时错误的哨兵。
	ErrTimeout = core.ErrTimeout
	// ErrBatchStopped 表示批量因 WithStopOnError 提前中止。
	ErrBatchStopped = core.ErrBatchStopped
)

// CommandError 表示 scamper 拒绝了一条命令。
type CommandError = core.CommandError

// TimeoutError 表示单目标测量超时。
type TimeoutError = core.TimeoutError

// ClientClosedError 表示在客户端关闭后仍尝试使用它。
type ClientClosedError = core.ClientClosedError

// InvalidConfigError 是 TraceConfig 校验失败的结构化错误。
type InvalidConfigError = core.InvalidConfigError

// IsRetryable 判断错误是否值得重试。
//
// 可重试：ErrDaemonUnavailable、TimeoutError、ErrProtocol、ErrParse；
// 不可重试：CommandError、InvalidConfigError、ClientClosedError、ErrBatchStopped。
func IsRetryable(err error) bool {
	return core.IsRetryable(err)
}
