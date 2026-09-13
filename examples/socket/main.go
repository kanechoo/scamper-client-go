// Command socket 演示连接 scamper daemon 的 control socket 做一次 traceroute。
//
// daemon 需由用户提前启动，例如：
//
//	scamper -D -U /var/run/scamper/scamper.sock
//
// 然后运行：
//
//	go run ./examples/socket -socket /var/run/scamper/scamper.sock 1.1.1.1
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	scamper "github.com/kanechoo/scamper-client-go"
)

func main() {
	target := flag.String("target", "1.1.1.1", "traceroute target")
	socketPath := flag.String("socket", scamper.DefaultSocketPath, "scamper control socket path")
	flag.Parse()

	c, err := scamper.NewSocketClient(scamper.WithSocketPath(*socketPath))
	if err != nil {
		log.Fatalf("new socket client: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := scamper.NewTraceConfig().
		Protocol(scamper.ProtocolICMPParis).
		FirstHop(1).
		MaxHops(16).
		Wait(time.Second).
		ProbesPerHop(1)

	res, err := c.Trace(ctx, *target, scamper.WithTraceConfig(cfg))
	if err != nil {
		log.Fatalf("trace: %v", err)
	}

	fmt.Printf("target=%s address=%s method=%s stop=%s hops=%d duration=%s\n",
		res.Target, res.Address, res.Method, res.StopReason, len(res.Hops), res.Duration)
	for _, h := range res.Hops {
		fmt.Printf("  %2d  %-15s  %-10s  %s\n", h.TTL, h.Address, h.RTT, h.Name)
	}
}
