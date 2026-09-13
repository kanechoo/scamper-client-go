package scamper

import "github.com/kanechoo/scamper-client-go/internal/core"

// TraceConfig 用 Go API builder 表达 scamper trace 命令的参数。
//
// 未调用对应方法即完全交给 scamper 默认值，避免 SDK 与 scamper 默认值漂移。
// 它是 internal/core.TraceConfig 的类型别名，builder 方法与 Build/Validate/Clone
// 均直接可用。
type TraceConfig = core.TraceConfig

// Method 是探测方法（scamper -P）。
type Method = core.Method

// GapAction 指定触发 gaplimit 后的行为（scamper -G）。
type GapAction = core.GapAction

// Confidence 是逐跳高置信度探测水平（scamper -c）。
type Confidence = core.Confidence

// Option 是 scamper 的 -O 可选参数。
type Option = core.Option

// 支持的探测方法。
const (
	ProtocolUDP       = core.ProtocolUDP
	ProtocolICMP      = core.ProtocolICMP
	ProtocolUDPParis  = core.ProtocolUDPParis
	ProtocolICMPParis = core.ProtocolICMPParis
	ProtocolTCP       = core.ProtocolTCP
	ProtocolTCPAck    = core.ProtocolTCPAck
)

// gaplimit 行为。
const (
	GapActionHalt      = core.GapActionHalt
	GapActionLastDitch = core.GapActionLastDitch
)

// 置信度水平。
const (
	Confidence95 = core.Confidence95
	Confidence99 = core.Confidence99
)

// -O 可选参数。
const (
	OptionBack         = core.OptionBack
	OptionConstPayload = core.OptionConstPayload
	OptionDL           = core.OptionDL
	OptionDTreeNoBack  = core.OptionDTreeNoBack
	OptionPTR          = core.OptionPTR
	OptionRaw          = core.OptionRaw
)

// NewTraceConfig 返回一个未设置任何参数的 TraceConfig。
//
// 未设置的参数会完全交给 scamper 默认值。
func NewTraceConfig() *TraceConfig {
	return core.NewTraceConfig()
}
