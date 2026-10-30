package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/sse"
)

const (
	apiPrefix       = "/api"
	eventStreamType = "text/event-stream"
	offlineGrace    = 4 * time.Second
	retryFeedback   = time.Second
	reconnectBase   = 2 * time.Second
	reconnectMax    = 30 * time.Second
	chatIdle        = 75 * time.Second
)

var errStreamResponse = errors.New("unexpected event stream response")

type hook[F any] struct {
	id int
	fn F
}

type hooks[F any] struct {
	next  int
	items []hook[F]
}

func (h *hooks[F]) add(fn F) func() {
	h.next++
	id := h.next
	h.items = append(h.items, hook[F]{id: id, fn: fn})
	return func() {
		h.items = slices.DeleteFunc(h.items, func(x hook[F]) bool { return x.id == id })
	}
}

func (h *hooks[F]) each(call func(F)) {
	for _, x := range slices.Clone(h.items) {
		call(x.fn)
	}
}

func (s *Store) OnMessage(fn func(cid string, m api.ChatMessage)) (off func()) {
	return s.onMessage.add(fn)
}

func (s *Store) OnResync(fn func()) (off func()) {
	return s.onResync.add(fn)
}

func (s *Store) OnSessionReset(fn func()) (off func()) {
	return s.onReset.add(fn)
}

func (s *Store) SetForeground(visible bool) {
	s.foreground = visible
	if !visible || s.Phase != PhaseReady {
		return
	}
	s.closeActive()
	if ch, ok := s.chats[s.active]; ok && ch.Loaded {
		ch.MarkRead(0)
	}
	s.refetchStale()
	s.ReconnectNow()
}

func (s *Store) Foreground() bool {
	return s.foreground
}

func (s *Store) ReconnectNow() {
	if s.Online || s.wake == nil {
		return
	}
	if s.Offline {
		s.retrying = true
		if !s.Reconnecting {
			s.Reconnecting = true
			s.holdReconnecting()
		}
	}
	signal(s.wake)
}

func (s *Store) holdReconnecting() {
	sess := s.sess
	ctx, cancel := context.WithCancel(sess.ctx)
	s.stopHold = cancel
	go func() {
		if s.o.Sleep(ctx, retryFeedback, nil) != nil {
			return
		}
		s.o.Post(func() {
			if s.sess != sess || ctx.Err() != nil {
				return
			}
			s.endHold()
			s.settleReconnect()
		})
	}()
}

func (s *Store) endHold() {
	if s.stopHold != nil {
		s.stopHold()
		s.stopHold = nil
	}
}

func (s *Store) settleReconnect() {
	if !s.Reconnecting || s.retrying || s.stopHold != nil {
		return
	}
	s.Reconnecting = false
	s.ReconnectFailed = true
}

func (s *Store) clearReconnect() {
	s.endHold()
	s.Reconnecting = false
	s.ReconnectFailed = false
	s.retrying = false
}

func (s *Store) Reconnect() {
	if s.wake == nil {
		return
	}
	signal(s.restart)
	signal(s.wake)
}

func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (s *Store) idleTimeout(fallback time.Duration) time.Duration {
	if s.o.StreamIdle > 0 {
		return s.o.StreamIdle
	}
	return fallback
}

func (s *Store) startStream() {
	sess := s.sess
	wake, restart := make(chan struct{}, 1), make(chan struct{}, 1)
	s.wake, s.restart = wake, restart
	s.busy.Add(1)
	go s.runStream(sess, wake, restart)
}

func (s *Store) runStream(sess *session, wake, restart chan struct{}) {
	connecting := true
	settle := func() {
		if connecting {
			connecting = false
			s.busy.Add(-1)
		}
	}
	defer settle()
	post := func(fn func()) {
		s.o.Post(func() {
			if s.sess == sess {
				fn()
			}
		})
	}
	_ = sse.Run(sess.ctx, sse.Options{
		Open: func(ctx context.Context) (io.ReadCloser, error) {
			return openStream(ctx, sess.client, api.ChatStreamPath)
		},
		OnOpen: func() {
			post(s.streamOpened)
			settle()
		},
		OnEvent: func(e sse.Event) {
			if ev, ok := parseEvent(e.Data); ok {
				post(func() { s.handleEvent(ev) })
			}
		},
		OnDrop: func(error, int) {
			post(s.streamDropped)
			settle()
		},
		Backoff:     sse.Backoff{Base: reconnectBase, Max: reconnectMax},
		IdleTimeout: s.idleTimeout(chatIdle),
		Wake:        wake,
		Restart:     restart,
		Sleep: func(ctx context.Context, d time.Duration, wake <-chan struct{}) error {
			err := s.o.Sleep(ctx, d, wake)
			if err == nil {
				connecting = true
				s.busy.Add(1)
			}
			return err
		},
	})
}

func openStream(ctx context.Context, c *api.Client, path string) (io.ReadCloser, error) {
	if err := c.EnsureFresh(ctx); errors.Is(err, api.ErrUnauthorized) {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL()+apiPrefix+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", eventStreamType)
	resp, err := c.DoRaw(ctx, req)
	if err != nil {
		return nil, err
	}
	mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || mt != eventStreamType {
		resp.Body.Close()
		return nil, fmt.Errorf("%w: status %d, content type %q", errStreamResponse, resp.StatusCode, mt)
	}
	return resp.Body, nil
}

func parseEvent(data string) (api.ChatEvent, bool) {
	var frame struct {
		api.ChatEvent
		Seq *uint64 `json:"seq"`
	}
	if json.Unmarshal([]byte(data), &frame) != nil || frame.Seq == nil {
		return api.ChatEvent{}, false
	}
	ev := frame.ChatEvent
	ev.Seq = *frame.Seq
	return ev, true
}

func (s *Store) streamOpened() {
	s.Online = true
	s.Offline = false
	s.endGrace()
	s.clearReconnect()
	s.resync()
}

func (s *Store) streamDropped() {
	s.Online = false
	s.retrying = false
	s.settleReconnect()
	if s.Offline || s.stopGrace != nil {
		return
	}
	sess := s.sess
	ctx, cancel := context.WithCancel(sess.ctx)
	s.stopGrace = cancel
	go func() {
		if s.o.Sleep(ctx, offlineGrace, nil) != nil {
			return
		}
		s.o.Post(func() {
			if s.sess != sess || ctx.Err() != nil {
				return
			}
			s.endGrace()
			s.Offline = true
		})
	}()
}

func (s *Store) endGrace() {
	if s.stopGrace != nil {
		s.stopGrace()
		s.stopGrace = nil
	}
}

func (s *Store) resync() {
	s.lastSeq = nil
	s.invalidateWorkers()
	s.Conversations.Invalidate()
	for _, ch := range s.chats {
		ch.resync()
	}
	s.onResync.each(func(fn func()) { fn() })
}

func (s *Store) handleEvent(ev api.ChatEvent) {
	gap := model.SeqGap(s.lastSeq, ev.Seq)
	eff := model.ApplyEvent(ev, s.summaries(), s.WorkerMap(), s.active, s.foreground)
	s.applyEffect(eff)
	if gap {
		s.resync()
	}
	seq := ev.Seq
	s.lastSeq = &seq
	if ev.Type == "worker_status" {
		s.patchDetailStatus(ev.WorkerID, ev.Status)
	}
	if ev.Type != "message" || ev.Message == nil {
		return
	}
	m := *ev.Message
	if ch, ok := s.chats[ev.ConversationID]; ok {
		ch.receive(m)
	}
	s.onMessage.each(func(fn func(string, api.ChatMessage)) { fn(ev.ConversationID, m) })
	s.attend(ev, eff.RefetchWorkers)
}

func (s *Store) noteSent(cid string, m api.ChatMessage) {
	summaries := s.summaries()
	i := slices.IndexFunc(summaries, func(c api.ConversationSummary) bool { return c.ID == cid })
	if i >= 0 && summaries[i].LastMessage != nil && summaries[i].LastMessage.TS > m.TS {
		return
	}
	s.applyEffect(model.ApplyEvent(api.ChatEvent{Type: "message", ConversationID: cid, Message: &m}, summaries, s.WorkerMap(), s.active, s.foreground))
}

func (s *Store) applyEffect(eff model.EventEffect) {
	if eff.SummariesChanged {
		s.Conversations.patch(eff.Summaries)
		s.syncUnread()
	}
	if eff.RefetchConversations {
		s.Conversations.Invalidate()
	}
	if eff.RefetchWorkers {
		s.invalidateWorkers()
	}
	for id, status := range eff.WorkerStatus {
		s.patchWorkerStatus(id, status)
	}
}

func (s *Store) resetLive() {
	s.Online = false
	s.Offline = false
	s.lastSeq = nil
	s.wake, s.restart = nil, nil
	s.active = ""
	s.endGrace()
	s.clearReconnect()
	s.setUnread(0)
}

func sleep(ctx context.Context, d time.Duration, wake <-chan struct{}) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wake:
		return nil
	case <-t.C:
		return nil
	}
}
