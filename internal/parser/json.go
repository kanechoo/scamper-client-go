// Package parser 把 scamper 的 JSON 输出转换为 core.TraceResult。
//
// 它同时服务 legacy（解析 scamper CLI stdout）与 socket（解析 control socket
// 的 DATA 帧）两种模式，保证两种模式产出完全一致的结果结构。
//
// 只支持 JSON，不实现 WARTS。
package parser

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/kanechoo/scamper-client-go/internal/core"
)

// traceRecord 映射 scamper trace JSON 记录的字段。
//
// 字段名与 scamper 的 JSON writer 严格对齐（见 scamper 源码 json 输出）。
// 使用指针区分“字段缺失”与“字段为零值”。
type traceRecord struct {
	Type       string      `json:"type"`
	Version    string      `json:"version"`
	UserID     *uint32     `json:"userid"`
	Method     string      `json:"method"`
	Src        string      `json:"src"`
	Dst        string      `json:"dst"`
	Rtr        string      `json:"rtr"`
	Sport      *int        `json:"sport"`
	Dport      *int        `json:"dport"`
	ICMPsum    *int        `json:"icmp_sum"`
	StopReason string      `json:"stop_reason"`
	StopData   *int64      `json:"stop_data"`
	ErrMsg     string      `json:"errmsg"`
	HopCount   *int        `json:"hop_count"`
	Attempts   *int        `json:"attempts"`
	HopLimit   *int        `json:"hoplimit"`
	FirstHop   *int        `json:"firsthop"`
	Wait       *int        `json:"wait"`
	WaitProbe  *int        `json:"wait_probe"`
	TOS        *int        `json:"tos"`
	ProbeSize  *int        `json:"probe_size"`
	ProbeCount *int        `json:"probe_count"`
	Monitor    string      `json:"monitor"`
	Hops       []hopRecord `json:"hops"`
}

// hopRecord 映射 scamper trace 的单跳 JSON。
type hopRecord struct {
	Addr      string   `json:"addr"`
	Name      string   `json:"name"`
	RTT       *float64 `json:"rtt"`
	ReplyTTL  *int     `json:"reply_ttl"`
	ReplyTOS  *int     `json:"reply_tos"`
	ReplyIPID *int64   `json:"reply_ipid"`
	ReplySize *int     `json:"reply_size"`
	ICMPType  *int     `json:"icmp_type"`
	ICMPCode  *int     `json:"icmp_code"`
	ProbeTTL  *int     `json:"probe_ttl"`
	ProbeID   *int     `json:"probe_id"`
	ProbeSize *int     `json:"probe_size"`
	TCPFlags  *int     `json:"tcp_flags"`
}

// ParseTrace 从 data 中解析第一条 type=="trace" 的记录。
//
// data 可以是单个 JSON 对象，也可以是 JSON 流（scamper -O json 逐行输出多条
// 记录）。非 trace 类型（如 cycle-start / cycle-stop / list）会被跳过。
//
// target 是调用方请求的原始目标，会写入 TraceResult.Target；scamper 实际使用的
// 地址写入 Address。
func ParseTrace(data []byte, target string) (*core.TraceResult, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var rec traceRecord
		if err := dec.Decode(&rec); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("%w: %v", core.ErrParse, err)
		}
		if rec.Type != "trace" {
			continue
		}
		return convert(&rec, target), nil
	}
	return nil, fmt.Errorf("%w: no trace record in output", core.ErrParse)
}

// convert 把 json 记录转换为 core.TraceResult。
func convert(rec *traceRecord, target string) *core.TraceResult {
	res := &core.TraceResult{
		Target:     target,
		Address:    rec.Dst,
		Method:     rec.Method,
		StopReason: rec.StopReason,
		Hops:       make([]core.Hop, 0, len(rec.Hops)),
	}

	for _, h := range rec.Hops {
		hop := core.Hop{
			Address: h.Addr,
			Name:    h.Name,
		}
		if h.ProbeTTL != nil {
			hop.TTL = *h.ProbeTTL
		}
		if h.RTT != nil {
			hop.RTT = time.Duration(*h.RTT * float64(time.Millisecond))
		}
		if h.ReplyTTL != nil {
			hop.ReplyTTL = *h.ReplyTTL
		}
		switch {
		case h.ProbeSize != nil:
			hop.ProbeSize = *h.ProbeSize
		case rec.ProbeSize != nil:
			hop.ProbeSize = *rec.ProbeSize
		}
		res.Hops = append(res.Hops, hop)
	}

	res.Metadata = buildMetadata(rec)

	if rec.ErrMsg != "" {
		res.Error = fmt.Errorf("scamper: trace %s failed: %s", target, rec.ErrMsg)
	}
	return res
}

// buildMetadata 提取调用方可能关心的原始字段；无有效字段时返回 nil。
func buildMetadata(rec *traceRecord) core.Metadata {
	md := make(core.Metadata, 12)
	if rec.StopData != nil {
		md["stop_data"] = *rec.StopData
	}
	if rec.ProbeCount != nil {
		md["probe_count"] = *rec.ProbeCount
	}
	if rec.HopCount != nil {
		md["hop_count"] = *rec.HopCount
	}
	if rec.Attempts != nil {
		md["attempts"] = *rec.Attempts
	}
	if rec.HopLimit != nil {
		md["hoplimit"] = *rec.HopLimit
	}
	if rec.FirstHop != nil {
		md["firsthop"] = *rec.FirstHop
	}
	if rec.Wait != nil {
		md["wait"] = *rec.Wait
	}
	if rec.WaitProbe != nil {
		md["wait_probe"] = *rec.WaitProbe
	}
	if rec.TOS != nil {
		md["tos"] = *rec.TOS
	}
	if rec.ProbeSize != nil {
		md["probe_size"] = *rec.ProbeSize
	}
	if rec.Sport != nil {
		md["sport"] = *rec.Sport
	}
	if rec.Dport != nil {
		md["dport"] = *rec.Dport
	}
	if rec.ICMPsum != nil {
		md["icmp_sum"] = *rec.ICMPsum
	}
	if rec.Src != "" {
		md["src"] = rec.Src
	}
	if rec.Rtr != "" {
		md["rtr"] = rec.Rtr
	}
	if rec.Version != "" {
		md["version"] = rec.Version
	}
	if rec.Monitor != "" {
		md["monitor"] = rec.Monitor
	}
	if rec.UserID != nil {
		md["userid"] = *rec.UserID
	}
	if rec.ErrMsg != "" {
		md["errmsg"] = rec.ErrMsg
	}
	if len(md) == 0 {
		return nil
	}
	return md
}
