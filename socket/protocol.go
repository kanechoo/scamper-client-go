// Package socket 提供通过 scamper daemon 的 control socket（Unix domain socket）
// 做高并发 traceroute 的 Client 实现。
//
// 协议要点（见 man scamper 的 CONTROL SOCKET / ATTACH MODE）：
//
//	连接后发送 "attach format json\n"，等待 "OK"。
//	attach 模式下直接发送 "trace <args> <target>\n"。
//	daemon 以 "OK id-N"、"ERR <msg>"、"MORE"、
//	"DATA <len> [id-N]\n<len 字节 JSON>" 应答。
//	超时/取消时发送 "halt <id>\n"。
//
// 普通用户应通过 root 包使用 NewSocketClient，而非直接 import 本包。
package socket

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kanechoo/scamper-client-go/internal/core"
)

const (
	// attachCommand 是进入 attach JSON 模式的首行命令。
	attachCommand = "attach format json\n"
	// idPrefix 是 daemon 返回的 id 前缀（形如 "id-12"）。
	idPrefix = "id-"
	// inProgressReason 是 -y stream 中间结果的 stop_reason；收到时应继续等待。
	inProgressReason = "INPROGRESS"
)

// parseID 解析 "id-N" 中的 N（或裸数字串）。
func parseID(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, idPrefix)
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid id %q", core.ErrProtocol, s)
	}
	return id, nil
}
