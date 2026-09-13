package core

import (
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Method 是 scamper trace 的探测方法（-P）。
type Method string

// 支持的探测方法，大小写不敏感地映射到 scamper 的 -P 参数值。
const (
	ProtocolUDP       Method = "udp"
	ProtocolICMP      Method = "icmp"
	ProtocolUDPParis  Method = "udp-paris"
	ProtocolICMPParis Method = "icmp-paris"
	ProtocolTCP       Method = "tcp"
	ProtocolTCPAck    Method = "tcp-ack"
)

// validMethod 报告 m 是否为受支持的探测方法。
func validMethod(m Method) bool {
	switch m {
	case ProtocolUDP, ProtocolICMP, ProtocolUDPParis, ProtocolICMPParis, ProtocolTCP, ProtocolTCPAck:
		return true
	default:
		return false
	}
}

// GapAction 指定触发 gaplimit 后的行为（-G）。
type GapAction int

const (
	// GapActionHalt 表示停止探测（scamper 默认）。
	GapActionHalt GapAction = 1
	// GapActionLastDitch 表示发送 last-ditch probes。
	GapActionLastDitch GapAction = 2
)

// Confidence 是逐跳高置信度探测水平（-c）。
type Confidence int

const (
	// Confidence95 要求 95% 置信度。
	Confidence95 Confidence = 95
	// Confidence99 要求 99% 置信度。
	Confidence99 Confidence = 99
)

// Option 是 -O 可选参数。
type Option string

const (
	// OptionBack 以递减 TTL 进行 traceroute。
	OptionBack Option = "back"
	// OptionConstPayload 不修改负载以匹配 UDP 校验和。
	OptionConstPayload Option = "const-payload"
	// OptionDL 使用 datalink socket 为收发报文打时间戳。
	OptionDL Option = "dl"
	// OptionDTreeNoBack 双树算法中不做反向探测。
	OptionDTreeNoBack Option = "dtree-noback"
	// OptionPTR 对中间跳做反向 DNS 查询。
	OptionPTR Option = "ptr"
	// OptionRaw 使用原始 socket 发送 IPv4 TCP 探测。
	OptionRaw Option = "raw"
)

// validOption 报告 o 是否为受支持的 -O 参数。
func validOption(o Option) bool {
	switch o {
	case OptionBack, OptionConstPayload, OptionDL, OptionDTreeNoBack, OptionPTR, OptionRaw:
		return true
	default:
		return false
	}
}

// TraceConfig 用 Go API builder 表达 scamper trace 命令的参数。
//
// 零值（NewTraceConfig 的返回值）不设置任何参数，完全交给 scamper 默认值，
// 避免 SDK 与 scamper 的默认值发生隐性漂移。builder 方法链式返回自身。
//
// TraceConfig 不是并发安全的；跨调用共享时请先用 Clone 复制。
type TraceConfig struct {
	protocol           *Method
	firstHop           *int
	maxHops            *int
	wait               *time.Duration
	probesPerHop       *int
	source             *string
	dstPort            *int
	srcPort            *int
	tos                *int
	confidence         *Confidence
	gapLimit           *int
	gapAction          *GapAction
	waitProbe          *time.Duration
	waitProbeHop       *time.Duration
	loops              *int
	squeries           *int
	offset             *int
	payloadHex         *string
	routerAddr         *string
	userID             *uint32
	stream             *int
	pmtud              *bool
	allProbes          *bool
	ttlExceededNotDest *bool
	options            []Option
	extraArgs          []string
}

// NewTraceConfig 返回一个未设置任何参数的 TraceConfig。
func NewTraceConfig() *TraceConfig { return &TraceConfig{} }

// Clone 返回 TraceConfig 的深拷贝，避免跨调用共享可变状态。
func (c *TraceConfig) Clone() *TraceConfig {
	if c == nil {
		return NewTraceConfig()
	}
	cp := &TraceConfig{}
	*cp = *c
	if c.protocol != nil {
		v := *c.protocol
		cp.protocol = &v
	}
	if c.firstHop != nil {
		v := *c.firstHop
		cp.firstHop = &v
	}
	if c.maxHops != nil {
		v := *c.maxHops
		cp.maxHops = &v
	}
	if c.wait != nil {
		v := *c.wait
		cp.wait = &v
	}
	if c.probesPerHop != nil {
		v := *c.probesPerHop
		cp.probesPerHop = &v
	}
	if c.source != nil {
		v := *c.source
		cp.source = &v
	}
	if c.dstPort != nil {
		v := *c.dstPort
		cp.dstPort = &v
	}
	if c.srcPort != nil {
		v := *c.srcPort
		cp.srcPort = &v
	}
	if c.tos != nil {
		v := *c.tos
		cp.tos = &v
	}
	if c.confidence != nil {
		v := *c.confidence
		cp.confidence = &v
	}
	if c.gapLimit != nil {
		v := *c.gapLimit
		cp.gapLimit = &v
	}
	if c.gapAction != nil {
		v := *c.gapAction
		cp.gapAction = &v
	}
	if c.waitProbe != nil {
		v := *c.waitProbe
		cp.waitProbe = &v
	}
	if c.waitProbeHop != nil {
		v := *c.waitProbeHop
		cp.waitProbeHop = &v
	}
	if c.loops != nil {
		v := *c.loops
		cp.loops = &v
	}
	if c.squeries != nil {
		v := *c.squeries
		cp.squeries = &v
	}
	if c.offset != nil {
		v := *c.offset
		cp.offset = &v
	}
	if c.payloadHex != nil {
		v := *c.payloadHex
		cp.payloadHex = &v
	}
	if c.routerAddr != nil {
		v := *c.routerAddr
		cp.routerAddr = &v
	}
	if c.userID != nil {
		v := *c.userID
		cp.userID = &v
	}
	if c.stream != nil {
		v := *c.stream
		cp.stream = &v
	}
	if c.pmtud != nil {
		v := *c.pmtud
		cp.pmtud = &v
	}
	if c.allProbes != nil {
		v := *c.allProbes
		cp.allProbes = &v
	}
	if c.ttlExceededNotDest != nil {
		v := *c.ttlExceededNotDest
		cp.ttlExceededNotDest = &v
	}
	cp.options = append([]Option(nil), c.options...)
	cp.extraArgs = append([]string(nil), c.extraArgs...)
	return cp
}

// Protocol 设置探测方法（-P）。大小写不敏感，统一转为小写。
func (c *TraceConfig) Protocol(m Method) *TraceConfig {
	v := Method(strings.ToLower(string(m)))
	c.protocol = &v
	return c
}

// FirstHop 设置起始 TTL（-f），合法范围 1..255。
func (c *TraceConfig) FirstHop(ttl int) *TraceConfig {
	c.firstHop = intPtr(ttl)
	return c
}

// MaxHops 设置最大 TTL（-m），合法范围 1..255。
func (c *TraceConfig) MaxHops(ttl int) *TraceConfig {
	c.maxHops = intPtr(ttl)
	return c
}

// Wait 设置每个 probe 的等待时间（-w，秒）。
func (c *TraceConfig) Wait(d time.Duration) *TraceConfig {
	c.wait = durPtr(d)
	return c
}

// ProbesPerHop 设置每跳最大尝试次数（-q），必须 >= 1。
func (c *TraceConfig) ProbesPerHop(n int) *TraceConfig {
	c.probesPerHop = intPtr(n)
	return c
}

// Source 设置探测源地址（-S，IPv4/IPv6）。空字符串表示不设置。
func (c *TraceConfig) Source(addr string) *TraceConfig {
	c.source = strPtr(addr)
	return c
}

// DstPort 设置目的端口基址（-d），合法范围 0..65535。
func (c *TraceConfig) DstPort(port int) *TraceConfig {
	c.dstPort = intPtr(port)
	return c
}

// SrcPort 设置源端口 / ICMP ID（-s），合法范围 0..65535；0 表示由 OS 分配。
func (c *TraceConfig) SrcPort(port int) *TraceConfig {
	c.srcPort = intPtr(port)
	return c
}

// TOS 设置 IP ToS/DSCP+ECN 字节（-t），合法范围 0..255。
func (c *TraceConfig) TOS(tos int) *TraceConfig {
	c.tos = intPtr(tos)
	return c
}

// Confidence 设置逐跳高置信度探测水平（-c），仅接受 95 或 99。
func (c *TraceConfig) Confidence(conf Confidence) *TraceConfig {
	c.confidence = &conf
	return c
}

// GapLimit 设置连续无响应跳数上限（-g）；0 表示禁用。
func (c *TraceConfig) GapLimit(n int) *TraceConfig {
	c.gapLimit = intPtr(n)
	return c
}

// GapAction 设置触发 gaplimit 后的行为（-G）。
func (c *TraceConfig) GapAction(a GapAction) *TraceConfig {
	c.gapAction = &a
	return c
}

// WaitProbe 设置相邻 probe 之间的最小间隔（-W，按 10ms 粒度向下取整）。
func (c *TraceConfig) WaitProbe(d time.Duration) *TraceConfig {
	c.waitProbe = durPtr(d)
	return c
}

// WaitProbeHop 设置同一 TTL 连续 probe 之间的最小间隔（-H，秒，上限 2s）。
func (c *TraceConfig) WaitProbeHop(d time.Duration) *TraceConfig {
	c.waitProbeHop = durPtr(d)
	return c
}

// Loops 设置允许的环路数（-l）；0 表示禁用环路检测。
func (c *TraceConfig) Loops(n int) *TraceConfig {
	c.loops = intPtr(n)
	return c
}

// Squeries 设置允许同时在途的跳数（-N），必须 >= 1 且小于 GapLimit。
func (c *TraceConfig) Squeries(n int) *TraceConfig {
	c.squeries = intPtr(n)
	return c
}

// Offset 设置分片偏移（-o），合法范围 0..8191。
func (c *TraceConfig) Offset(n int) *TraceConfig {
	c.offset = intPtr(n)
	return c
}

// PayloadHex 设置 probe 负载基址（-p，十六进制）。空字符串表示不设置。
func (c *TraceConfig) PayloadHex(payload string) *TraceConfig {
	c.payloadHex = strPtr(payload)
	return c
}

// RouterAddr 设置要使用的路由器地址（-r，IP）。空字符串表示不设置。
func (c *TraceConfig) RouterAddr(addr string) *TraceConfig {
	c.routerAddr = strPtr(addr)
	return c
}

// UserID 设置随数据返回的用户自定义标识（-U）。
func (c *TraceConfig) UserID(id uint32) *TraceConfig {
	c.userID = &id
	return c
}

// Stream 设置每发送 N 个包上报一次中间结果（-y）；0 表示仅完成后上报。
func (c *TraceConfig) Stream(n int) *TraceConfig {
	c.stream = intPtr(n)
	return c
}

// PMTUD 设置是否在 traceroute 完成后做路径 MTU 发现（-M）。
func (c *TraceConfig) PMTUD(enabled bool) *TraceConfig {
	c.pmtud = boolPtr(enabled)
	return c
}

// AllProbes 设置是否无论收到多少响应都发完所有 probe（-Q）。
func (c *TraceConfig) AllProbes(enabled bool) *TraceConfig {
	c.allProbes = boolPtr(enabled)
	return c
}

// TTLExceededNotDest 设置目的地的 time-exceeded 是否不算到达（-T）。
func (c *TraceConfig) TTLExceededNotDest(enabled bool) *TraceConfig {
	c.ttlExceededNotDest = boolPtr(enabled)
	return c
}

// Option 追加一个 -O 可选参数；重复的取值会被去重（保留首次出现）。
func (c *TraceConfig) Option(o Option) *TraceConfig {
	for _, existing := range c.options {
		if existing == o {
			return c
		}
	}
	c.options = append(c.options, o)
	return c
}

// ExtraArgs 原样追加未来新增的 scamper 参数，置于 argv 尾部。
//
// 含空白或控制字符的 token 会导致行协议与 legacy 命令行组装出错，将被拒绝。
func (c *TraceConfig) ExtraArgs(args ...string) *TraceConfig {
	c.extraArgs = append(c.extraArgs, args...)
	return c
}

// Validate 校验配置是否合法，非法时返回 *InvalidConfigError
// （errors.Is(err, ErrInvalidConfig) 为 true）。
func (c *TraceConfig) Validate() error {
	if c == nil {
		return nil
	}
	if c.protocol != nil && !validMethod(*c.protocol) {
		return invalid("protocol", fmt.Sprintf("unsupported method %q", *c.protocol))
	}
	if c.firstHop != nil && (*c.firstHop < 1 || *c.firstHop > 255) {
		return invalid("firstHop", "must be in 1..255")
	}
	if c.maxHops != nil && (*c.maxHops < 1 || *c.maxHops > 255) {
		return invalid("maxHops", "must be in 1..255")
	}
	if c.firstHop != nil && c.maxHops != nil && *c.firstHop > *c.maxHops {
		return invalid("firstHop", "must be <= maxHops")
	}
	if c.wait != nil && *c.wait < 0 {
		return invalid("wait", "must be >= 0")
	}
	if c.probesPerHop != nil && *c.probesPerHop < 1 {
		return invalid("probesPerHop", "must be >= 1")
	}
	if c.source != nil && *c.source != "" {
		if net.ParseIP(*c.source) == nil {
			return invalid("source", fmt.Sprintf("not a valid IP address: %q", *c.source))
		}
	}
	if c.dstPort != nil && (*c.dstPort < 0 || *c.dstPort > 65535) {
		return invalid("dstPort", "must be in 0..65535")
	}
	if c.srcPort != nil && (*c.srcPort < 0 || *c.srcPort > 65535) {
		return invalid("srcPort", "must be in 0..65535")
	}
	if c.tos != nil && (*c.tos < 0 || *c.tos > 255) {
		return invalid("tos", "must be in 0..255")
	}
	if c.confidence != nil && *c.confidence != Confidence95 && *c.confidence != Confidence99 {
		return invalid("confidence", "must be 95 or 99")
	}
	if c.gapLimit != nil && *c.gapLimit < 0 {
		return invalid("gapLimit", "must be >= 0")
	}
	if c.gapAction != nil && *c.gapAction != GapActionHalt && *c.gapAction != GapActionLastDitch {
		return invalid("gapAction", "must be 1 (halt) or 2 (last-ditch)")
	}
	if c.waitProbe != nil && *c.waitProbe < 0 {
		return invalid("waitProbe", "must be >= 0")
	}
	if c.waitProbeHop != nil && (*c.waitProbeHop < 0 || *c.waitProbeHop > 2*time.Second) {
		return invalid("waitProbeHop", "must be in 0..2s")
	}
	if c.loops != nil && *c.loops < 0 {
		return invalid("loops", "must be >= 0")
	}
	if c.squeries != nil && *c.squeries < 1 {
		return invalid("squeries", "must be >= 1")
	}
	if c.squeries != nil && c.gapLimit != nil && *c.gapLimit > 0 && *c.squeries >= *c.gapLimit {
		return invalid("squeries", "must be less than gapLimit")
	}
	if c.offset != nil && (*c.offset < 0 || *c.offset > 8191) {
		return invalid("offset", "must be in 0..8191")
	}
	if c.payloadHex != nil && *c.payloadHex != "" {
		if len(*c.payloadHex)%2 != 0 {
			return invalid("payloadHex", "must have an even number of hex digits")
		}
		if _, err := hex.DecodeString(*c.payloadHex); err != nil {
			return invalid("payloadHex", "not valid hexadecimal")
		}
	}
	if c.routerAddr != nil && *c.routerAddr != "" {
		if net.ParseIP(*c.routerAddr) == nil {
			return invalid("routerAddr", fmt.Sprintf("not a valid IP address: %q", *c.routerAddr))
		}
	}
	if c.stream != nil && *c.stream < 0 {
		return invalid("stream", "must be >= 0")
	}
	for _, o := range c.options {
		if !validOption(o) {
			return invalid("option", fmt.Sprintf("unsupported -O option %q", o))
		}
	}
	for _, a := range c.extraArgs {
		if err := ValidateToken(a); err != nil {
			return invalid("extraArgs", err.Error())
		}
	}
	return nil
}

// Build 产出 scamper trace 命令的 argv（不含 "trace" 与 target）。
//
// 输出仅包含被显式设置的选项，顺序稳定，最后追加 ExtraArgs。
// 配置非法时返回 *InvalidConfigError。
func (c *TraceConfig) Build() ([]string, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if c == nil {
		return nil, nil
	}
	args := make([]string, 0, 24)

	if c.protocol != nil {
		args = append(args, "-P", string(*c.protocol))
	}
	if c.firstHop != nil {
		args = append(args, "-f", strconv.Itoa(*c.firstHop))
	}
	if c.maxHops != nil {
		args = append(args, "-m", strconv.Itoa(*c.maxHops))
	}
	if c.wait != nil {
		args = append(args, "-w", strconv.FormatInt(int64(*c.wait/time.Second), 10))
	}
	if c.probesPerHop != nil {
		args = append(args, "-q", strconv.Itoa(*c.probesPerHop))
	}
	if c.source != nil && *c.source != "" {
		args = append(args, "-S", *c.source)
	}
	if c.dstPort != nil {
		args = append(args, "-d", strconv.Itoa(*c.dstPort))
	}
	if c.srcPort != nil {
		args = append(args, "-s", strconv.Itoa(*c.srcPort))
	}
	if c.tos != nil {
		args = append(args, "-t", strconv.Itoa(*c.tos))
	}
	if c.confidence != nil {
		args = append(args, "-c", strconv.Itoa(int(*c.confidence)))
	}
	if c.gapLimit != nil {
		args = append(args, "-g", strconv.Itoa(*c.gapLimit))
	}
	if c.gapAction != nil {
		args = append(args, "-G", strconv.Itoa(int(*c.gapAction)))
	}
	if c.waitProbe != nil {
		// scamper -W 的单位是 10ms，向下取整到该粒度。
		args = append(args, "-W", strconv.FormatInt(int64(*c.waitProbe/(10*time.Millisecond)), 10))
	}
	if c.waitProbeHop != nil {
		args = append(args, "-H", strconv.FormatInt(int64(*c.waitProbeHop/time.Second), 10))
	}
	if c.loops != nil {
		args = append(args, "-l", strconv.Itoa(*c.loops))
	}
	if c.squeries != nil {
		args = append(args, "-N", strconv.Itoa(*c.squeries))
	}
	if c.offset != nil {
		args = append(args, "-o", strconv.Itoa(*c.offset))
	}
	if c.payloadHex != nil && *c.payloadHex != "" {
		args = append(args, "-p", *c.payloadHex)
	}
	if c.routerAddr != nil && *c.routerAddr != "" {
		args = append(args, "-r", *c.routerAddr)
	}
	if c.userID != nil {
		args = append(args, "-U", strconv.FormatUint(uint64(*c.userID), 10))
	}
	if c.stream != nil {
		args = append(args, "-y", strconv.Itoa(*c.stream))
	}
	if c.pmtud != nil && *c.pmtud {
		args = append(args, "-M")
	}
	if c.allProbes != nil && *c.allProbes {
		args = append(args, "-Q")
	}
	if c.ttlExceededNotDest != nil && *c.ttlExceededNotDest {
		args = append(args, "-T")
	}
	for _, o := range c.options {
		args = append(args, "-O", string(o))
	}
	args = append(args, c.extraArgs...)
	return args, nil
}

// BuildArgv 组装完整的 scamper 命令 argv：[trace, <Build()...>, target]。
//
// 它会校验 target 与配置；任一非法即返回错误。两种模式共用该函数，保证
// Legacy 与 Socket 的命令组装完全一致。
func BuildArgv(c *TraceConfig, target string) ([]string, error) {
	if err := ValidateToken(target); err != nil {
		return nil, invalid("target", err.Error())
	}
	if c == nil {
		c = NewTraceConfig()
	}
	args, err := c.Build()
	if err != nil {
		return nil, err
	}
	argv := make([]string, 0, len(args)+2)
	argv = append(argv, "trace")
	argv = append(argv, args...)
	argv = append(argv, target)
	return argv, nil
}

// ValidateToken 校验一个将要进入 scamper 命令行的 token。
//
// 它会拒绝空 token，以及包含空白、控制字符的 token：这类字符会破坏 socket
// 行协议（以 \n 分帧）并让 legacy 的 -I 字符串组装产生歧义。
func ValidateToken(s string) error {
	if s == "" {
		return fmt.Errorf("token must not be empty")
	}
	if i := strings.IndexFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}); i >= 0 {
		return fmt.Errorf("token %q contains whitespace or control character at byte %d", s, i)
	}
	return nil
}

// invalid 构造字段级校验错误。
func invalid(field, reason string) error {
	return &InvalidConfigError{Field: field, Reason: reason}
}

func intPtr(v int) *int                     { return &v }
func durPtr(v time.Duration) *time.Duration { return &v }
func strPtr(v string) *string               { return &v }
func boolPtr(v bool) *bool                  { return &v }
