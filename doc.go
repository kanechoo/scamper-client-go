// Package scamper 是一个驱动 scamper 做 traceroute 测量的通用 Go SDK。
//
// SDK 负责连接（本地 scamper CLI 或 daemon control socket）、提交任务、接收与
// 解析 JSON 结果，以及 socket 模式下的并发调度（worker pool、有界队列、背压、
// 重试）。SDK 不负责启动/停止 scamper daemon，也不管理其窗口、PPS 与权限。
//
// 两种运行模式对用户完全透明，返回相同的 *TraceResult：
//
//	Legacy:  exec 本地 scamper，解析 stdout 的 JSON。
//	Socket:  连接 daemon 的 Unix domain socket，attach JSON 模式逐帧解析。
//
// 推荐只 import 本包，通过工厂函数构造客户端：
//
//	c, err := scamper.NewSocketClient(
//	    scamper.WithSocketPath("/var/run/scamper/scamper.sock"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer c.Close()
//
//	cfg := scamper.NewTraceConfig().
//	    Protocol(scamper.ProtocolUDP).
//	    FirstHop(9).MaxHops(16).
//	    Wait(time.Second).ProbesPerHop(1)
//
//	res, err := c.Trace(ctx, "1.1.1.1", scamper.WithTraceConfig(cfg))
//
// 子包 legacy 与 socket 面向需要直接控制具体实现的高级用户；普通用户无需关心。
package scamper
