// Command custom-args 演示 TraceConfig 的全部常用参数以及 ExtraArgs 透传。
//
// ExtraArgs 用于 scamper 新增、而 SDK 尚未提供专用方法参数（例如双树参数
// -z/-Z）；token 中不得包含空白或控制字符。
//
// 运行：
//
//	go run ./examples/custom-args -socket /var/run/scamper/scamper.sock 1.1.1.1
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	scamper "github.com/kanechoo/scamper-client-go"
)

func main() {
	target := flag.String("target", "1.1.1.1", "traceroute target")
	socketPath := flag.String("socket", scamper.DefaultSocketPath, "scamper control socket path")
	extra := flag.String("extra", "", "comma-separated extra scamper args passed through (e.g. -z,192.0.2.1)")
	flag.Parse()

	c, err := scamper.NewSocketClient(scamper.WithSocketPath(*socketPath))
	if err != nil {
		log.Fatalf("new socket client: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := scamper.NewTraceConfig().
		Protocol(scamper.ProtocolUDPParis).
		FirstHop(1).
		MaxHops(32).
		Wait(2 * time.Second).
		WaitProbe(100 * time.Millisecond). // -W，10ms 粒度
		WaitProbeHop(time.Second).         // -H，上限 2s
		ProbesPerHop(2).
		Squeries(1).
		GapLimit(12).
		GapAction(scamper.GapActionHalt).
		Loops(1).
		TOS(0).
		SrcPort(0).
		DstPort(33434).
		PayloadHex("00").
		PMTUD(true).
		AllProbes(false).
		TTLExceededNotDest(false).
		Option(scamper.OptionPTR)

	// ExtraArgs 原样透传 scamper 新增参数（token 内不得含空白/控制字符）。
	if *extra != "" {
		cfg.ExtraArgs(strings.Split(*extra, ",")...)
	}

	res, err := c.Trace(ctx, *target, scamper.WithTraceConfig(cfg))
	if err != nil {
		log.Fatalf("trace: %v", err)
	}

	fmt.Printf("target=%s address=%s method=%s stop=%s\n",
		res.Target, res.Address, res.Method, res.StopReason)
	for _, h := range res.Hops {
		fmt.Printf("  %2d  %-15s  %-10s  %s\n", h.TTL, h.Address, h.RTT, h.Name)
	}
}
