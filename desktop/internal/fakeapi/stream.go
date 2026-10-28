package fakeapi

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

type stream struct {
	mu    sync.Mutex
	queue [][]byte
	wake  chan struct{}
	done  chan struct{}
	once  sync.Once
}

func newStream() *stream {
	return &stream{wake: make(chan struct{}, 1), done: make(chan struct{})}
}

func (st *stream) push(frame []byte) {
	st.mu.Lock()
	st.queue = append(st.queue, frame)
	st.mu.Unlock()
	select {
	case st.wake <- struct{}{}:
	default:
	}
}

func (st *stream) take() [][]byte {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := st.queue
	st.queue = nil
	return out
}

func (st *stream) close() {
	st.once.Do(func() { close(st.done) })
}

type chatHub struct {
	seq     uint64
	streams map[*stream]struct{}
}

func newChatHub() chatHub {
	return chatHub{streams: map[*stream]struct{}{}}
}

func dataFrame(v any) []byte {
	data, _ := json.Marshal(v)
	return append(append([]byte("data: "), data...), '\n', '\n')
}

func (s *Server) emitLocked(ev api.ChatEvent) {
	s.hub.seq++
	ev.Seq = s.hub.seq
	frame := dataFrame(ev)
	for st := range s.hub.streams {
		st.push(frame)
	}
}

func (s *Server) Emit(ev api.ChatEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emitLocked(ev)
}

func (s *Server) DropStreams() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for st := range s.hub.streams {
		st.close()
		delete(s.hub.streams, st)
	}
}

func (s *Server) Streams() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.hub.streams)
}

func (s *Server) chatStream(w http.ResponseWriter, r *http.Request) {
	st := newStream()
	s.mu.Lock()
	s.hub.streams[st] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.hub.streams, st)
		s.mu.Unlock()
	}()

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, ": ok\n\n")
	s.pump(w, r, st)
}

func (s *Server) pump(w http.ResponseWriter, r *http.Request, st *stream) {
	flusher, _ := w.(http.Flusher)
	flush := func() {
		for _, frame := range st.take() {
			_, _ = w.Write(frame)
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	flush()
	for {
		select {
		case <-st.wake:
			flush()
		case <-st.done:
			flush()
			return
		case <-r.Context().Done():
			return
		case <-s.stop:
			return
		}
	}
}
