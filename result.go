package scamper

import "github.com/kanechoo/scamper-client-go/internal/core"

// TraceResult 是一次 traceroute 测量的统一结果，两种模式完全一致。
//
// 它是 internal/core.TraceResult 的类型别名，二者是同一类型。
type TraceResult = core.TraceResult

// Hop 表示路径上的一跳。
type Hop = core.Hop

// Metadata 是附加到结果上的 JSON 友好键值集合。
type Metadata = core.Metadata
