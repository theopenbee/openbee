package pluginchannel

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/logger"
)

const (
	CodeConfigInvalid  = -32001
	CodeMethodNotFound = -32601

	maxLineBytes = 8 << 20
)

var (
	ErrConnClosed  = errors.New("plugin connection closed")
	errLineTooLong = errors.New("protocol line exceeds limit")
)

var log = logger.With(zap.String("component", "pluginchannel"))

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("plugin error %d: %s", e.Code, e.Message)
}

type NotificationHandler func(method string, params json.RawMessage)

type wireMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type outgoing struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type response struct {
	result json.RawMessage
	err    *RPCError
}

type Conn struct {
	r        *bufio.Reader
	w        io.Writer
	onNotify NotificationHandler
	log      *logger.Logger

	writeMu sync.Mutex
	nextID  atomic.Int64

	mu      sync.Mutex
	pending map[int64]chan response
	closed  chan struct{}
}

func NewConn(r io.Reader, w io.Writer, onNotify NotificationHandler, l *logger.Logger) *Conn {
	if l == nil {
		l = log
	}
	return &Conn{
		r:        bufio.NewReaderSize(r, 64<<10),
		w:        w,
		onNotify: onNotify,
		log:      l,
		pending:  make(map[int64]chan response),
		closed:   make(chan struct{}),
	}
}

func (c *Conn) Run() error {
	defer c.shutdown()
	for {
		line, err := c.readLine()
		if errors.Is(err, errLineTooLong) {
			c.log.Warn("dropping oversized protocol line", zap.Int("limit", maxLineBytes))
			continue
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
			return nil
		}
		if err != nil {
			return err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		c.handle(line)
	}
}

func (c *Conn) readLine() ([]byte, error) {
	var buf []byte
	tooLong := false
	for {
		chunk, isPrefix, err := c.r.ReadLine()
		if err != nil {
			return nil, err
		}
		if !tooLong {
			if len(buf)+len(chunk) > maxLineBytes {
				tooLong = true
				buf = nil
			} else {
				buf = append(buf, chunk...)
			}
		}
		if !isPrefix {
			if tooLong {
				return nil, errLineTooLong
			}
			return buf, nil
		}
	}
}

func (c *Conn) handle(line []byte) {
	var msg wireMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		c.log.Warn("ignoring non-JSON protocol line", zap.String("line", truncate(string(line), 200)))
		return
	}
	hasID := len(msg.ID) > 0 && string(msg.ID) != "null"
	switch {
	case msg.Method != "" && hasID:
		_ = c.write(outgoing{JSONRPC: "2.0", ID: msg.ID, Error: &RPCError{Code: CodeMethodNotFound, Message: "method not found: " + msg.Method}})
	case msg.Method != "":
		if c.onNotify != nil {
			c.onNotify(msg.Method, msg.Params)
		}
	case hasID:
		var id int64
		if err := json.Unmarshal(msg.ID, &id); err != nil {
			return
		}
		c.mu.Lock()
		ch, ok := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if ok {
			ch <- response{result: msg.Result, err: msg.Error}
		}
	}
}

func (c *Conn) shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.closed:
		return
	default:
	}
	close(c.closed)
	for id, ch := range c.pending {
		delete(c.pending, id)
		close(ch)
	}
}

func (c *Conn) write(m outgoing) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.w.Write(data)
	return err
}

func (c *Conn) Call(ctx context.Context, method string, params, result any) error {
	id := c.nextID.Add(1)
	ch := make(chan response, 1)
	c.mu.Lock()
	select {
	case <-c.closed:
		c.mu.Unlock()
		return ErrConnClosed
	default:
	}
	c.pending[id] = ch
	c.mu.Unlock()

	written := make(chan error, 1)
	go func() { written <- c.write(outgoing{JSONRPC: "2.0", ID: id, Method: method, Params: params}) }()
	select {
	case err := <-written:
		if err != nil {
			c.forget(id)
			return fmt.Errorf("%w: %v", ErrConnClosed, err)
		}
	case <-ctx.Done():
		c.forget(id)
		go func() {
			if <-written == nil {
				c.cancelRemote(id)
			}
		}()
		return ctx.Err()
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return ErrConnClosed
		}
		if resp.err != nil {
			return resp.err
		}
		if result != nil && len(resp.result) > 0 {
			if err := json.Unmarshal(resp.result, result); err != nil {
				return fmt.Errorf("decode %s result: %w", method, err)
			}
		}
		return nil
	case <-ctx.Done():
		c.forget(id)
		go c.cancelRemote(id)
		return ctx.Err()
	}
}

func (c *Conn) cancelRemote(id int64) {
	_ = c.Notify("$/cancelRequest", map[string]int64{"id": id})
}

func (c *Conn) forget(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Conn) Notify(method string, params any) error {
	return c.write(outgoing{JSONRPC: "2.0", Method: method, Params: params})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
