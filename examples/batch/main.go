// Command batch 演示 socket 模式下的批量并发测量（worker pool + 背压 + 进度）。
//
// 运行：
//
//	go run ./examples/batch -socket /var/run/scamper/scamper.sock 1.1.1.1 8.8.8.8 9.9.9.9
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
	socketPath := flag.String("socket", scamper.DefaultSocketPath, "scamper control socket path")
	concurrency := flag.Int("c", 32, "max in-flight tasks")
	flag.Parse()

	targets := flag.Args()
	if len(targets) == 0 {
		targets = []string{"1.1.1.1", "8.8.8.8", "9.9.9.9"}
	}

	c, err := scamper.NewSocketClient(scamper.WithSocketPath(*socketPath))
	if err != nil {
		log.Fatalf("new socket client: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cfg := scamper.NewTraceConfig().
		Protocol(scamper.ProtocolUDP).
		FirstHop(1).
		MaxHops(16).
		Wait(time.Second).
		ProbesPerHop(1)

	results, err := c.TraceBatch(ctx, targets,
		scamper.WithConcurrency(*concurrency),
		scamper.WithBatchTraceConfig(cfg),
		scamper.WithBatchTimeout(20*time.Second),
		scamper.WithBatchRetry(1),
		scamper.WithProgress(func(done, total int) {
			log.Printf("progress %d/%d", done, total)
		}),
	)
	if err != nil {
		log.Fatalf("trace batch: %v", err)
	}

	for _, r := range results {
		if r.Error != nil {
			fmt.Printf("%-15s  ERROR: %v\n", r.Target, r.Error)
			continue
		}
		fmt.Printf("%-15s  %-12s  hops=%-3d  %s\n", r.Target, r.StopReason, len(r.Hops), r.Duration)
	}
}
