package pluginchannel

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type peer struct {
	t      *testing.T
	in     *bufio.Reader
	out    io.WriteCloser
	toHost io.WriteCloser
}

type notification struct {
	method string
	params json.RawMessage
}

func newConnPair(t *testing.T) (*Conn, *peer, <-chan notification, <-chan error) {
	t.Helper()
	hostR, peerW := io.Pipe()
	peerR, hostW := io.Pipe()
	notes := make(chan notification, 16)
	conn := NewConn(hostR, hostW, func(method string, params json.RawMessage) {
		notes <- notification{method, params}
	}, nil)
	done := make(chan error, 1)
	go func() { done <- conn.Run() }()
	t.Cleanup(func() {
		peerW.Close()
		hostW.Close()
	})
	return conn, &peer{t: t, in: bufio.NewReaderSize(peerR, 1<<20), out: peerW, toHost: peerW}, notes, done
}

func (p *peer) readMessage() map[string]any {
	p.t.Helper()
	line, err := p.in.ReadString('\n')
	require.NoError(p.t, err)
	var m map[string]any
	require.NoError(p.t, json.Unmarshal([]byte(line), &m))
	return m
}

func (p *peer) write(s string) {
	p.t.Helper()
	_, err := io.WriteString(p.out, s+"\n")
	require.NoError(p.t, err)
}

func TestConn_CallRoundTrip(t *testing.T) {
	conn, p, _, _ := newConnPair(t)
	resCh := make(chan error, 1)
	var result struct {
		OK bool `json:"ok"`
	}
	go func() { resCh <- conn.Call(context.Background(), "initialize", map[string]any{"a": 1}, &result) }()

	req := p.readMessage()
	assert.Equal(t, "2.0", req["jsonrpc"])
	assert.Equal(t, "initialize", req["method"])
	assert.Equal(t, map[string]any{"a": float64(1)}, req["params"])
	p.write(`{"jsonrpc":"2.0","id":` + jsonNumber(req["id"]) + `,"result":{"ok":true}}`)

	require.NoError(t, <-resCh)
	assert.True(t, result.OK)
}

func jsonNumber(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestConn_CallError(t *testing.T) {
	conn, p, _, _ := newConnPair(t)
	resCh := make(chan error, 1)
	go func() { resCh <- conn.Call(context.Background(), "initialize", nil, nil) }()
	req := p.readMessage()
	p.write(`{"jsonrpc":"2.0","id":` + jsonNumber(req["id"]) + `,"error":{"code":-32001,"message":"bad token"}}`)

	err := <-resCh
	var rpcErr *RPCError
	require.True(t, errors.As(err, &rpcErr), "got %v", err)
	assert.Equal(t, CodeConfigInvalid, rpcErr.Code)
	assert.Equal(t, "bad token", rpcErr.Message)
}

func TestConn_CancelSendsCancelRequest(t *testing.T) {
	conn, p, _, _ := newConnPair(t)
	ctx, cancel := context.WithCancel(context.Background())
	resCh := make(chan error, 1)
	go func() { resCh <- conn.Call(ctx, "send", nil, nil) }()
	req := p.readMessage()
	cancel()

	note := p.readMessage()
	assert.ErrorIs(t, <-resCh, context.Canceled)
	assert.Equal(t, "$/cancelRequest", note["method"])
	assert.Equal(t, map[string]any{"id": req["id"]}, note["params"])
	assert.NotContains(t, note, "id")
}

func TestConn_IgnoresGarbageLines(t *testing.T) {
	conn, p, _, _ := newConnPair(t)
	resCh := make(chan error, 1)
	go func() { resCh <- conn.Call(context.Background(), "send", nil, nil) }()
	req := p.readMessage()
	p.write("hello world")
	p.write(`{"jsonrpc":"2.0","id":"not-ours","result":{}}`)
	p.write(`{"jsonrpc":"2.0","id":` + jsonNumber(req["id"]) + `,"result":{}}`)
	require.NoError(t, <-resCh)
}

func TestConn_DispatchesNotifications(t *testing.T) {
	_, p, notes, _ := newConnPair(t)
	p.write(`{"jsonrpc":"2.0","method":"inbound","params":{"chat_id":"1"}}`)
	select {
	case n := <-notes:
		assert.Equal(t, "inbound", n.method)
		assert.JSONEq(t, `{"chat_id":"1"}`, string(n.params))
	case <-time.After(2 * time.Second):
		t.Fatal("notification not dispatched")
	}
}

func TestConn_OversizedLineSkipped(t *testing.T) {
	conn, p, _, _ := newConnPair(t)
	resCh := make(chan error, 1)
	go func() { resCh <- conn.Call(context.Background(), "send", nil, nil) }()
	req := p.readMessage()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.write(`{"jsonrpc":"2.0","method":"log","params":{"message":"` + strings.Repeat("x", 9<<20) + `"}}`)
		p.write(`{"jsonrpc":"2.0","id":` + jsonNumber(req["id"]) + `,"result":{}}`)
	}()
	require.NoError(t, <-resCh)
	wg.Wait()
}

func TestConn_EOFFailsPending(t *testing.T) {
	conn, p, _, done := newConnPair(t)
	resCh := make(chan error, 1)
	go func() { resCh <- conn.Call(context.Background(), "send", nil, nil) }()
	p.readMessage()
	p.out.Close()

	assert.ErrorIs(t, <-resCh, ErrConnClosed)
	assert.NoError(t, <-done)
	assert.ErrorIs(t, conn.Call(context.Background(), "send", nil, nil), ErrConnClosed)
}

func TestConn_RejectsPeerRequests(t *testing.T) {
	_, p, _, _ := newConnPair(t)
	p.write(`{"jsonrpc":"2.0","id":7,"method":"x"}`)
	resp := p.readMessage()
	assert.Equal(t, float64(7), resp["id"])
	errObj, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(CodeMethodNotFound), errObj["code"])
}

func TestConn_Notify(t *testing.T) {
	conn, p, _, _ := newConnPair(t)
	go func() { _ = conn.Notify("shutdown-hint", map[string]string{"k": "v"}) }()
	msg := p.readMessage()
	assert.Equal(t, "shutdown-hint", msg["method"])
	assert.NotContains(t, msg, "id")
}

func TestConn_CallRespectsContextWhenPeerNotReading(t *testing.T) {
	conn, _, _, _ := newConnPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := conn.Call(ctx, "send", nil, nil)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), time.Second)
}
