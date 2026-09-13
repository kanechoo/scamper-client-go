package socket

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kanechoo/scamper-client-go/internal/core"
	"github.com/kanechoo/scamper-client-go/internal/parser"
)

// jobResult 是一次任务尝试的最终结果。
type jobResult struct {
	res *core.TraceResult
	err error
}

// job 表示一次已提交（或即将提交）给 daemon 的任务。
//
// 它同时被 writer（登记 acceptQueue）、reader（绑定 id、投递结果）与调用方
// （等待结果、超时取消）访问，状态由 mu 保护。
type job struct {
	target string
	ch     chan jobResult

	mu       sync.Mutex
	done     bool
	canceled bool
	conn     *connection
	id       uint64
	hasID    bool
}

func newJob(target string) *job {
	return &job{target: target, ch: make(chan jobResult, 1)}
}

// complete 只交付一次结果；已交付则忽略后续调用。
func (j *job) complete(res *core.TraceResult, err error) {
	j.mu.Lock()
	if j.done {
		j.mu.Unlock()
		return
	}
	j.done = true
	j.mu.Unlock()
	j.ch <- jobResult{res: res, err: err}
}

// setConn 记录任务所属连接（在写入前调用）。
func (j *job) setConn(c *connection) {
	j.mu.Lock()
	j.conn = c
	j.mu.Unlock()
}

// setID 记录 daemon 分配的 id（在收到 OK 时调用）。
func (j *job) setID(c *connection, id uint64) {
	j.mu.Lock()
	j.conn = c
	j.id = id
	j.hasID = true
	j.mu.Unlock()
}

// canceledState 报告任务是否已被取消（超时/中止）。
func (j *job) canceledState() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.canceled
}

// markCanceled 标记任务取消并返回其连接/id 状态，供发送 halt 使用。
func (j *job) markCanceled() (c *connection, id uint64, hasID, done bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.canceled = true
	return j.conn, j.id, j.hasID, j.done
}

// writeReq 是交给单 writer 的一行写入请求；job 非 nil 表示这是一次任务提交。
type writeReq struct {
	line []byte
	job  *job
}

// connection 表示一条已 attach 的 control 连接及其任务映射。
//
// 每个连接拥有独立的回复队列与 id→job 映射，因此旧连接的 reader 不会污染
// 新连接的状态；这正是 generation 隔离的落点。
type connection struct {
	sess *session
	gen  uint64
	nc   net.Conn
	br   *bufio.Reader
	bw   *bufio.Writer

	writeCh chan writeReq
	dead    chan struct{}
	once    sync.Once
	err     error

	mu     sync.Mutex
	replyQ []*job // FIFO：每个待回复的控制命令一项；nil 表示 halt 等无结果命令
	ids    map[uint64]*job
}

func newConnection(s *session, gen uint64, nc net.Conn) *connection {
	return &connection{
		sess:    s,
		gen:     gen,
		nc:      nc,
		br:      bufio.NewReaderSize(nc, 64*1024),
		bw:      bufio.NewWriterSize(nc, 64*1024),
		writeCh: make(chan writeReq, 256),
		dead:    make(chan struct{}),
		ids:     make(map[uint64]*job),
	}
}

// attach 执行 "attach format json" 握手，并等待 OK。
func (c *connection) attach(ctx context.Context) error {
	if dl, ok := ctx.Deadline(); ok {
		_ = c.nc.SetDeadline(dl)
		defer func() { _ = c.nc.SetDeadline(time.Time{}) }()
	}
	if _, err := c.bw.WriteString(attachCommand); err != nil {
		return fmt.Errorf("%w: attach write: %v", core.ErrDaemonUnavailable, err)
	}
	if err := c.bw.Flush(); err != nil {
		return fmt.Errorf("%w: attach flush: %v", core.ErrDaemonUnavailable, err)
	}
	for {
		line, err := c.br.ReadString('\n')
		if err != nil {
			return fmt.Errorf("%w: attach read: %v", core.ErrDaemonUnavailable, err)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "OK", strings.HasPrefix(line, "OK "+idPrefix):
			return nil
		case line == "MORE":
			continue
		case strings.HasPrefix(line, "ERR"):
			return &core.CommandError{
				Args:    []string{"attach", "format", "json"},
				Message: strings.TrimSpace(strings.TrimPrefix(line, "ERR")),
			}
		default:
			c.sess.log.Debug("scamper: unexpected line during attach", "line", line)
		}
	}
}

// writeLoop 是唯一的写者：串行写出命令与 halt，写前登记 acceptQueue。
func (c *connection) writeLoop() {
	for {
		select {
		case <-c.dead:
			return
		case req := <-c.writeCh:
			// 单 writer 保证写出顺序 = daemon 接受/回复顺序；halt 也登记一个
			// nil 占位，避免其 ERR 回复被误配给后续的 trace 任务。
			c.trackReply(req.job)
			if req.job != nil {
				req.job.setConn(c)
			}
			if _, err := c.bw.Write(req.line); err != nil {
				c.fail(err)
				return
			}
			if err := c.bw.Flush(); err != nil {
				c.fail(err)
				return
			}
		}
	}
}

// readLoop 是唯一的读者：逐行解析帧头，DATA 负载按长度读满后投递。
func (c *connection) readLoop() {
	for {
		line, err := c.br.ReadString('\n')
		if err != nil {
			c.fail(err)
			return
		}
		if err := c.handleLine(strings.TrimRight(line, "\r\n")); err != nil {
			c.fail(err)
			return
		}
	}
}

// handleLine 分派一行帧头。
func (c *connection) handleLine(line string) error {
	switch {
	case line == "":
		// 帧间空行（如某些版本的 DATA 负载后附带的换行）直接忽略。
		return nil
	case line == "MORE" || line == "OK":
		// MORE 表示有并行容量；无 id 的 OK 是握手/无 id 确认，运行期忽略。
		return nil
	case strings.HasPrefix(line, "OK "+idPrefix):
		id, err := parseID(line[len("OK "+idPrefix):])
		if err != nil {
			return err
		}
		c.bindID(id)
		return nil
	case strings.HasPrefix(line, "OK halted"):
		// halt 的确认；消费其占位，保持回复队列对齐。
		c.consumeReply(line)
		return nil
	case strings.HasPrefix(line, "ERR"):
		c.failNextReply(&core.CommandError{
			Message: strings.TrimSpace(strings.TrimPrefix(line, "ERR")),
		})
		return nil
	case strings.HasPrefix(line, "DATA"):
		return c.handleData(line)
	default:
		c.sess.log.Debug("scamper: ignoring unknown control line", "line", line)
		return nil
	}
}

// handleData 读取一个 DATA 帧并投递给对应任务。
//
// 无 id 的 DATA（cycle-start / cycle-stop 等）只读负载后丢弃；INPROGRESS
// 的中间结果保留映射，等待最终结果。
func (c *connection) handleData(header string) error {
	fields := strings.Fields(header)
	if len(fields) < 2 {
		return fmt.Errorf("%w: malformed DATA header %q", core.ErrProtocol, header)
	}
	n, err := strconv.Atoi(fields[1])
	if err != nil || n < 0 {
		return fmt.Errorf("%w: bad DATA length in %q", core.ErrProtocol, header)
	}

	var id uint64
	hasID := false
	if len(fields) >= 3 && strings.HasPrefix(fields[2], idPrefix) {
		id, err = parseID(fields[2])
		if err != nil {
			return err
		}
		hasID = true
	}

	payload := make([]byte, n)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		return fmt.Errorf("%w: short DATA payload: %v", core.ErrProtocol, err)
	}
	// 不使用 Peek 去“探测性”消费负载后的换行：当底层已无数据且连接仍打开时，
	// Peek 会阻塞，导致任务直到超时才返回。scamper 的 DATA 负载后不追加换行；
	// 即便某些版本追加，也只会多出一个空行，由 handleLine 忽略。
	if !hasID {
		return nil
	}
	j := c.lookup(id)
	if j == nil {
		c.sess.log.Debug("scamper: DATA for unknown id", "id", id)
		return nil
	}

	res, perr := parser.ParseTrace(payload, j.target)
	if perr != nil {
		c.take(id)
		j.complete(nil, perr)
		return nil
	}
	if res.StopReason == inProgressReason {
		return nil
	}
	c.take(id)
	j.complete(res, nil)
	return nil
}

// trackReply 把一次写出的控制命令登记到 FIFO 回复队列（job 可为 nil）。
//
// 单 writer 保证写出顺序 = daemon 回复顺序，因此 OK/ERR 与队列严格按序对应。
func (c *connection) trackReply(j *job) {
	c.mu.Lock()
	c.replyQ = append(c.replyQ, j)
	c.mu.Unlock()
}

// bindID 把 daemon 返回的 id 绑定到回复队列队首的 trace 任务。
func (c *connection) bindID(id uint64) {
	c.mu.Lock()
	var j *job
	if len(c.replyQ) > 0 {
		j = c.replyQ[0]
		c.replyQ = c.replyQ[1:]
	}
	if j != nil {
		c.ids[id] = j
	}
	c.mu.Unlock()

	if j == nil {
		c.sess.log.Warn("scamper: OK id with no pending trace command", "id", id)
		return
	}
	j.setID(c, id)
	if j.canceledState() {
		c.enqueueHalt(id)
	}
}

// consumeReply 消费一个非任务命令（如 halt）的回复占位。
func (c *connection) consumeReply(line string) {
	c.mu.Lock()
	var j *job
	if len(c.replyQ) > 0 {
		j = c.replyQ[0]
		c.replyQ = c.replyQ[1:]
	}
	c.mu.Unlock()
	if j != nil {
		c.sess.log.Warn("scamper: OK for halt while a trace reply was pending", "line", line)
	}
}

// failNextReply 把命令被拒（ERR）的错误交给回复队列队首的 trace 任务。
//
// 队首为 nil 表示该 ERR 属于 halt 等无结果命令，直接忽略，避免误伤 trace。
func (c *connection) failNextReply(err error) {
	c.mu.Lock()
	var j *job
	if len(c.replyQ) > 0 {
		j = c.replyQ[0]
		c.replyQ = c.replyQ[1:]
	}
	c.mu.Unlock()
	if j == nil {
		c.sess.log.Debug("scamper: ERR for a non-trace command", "err", err)
		return
	}
	j.complete(nil, err)
}

func (c *connection) lookup(id uint64) *job {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ids[id]
}

func (c *connection) take(id uint64) *job {
	c.mu.Lock()
	defer c.mu.Unlock()
	j := c.ids[id]
	delete(c.ids, id)
	return j
}

// enqueueHalt 尽力发送 halt；写队列繁忙或连接已死时直接放弃。
func (c *connection) enqueueHalt(id uint64) {
	line := []byte("halt " + strconv.FormatUint(id, 10) + "\n")
	select {
	case c.writeCh <- writeReq{line: line}:
	case <-c.dead:
	default:
		c.sess.log.Warn("scamper: halt dropped, write queue busy", "id", id)
	}
}

// fail 关闭连接并把所有在途任务置为可重试错误；只执行一次。
func (c *connection) fail(cause error) {
	c.once.Do(func() {
		c.err = cause
		_ = c.nc.Close()

		c.mu.Lock()
		queued := c.replyQ
		bound := c.ids
		c.replyQ = nil
		c.ids = make(map[uint64]*job)
		c.mu.Unlock()

		var jobErr error
		switch {
		case errors.Is(cause, core.ErrClosed):
			jobErr = &core.ClientClosedError{}
		case errors.Is(cause, core.ErrProtocol):
			jobErr = fmt.Errorf("%w: %v", core.ErrProtocol, cause)
		case errors.Is(cause, core.ErrParse):
			jobErr = fmt.Errorf("%w: %v", core.ErrParse, cause)
		default:
			jobErr = fmt.Errorf("%w: %v", core.ErrDaemonUnavailable, cause)
		}
		// 先投递在途任务结果，再关闭 dead 通道；否则等待者可能先观察到
		// dead 而拿到语义较弱的 ErrDaemonUnavailable。
		for _, j := range queued {
			if j != nil {
				j.complete(nil, jobErr)
			}
		}
		for _, j := range bound {
			j.complete(nil, jobErr)
		}
		close(c.dead)
	})
}

// session 管理一条（任意时刻）control 连接及其上的任务调度。
type session struct {
	opts core.ClientOptions
	log  *slog.Logger

	closeCh   chan struct{}
	closeOnce sync.Once
	closed    atomic.Bool

	sem chan struct{}

	mu      sync.Mutex
	conn    *connection
	readyCh chan struct{}
	gen     uint64
}

func newSession(opts core.ClientOptions) (*session, error) {
	s := &session{
		opts:    opts,
		log:     opts.Logger,
		closeCh: make(chan struct{}),
		readyCh: make(chan struct{}),
	}
	if s.log == nil {
		s.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if opts.MaxInflight > 0 {
		s.sem = make(chan struct{}, opts.MaxInflight)
	}

	ctx := context.Background()
	if opts.DialTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.DialTimeout)
		defer cancel()
	}
	c, err := s.dial(ctx)
	if err != nil {
		return nil, err
	}
	s.setConn(c)
	go s.supervise(c)
	return s, nil
}

// trace 把一次任务交给单 writer，并等待 reader 回填结果或 ctx 结束。
func (s *session) trace(ctx context.Context, target string, argv []string) (*core.TraceResult, error) {
	if s.closed.Load() {
		return nil, &core.ClientClosedError{}
	}

	if s.sem != nil {
		select {
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.closeCh:
			return nil, &core.ClientClosedError{}
		}
	}

	c, err := s.waitReady(ctx)
	if err != nil {
		return nil, err
	}

	j := newJob(target)
	req := writeReq{line: []byte(strings.Join(argv, " ") + "\n"), job: j}

	select {
	case c.writeCh <- req:
	case <-c.dead:
		return nil, fmt.Errorf("%w: connection closed before submit", core.ErrDaemonUnavailable)
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.closeCh:
		return nil, &core.ClientClosedError{}
	}

	select {
	case r := <-j.ch:
		return r.res, r.err
	case <-ctx.Done():
		s.halt(j)
		return nil, ctx.Err()
	case <-c.dead:
		// 连接断开：优先取回 fail 投递的具体错误（例如 ErrProtocol）。
		select {
		case r := <-j.ch:
			return r.res, r.err
		default:
		}
		s.halt(j)
		return nil, fmt.Errorf("%w: connection closed while awaiting result", core.ErrDaemonUnavailable)
	case <-s.closeCh:
		s.halt(j)
		return nil, &core.ClientClosedError{}
	}
}

// halt 在任务尚未拿到 id 时只标记取消（拿到 id 后由 bindID 触发 halt）。
func (s *session) halt(j *job) {
	c, id, hasID, done := j.markCanceled()
	if done || !hasID || c == nil {
		return
	}
	c.enqueueHalt(id)
}

// Close 取消 supervisor、关闭连接并唤醒所有等待者。
func (s *session) Close() error {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		close(s.closeCh)
		s.mu.Lock()
		c := s.conn
		s.mu.Unlock()
		if c != nil {
			c.fail(&core.ClientClosedError{})
		}
	})
	return nil
}
