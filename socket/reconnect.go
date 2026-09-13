package socket

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/kanechoo/scamper-client-go/internal/core"
)

// dial 建立一条 control 连接并完成 attach 握手。
func (s *session) dial(ctx context.Context) (*connection, error) {
	var d net.Dialer
	nc, err := d.DialContext(ctx, "unix", s.opts.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("%w: dial %s: %v", core.ErrDaemonUnavailable, s.opts.SocketPath, err)
	}
	c := newConnection(s, s.nextGen(), nc)
	if err := c.attach(ctx); err != nil {
		_ = nc.Close()
		return nil, err
	}
	return c, nil
}

// nextGen 递增并返回连接代数，用于日志与隔离观察。
func (s *session) nextGen() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gen++
	return s.gen
}

// supervise 维持连接：连接断开后按 ReconnectPolicy 指数退避重连。
func (s *session) supervise(initial *connection) {
	policy := s.opts.Reconnect.Normalized()
	backoff := policy.MinBackoff
	cur := initial

	for {
		go cur.writeLoop()
		go cur.readLoop()

		select {
		case <-cur.dead:
			s.log.Warn("scamper: control connection lost", "err", cur.err, "gen", cur.gen)
		case <-s.closeCh:
			cur.fail(&core.ClientClosedError{})
			return
		}
		s.clearConn(cur)

		for {
			if !s.sleep(backoff) {
				return
			}
			ctx := context.Background()
			var cancel context.CancelFunc
			if s.opts.DialTimeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, s.opts.DialTimeout)
			}
			c, err := s.dial(ctx)
			if cancel != nil {
				cancel()
			}
			if err != nil {
				s.log.Warn("scamper: reconnect failed", "err", err, "backoff", backoff)
				backoff = policy.Next(backoff)
				continue
			}
			backoff = policy.MinBackoff
			cur = c
			s.setConn(c)
			if s.opts.Metrics != nil {
				s.opts.Metrics.OnReconnect()
			}
			break
		}
	}
}

// setConn 发布当前可用连接并唤醒所有等待者。
func (s *session) setConn(c *connection) {
	s.mu.Lock()
	s.conn = c
	close(s.readyCh)
	s.mu.Unlock()
}

// clearConn 撤销当前连接并换上一个未就绪的 ready 通道。
func (s *session) clearConn(c *connection) {
	s.mu.Lock()
	if s.conn == c {
		s.conn = nil
		s.readyCh = make(chan struct{})
	}
	s.mu.Unlock()
}

// waitReady 等待一条可用连接，直到 ctx 结束或客户端关闭。
func (s *session) waitReady(ctx context.Context) (*connection, error) {
	for {
		s.mu.Lock()
		if s.conn != nil {
			c := s.conn
			s.mu.Unlock()
			return c, nil
		}
		ch := s.readyCh
		s.mu.Unlock()

		select {
		case <-ch:
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.closeCh:
			return nil, &core.ClientClosedError{}
		}
	}
}

// sleep 等待 d，可被 Close 中断；返回 false 表示客户端已关闭。
func (s *session) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-s.closeCh:
		return false
	}
}
