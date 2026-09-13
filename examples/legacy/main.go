// Command legacy 演示用本地 scamper CLI（legacy 模式）做一次 traceroute。
//
// 运行前请确保系统已安装 scamper 且在 PATH 中：
//
//	go run ./examples/legacy 1.1.1.1
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
	binary := flag.String("binary", "", "path to the scamper binary (default: find in PATH)")
	flag.Parse()

	var opts []scamper.ClientOption
	if *binary != "" {
		opts = append(opts, scamper.WithBinary(*binary))
	}

	c, err := scamper.NewLegacyClient(opts...)
	if err != nil {
		log.Fatalf("new legacy client: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := scamper.NewTraceConfig().
		Protocol(scamper.ProtocolUDP).
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
