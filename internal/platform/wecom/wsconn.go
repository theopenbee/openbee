package wecom

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Protocol command constants — WeCom AI Bot WebSocket wire commands.
const (
	WsCmdSubscribe         = "aibot_subscribe"
	WsCmdHeartbeat         = "ping"
	WsCmdCallback          = "aibot_msg_callback"
	WsCmdEventCallback     = "aibot_event_callback"
	WsCmdResponse          = "aibot_respond_msg"
	WsCmdSendMsg           = "aibot_send_msg"
	WsCmdUploadMediaInit   = "aibot_upload_media_init"
	WsCmdUploadMediaChunk  = "aibot_upload_media_chunk"
	WsCmdUploadMediaFinish = "aibot_upload_media_finish"
)

// Default connection parameters.
const (
	wsDefaultURL           = "wss://openws.work.weixin.qq.com"
	wsDefaultHeartbeat     = 30 * time.Second
	wsDefaultMaxReconnect  = 100
	wsDefaultReconnectBase = 1 * time.Second
	wsDefaultReconnectMax  = 30 * time.Second
	wsDefaultAckTimeout    = 5 * time.Second
	wsMaxMissedPong        = 2
)

// WsFrame is a single JSON message on the WeCom WebSocket wire.
type WsFrame struct {
	Cmd     string          `json:"cmd,omitempty"`
	Headers WsFrameHeaders  `json:"headers"`
	Body    json.RawMessage `json:"body,omitempty"`
	ErrCode int             `json:"errcode,omitempty"`
	ErrMsg  string          `json:"errmsg,omitempty"`
}

// WsFrameHeaders carries the req_id used to correlate requests and acks.
type WsFrameHeaders struct {
	ReqID string `json:"req_id"`
}

// generateReqID creates a unique request ID with the given prefix.
// Format: <prefix>-<unix_ms>-<random_hex>.
func generateReqID(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UnixMilli(), hex.EncodeToString(b))
}

// WsConnConfig configures a WsConn. Zero values use defaults.
type WsConnConfig struct {
	BotID                string
	Secret               string
	URL                  string        // default: wss://openws.work.weixin.qq.com
	HeartbeatInterval    time.Duration // default: 30s
	MaxReconnectAttempts int           // default: 100
	ReconnectBaseDelay   time.Duration // default: 1s
	ReplyAckTimeout      time.Duration // default: 5s
}

// replyEntry is a single item in the per-req_id reply queue.
type replyEntry struct {
	frame WsFrame
	done  chan struct{}
	resp  WsFrame
	err   error
}

// WsConn manages a persistent WebSocket connection to the WeCom AI Bot server.
// It handles auth, heartbeat, reconnection, and serialised reply queues.
type WsConn struct {
	cfg WsConnConfig

	mu             sync.Mutex
	conn           *websocket.Conn
	connGeneration int
	isManualClose  bool
	missedPong     int
	reconnectCount int

	queueMu sync.Mutex
	queues  map[string][]*replyEntry // per req_id send queue
	pending map[string]*replyEntry   // req_ids awaiting ack

	// Callbacks — set before Connect().
	OnAuthenticated func()
	OnMessage       func(WsFrame)
}

// NewWsConn creates a WsConn with defaults applied.
func NewWsConn(cfg WsConnConfig) *WsConn {
	if cfg.URL == "" {
		cfg.URL = wsDefaultURL
	}
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = wsDefaultHeartbeat
	}
	if cfg.MaxReconnectAttempts == 0 {
		cfg.MaxReconnectAttempts = wsDefaultMaxReconnect
	}
	if cfg.ReconnectBaseDelay == 0 {
		cfg.ReconnectBaseDelay = wsDefaultReconnectBase
	}
	if cfg.ReplyAckTimeout == 0 {
		cfg.ReplyAckTimeout = wsDefaultAckTimeout
	}
	return &WsConn{
		cfg:     cfg,
		queues:  make(map[string][]*replyEntry),
		pending: make(map[string]*replyEntry),
	}
}

// Connect establishes and maintains the WebSocket connection until ctx is cancelled.
// It blocks until ctx.Done().
func (c *WsConn) Connect(ctx context.Context) error {
	if err := c.dialAndAuth(ctx); err != nil {
		return fmt.Errorf("wecom initial connect: %w", err)
	}
	<-ctx.Done()
	c.mu.Lock()
	c.isManualClose = true
	if c.conn != nil {
		c.conn.Close()
	}
	c.mu.Unlock()
	return nil
}

// dialAndAuth opens a new WebSocket, sends auth, waits for ack, then starts
// heartbeat and disconnect-watcher goroutines.
func (c *WsConn) dialAndAuth(ctx context.Context) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.cfg.URL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connGeneration++
	gen := c.connGeneration
	c.missedPong = 0
	c.mu.Unlock()

	readDone := make(chan struct{})
	authDone := make(chan error, 1)
	go c.readLoop(ctx, conn, gen, readDone, authDone)

	// Send auth frame.
	authReqID := generateReqID(WsCmdSubscribe)
	authBody, _ := json.Marshal(map[string]string{"bot_id": c.cfg.BotID, "secret": c.cfg.Secret})
	if err := c.sendRaw(conn, WsFrame{
		Cmd:     WsCmdSubscribe,
		Headers: WsFrameHeaders{ReqID: authReqID},
		Body:    authBody,
	}); err != nil {
		conn.Close()
		return fmt.Errorf("send auth: %w", err)
	}

	// Wait for auth ack.
	select {
	case err := <-authDone:
		if err != nil {
			conn.Close()
			return fmt.Errorf("auth: %w", err)
		}
	case <-ctx.Done():
		conn.Close()
		return ctx.Err()
	case <-time.After(10 * time.Second):
		conn.Close()
		return fmt.Errorf("auth timeout")
	}

	go c.heartbeatLoop(ctx, conn, gen)
	go c.watchDisconnect(ctx, readDone)
	return nil
}

// sendRaw JSON-encodes and writes a frame to conn. Caller must not hold c.mu.
func (c *WsConn) sendRaw(conn *websocket.Conn, frame WsFrame) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// readLoop reads frames from conn and routes them.
func (c *WsConn) readLoop(ctx context.Context, conn *websocket.Conn, gen int, done chan struct{}, authDone chan<- error) {
	defer close(done)
	authSignaled := false
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			manual := c.isManualClose
			c.mu.Unlock()
			if !manual {
				slog.Warn("wecom ws read error", "component", "wecom", "error", err)
			}
			if !authSignaled {
				authDone <- fmt.Errorf("read error before auth: %w", err)
			}
			return
		}
		var frame WsFrame
		if err := json.Unmarshal(data, &frame); err != nil {
			slog.Warn("wecom ws parse error", "component", "wecom", "error", err)
			continue
		}
		c.handleFrame(frame, authDone, &authSignaled)
	}
}

// handleFrame routes a received frame to the correct handler.
func (c *WsConn) handleFrame(frame WsFrame, authDone chan<- error, authSignaled *bool) {
	reqID := frame.Headers.ReqID

	switch frame.Cmd {
	case WsCmdCallback:
		slog.Info("wecom message received", "component", "wecom", "reqId", reqID)
		if c.OnMessage != nil {
			c.OnMessage(frame)
		}
		return
	case WsCmdEventCallback:
		// Event callbacks (enter_chat, template_card_event, etc.) are out of scope.
		slog.Debug("wecom event callback dropped (out of scope)", "component", "wecom", "reqId", reqID)
		return
	}

	// No cmd = ack frame; identify by req_id prefix.
	switch {
	case strings.HasPrefix(reqID, WsCmdSubscribe):
		if !*authSignaled {
			*authSignaled = true
			if frame.ErrCode != 0 {
				authDone <- fmt.Errorf("errcode=%d msg=%s", frame.ErrCode, frame.ErrMsg)
				return
			}
			slog.Info("wecom authenticated", "component", "wecom")
			if c.OnAuthenticated != nil {
				c.OnAuthenticated()
			}
			authDone <- nil
		}
	case strings.HasPrefix(reqID, WsCmdHeartbeat):
		if frame.ErrCode != 0 {
			slog.Warn("wecom heartbeat ack error", "component", "wecom", "errcode", frame.ErrCode)
			return
		}
		c.mu.Lock()
		c.missedPong = 0
		c.mu.Unlock()
		slog.Debug("wecom heartbeat ack", "component", "wecom")
	default:
		c.releaseReplyAck(reqID, frame)
	}
}

// heartbeatLoop sends a ping every HeartbeatInterval.
// If wsMaxMissedPong consecutive pings are unanswered, it force-closes the connection.
func (c *WsConn) heartbeatLoop(ctx context.Context, conn *websocket.Conn, gen int) {
	ticker := time.NewTicker(c.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			if c.connGeneration != gen {
				c.mu.Unlock()
				return
			}
			if c.missedPong >= wsMaxMissedPong {
				c.mu.Unlock()
				slog.Warn("wecom heartbeat timeout, force-closing", "component", "wecom", "missed", c.missedPong)
				conn.Close()
				return
			}
			c.missedPong++
			c.mu.Unlock()

			reqID := generateReqID(WsCmdHeartbeat)
			if err := c.sendRaw(conn, WsFrame{
				Cmd:     WsCmdHeartbeat,
				Headers: WsFrameHeaders{ReqID: reqID},
			}); err != nil {
				slog.Warn("wecom heartbeat send failed", "component", "wecom", "error", err)
				conn.Close()
				return
			}
		}
	}
}

// watchDisconnect triggers reconnect when readLoop exits unexpectedly.
func (c *WsConn) watchDisconnect(ctx context.Context, readDone <-chan struct{}) {
	select {
	case <-ctx.Done():
		return
	case <-readDone:
		c.mu.Lock()
		manual := c.isManualClose
		c.mu.Unlock()
		if !manual {
			c.scheduleReconnect(ctx)
		}
	}
}

// scheduleReconnect waits with exponential backoff and re-dials.
func (c *WsConn) scheduleReconnect(ctx context.Context) {
	c.mu.Lock()
	c.reconnectCount++
	attempt := c.reconnectCount
	c.mu.Unlock()

	if c.cfg.MaxReconnectAttempts != -1 && attempt > c.cfg.MaxReconnectAttempts {
		slog.Error("wecom max reconnect attempts exceeded", "component", "wecom")
		return
	}

	delay := time.Duration(float64(c.cfg.ReconnectBaseDelay) * math.Pow(2, float64(attempt-1)))
	if delay > wsDefaultReconnectMax {
		delay = wsDefaultReconnectMax
	}
	slog.Info("wecom reconnecting", "component", "wecom", "attempt", attempt, "delay", delay)

	select {
	case <-ctx.Done():
		return
	case <-time.After(delay):
	}

	if err := c.dialAndAuth(ctx); err != nil {
		slog.Error("wecom reconnect failed", "component", "wecom", "attempt", attempt, "error", err)
		c.scheduleReconnect(ctx)
	} else {
		slog.Info("wecom reconnected", "component", "wecom", "attempt", attempt)
		c.mu.Lock()
		c.reconnectCount = 0
		c.mu.Unlock()
	}
}

// SendReply enqueues a frame for the given reqID and waits for the server ack.
// Frames with the same reqID are serialised; different reqIDs are independent.
// Each distinct operation (subscribe, heartbeat, each upload step, each reply)
// MUST use its own freshly generated reqID — reusing a reqID across operations
// would serialize them through the same queue slot and deadlock.
func (c *WsConn) SendReply(reqID, cmd string, body any) (WsFrame, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return WsFrame{}, fmt.Errorf("marshal body: %w", err)
	}
	frame := WsFrame{
		Cmd:     cmd,
		Headers: WsFrameHeaders{ReqID: reqID},
		Body:    bodyJSON,
	}
	entry := &replyEntry{
		frame: frame,
		done:  make(chan struct{}),
	}

	c.queueMu.Lock()
	c.queues[reqID] = append(c.queues[reqID], entry)
	start := len(c.queues[reqID]) == 1
	c.queueMu.Unlock()

	if start {
		go c.processQueue(reqID)
	}

	<-entry.done
	return entry.resp, entry.err
}

// processQueue drains the queue for reqID, sending one frame at a time and
// waiting for ack before proceeding.
func (c *WsConn) processQueue(reqID string) {
	for {
		c.queueMu.Lock()
		queue := c.queues[reqID]
		if len(queue) == 0 {
			delete(c.queues, reqID)
			c.queueMu.Unlock()
			return
		}
		entry := queue[0]
		c.queueMu.Unlock()

		// Send the frame.
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn == nil {
			entry.err = fmt.Errorf("not connected")
			close(entry.done)
			c.queueMu.Lock()
			c.queues[reqID] = c.queues[reqID][1:]
			c.queueMu.Unlock()
			continue
		}
		if err := c.sendRaw(conn, entry.frame); err != nil {
			entry.err = err
			close(entry.done)
			c.queueMu.Lock()
			c.queues[reqID] = c.queues[reqID][1:]
			c.queueMu.Unlock()
			continue
		}

		// Register ack listener.
		c.queueMu.Lock()
		c.pending[reqID] = entry
		c.queueMu.Unlock()

		// Wait for ack or timeout.
		select {
		case <-entry.done:
			// ack arrived via releaseReplyAck
		case <-time.After(c.cfg.ReplyAckTimeout):
			c.queueMu.Lock()
			delete(c.pending, reqID)
			c.queueMu.Unlock()
			entry.err = fmt.Errorf("ack timeout (5s) for reqID %s", reqID)
			close(entry.done)
		}

		c.queueMu.Lock()
		c.queues[reqID] = c.queues[reqID][1:]
		c.queueMu.Unlock()
	}
}

// releaseReplyAck resolves the pending ack for reqID with the received frame.
func (c *WsConn) releaseReplyAck(reqID string, frame WsFrame) {
	c.queueMu.Lock()
	entry, ok := c.pending[reqID]
	if ok {
		delete(c.pending, reqID)
	}
	c.queueMu.Unlock()
	if !ok {
		slog.Debug("wecom unexpected ack (ignored)", "component", "wecom", "reqId", reqID)
		return
	}
	if frame.ErrCode != 0 {
		entry.err = fmt.Errorf("ack error: code=%d msg=%s", frame.ErrCode, frame.ErrMsg)
	} else {
		entry.resp = frame
	}
	close(entry.done)
}

// IsConnected reports whether the WebSocket connection is currently open.
func (c *WsConn) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil
}
