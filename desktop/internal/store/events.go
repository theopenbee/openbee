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
	chatStreamURLPath = "/api" + api.ChatStreamPath
	eventStreamType   = "text/event-stream"
	offlineGrace      = 4 * time.Second
	reconnectBase     = 2 * time.Second
	reconnectMax      = 30 * time.Second
)

var errStreamResponse = errors.New("unexpected chat stream response")

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

func (s *Store) SetForeground(visible bool) {
	s.foreground = visible
	if !visible || s.Phase != PhaseReady {
		return
	}
	s.closeActive()
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
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Store) startStream() {
	sess := s.sess
	wake := make(chan struct{}, 1)
	s.wake = wake
	s.busy.Add(1)
	go s.runStream(sess, wake)
}

func (s *Store) runStream(sess *session, wake chan struct{}) {
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
			return openStream(ctx, sess.client)
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
		Backoff: sse.Backoff{Base: reconnectBase, Max: reconnectMax},
		Wake:    wake,
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

func openStream(ctx context.Context, c *api.Client) (io.ReadCloser, error) {
	if err := c.EnsureFresh(ctx); errors.Is(err, api.ErrUnauthorized) {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL()+chatStreamURLPath, nil)
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
	s.resync()
}

func (s *Store) streamDropped() {
	s.Online = false
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
	if ev.Type != "message" || ev.Message == nil {
		return
	}
	m := *ev.Message
	s.onMessage.each(func(fn func(string, api.ChatMessage)) { fn(ev.ConversationID, m) })
	s.attend(ev, eff.RefetchWorkers)
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
	s.wake = nil
	s.active = ""
	s.endGrace()
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
