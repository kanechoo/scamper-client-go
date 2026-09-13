package core

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// 统一错误哨兵。所有导出错误都可用 errors.Is/errors.As 判定。
var (
	// ErrNoScamperBinary 表示找不到可执行的 scamper CLI（legacy 模式）。
	ErrNoScamperBinary = errors.New("scamper: binary not found")
	// ErrDaemonUnavailable 表示无法连接或连接已断开（socket 模式，可重试）。
	ErrDaemonUnavailable = errors.New("scamper: daemon unavailable")
	// ErrProtocol 表示 control socket 帧格式错误（可重试）。
	ErrProtocol = errors.New("scamper: protocol error")
	// ErrParse 表示 scamper JSON 输出无法解析（可重试）。
	ErrParse = errors.New("scamper: result parse error")
	// ErrCommand 表示 scamper 拒绝命令（不可重试）。
	ErrCommand = errors.New("scamper: command rejected")
	// ErrInvalidConfig 表示 TraceConfig 非法（不可重试）。
	ErrInvalidConfig = errors.New("scamper: invalid trace config")
	// ErrClosed 表示客户端已关闭。
	ErrClosed = errors.New("scamper: client closed")
	// ErrTimeout 是 TimeoutError 的哨兵，便于 errors.Is 判定。
	ErrTimeout = errors.New("scamper: timeout")
	// ErrBatchStopped 表示批量任务因 WithStopOnError 提前中止，剩余目标未执行。
	ErrBatchStopped = errors.New("scamper: batch stopped after error")
)

// CommandError 表示 scamper 拒绝了一条命令（attach 失败或 trace 参数被拒）。
type CommandError struct {
	// Args 是被拒绝的命令 token 序列（不含 shell 引号）。
	Args []string
	// Message 是 scamper 返回的错误文本。
	Message string
}

func (e *CommandError) Error() string {
	if e == nil {
		return ErrCommand.Error()
	}
	if len(e.Args) == 0 {
		return fmt.Sprintf("%s: %s", ErrCommand, e.Message)
	}
	return fmt.Sprintf("%s: %s: %s", ErrCommand, joinArgs(e.Args), e.Message)
}

// Unwrap 使 errors.Is(err, ErrCommand) 成立。
func (e *CommandError) Unwrap() error { return ErrCommand }

// TimeoutError 表示单目标测量超时。
type TimeoutError struct {
	// Target 是发生超时的目标。
	Target string
	// Timeout 是生效的单目标超时。
	Timeout time.Duration
}

func (e *TimeoutError) Error() string {
	if e == nil {
		return ErrTimeout.Error()
	}
	if e.Target == "" {
		return fmt.Sprintf("%s: after %s", ErrTimeout, e.Timeout)
	}
	return fmt.Sprintf("%s: target %s after %s", ErrTimeout, e.Target, e.Timeout)
}

// Unwrap 使 errors.Is(err, ErrTimeout) 成立。
func (e *TimeoutError) Unwrap() error { return ErrTimeout }

// ClientClosedError 表示在客户端已关闭后仍尝试使用它。
type ClientClosedError struct{}

func (e *ClientClosedError) Error() string { return ErrClosed.Error() }

// Unwrap 使 errors.Is(err, ErrClosed) 成立。
func (e *ClientClosedError) Unwrap() error { return ErrClosed }

// InvalidConfigError 包装 TraceConfig 校验失败的具体原因。
type InvalidConfigError struct {
	// Field 是出错的配置字段名。
	Field string
	// Reason 是失败原因。
	Reason string
}

func (e *InvalidConfigError) Error() string {
	if e == nil {
		return ErrInvalidConfig.Error()
	}
	return fmt.Sprintf("%s: %s: %s", ErrInvalidConfig, e.Field, e.Reason)
}

// Unwrap 使 errors.Is(err, ErrInvalidConfig) 成立。
func (e *InvalidConfigError) Unwrap() error { return ErrInvalidConfig }

// IsRetryable 判断错误是否值得重试。
//
// 可重试：ErrDaemonUnavailable、TimeoutError、ErrProtocol、ErrParse 以及
// context 的 DeadlineExceeded/Canceled（由上层 ctx 决定是否停止重试）。
// 不可重试：CommandError、InvalidConfigError、ClientClosedError、ErrBatchStopped。
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	var ce *CommandError
	if errors.As(err, &ce) {
		return false
	}
	var ic *InvalidConfigError
	if errors.As(err, &ic) {
		return false
	}
	var cc *ClientClosedError
	if errors.As(err, &cc) {
		return false
	}
	var te *TimeoutError
	if errors.As(err, &te) {
		return true
	}

	switch {
	case errors.Is(err, ErrDaemonUnavailable),
		errors.Is(err, ErrProtocol),
		errors.Is(err, ErrParse),
		errors.Is(err, ErrTimeout),
		errors.Is(err, context.DeadlineExceeded):
		return true
	case errors.Is(err, context.Canceled):
		// 取消通常来自调用方主动中止，是否继续由上层 ctx 状态决定。
		return true
	default:
		return false
	}
}

// joinArgs 用于错误信息展示，避免引入 strings 依赖之外的拼接逻辑。
func joinArgs(args []string) string {
	n := 0
	for _, a := range args {
		n += len(a) + 1
	}
	b := make([]byte, 0, n)
	for i, a := range args {
		if i > 0 {
			b = append(b, ' ')
		}
		b = append(b, a...)
	}
	return string(b)
}
