package fakeapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const indexHTML = `<!doctype html><html><head><title>OpenBee</title></head><body><div id="root"></div></body></html>`

type Call struct {
	Method string
	Path   string
	Query  string
	Body   []byte
}

type failure struct {
	method string
	path   string
	status int
	body   any
}

type hold struct {
	method  string
	path    string
	release chan struct{}
}

type Server struct {
	URL string

	t    testing.TB
	srv  *httptest.Server
	mux  *http.ServeMux
	stop chan struct{}
	once sync.Once

	mu     sync.Mutex
	st     State
	calls  []Call
	fails  []failure
	holds  []hold
	lastTS int64
	idSeq  int

	tokens  tokenBook
	account account
	hub     chatHub
	reads   map[string]int64
	setups  setupBook
}

func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{
		t:       t,
		mux:     http.NewServeMux(),
		stop:    make(chan struct{}),
		st:      seedState(),
		tokens:  newTokenBook(),
		account: account{createdAt: time.Now().UnixMilli()},
		hub:     newChatHub(),
		reads:   map[string]int64{},
		setups:  newSetupBook(),
	}
	s.routes()
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	s.URL = s.srv.URL
	t.Cleanup(s.shutdown)
	return s
}

func (s *Server) shutdown() {
	s.once.Do(func() {
		close(s.stop)
		s.srv.Close()
	})
}

func (s *Server) routes() {
	s.mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, indexHTML)
	})
	s.authRoutes()
	s.workerRoutes()
	s.chatRoutes()
	s.envRoutes()
	s.pluginRoutes()
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))

	s.mu.Lock()
	s.calls = append(s.calls, Call{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Body: body})
	held := s.takeHold(r)
	s.mu.Unlock()

	if held != nil {
		select {
		case <-held:
		case <-r.Context().Done():
			return
		case <-s.stop:
			return
		}
	}

	s.mu.Lock()
	canned, found := s.takeFailure(r)
	s.mu.Unlock()

	if found {
		canned.write(w)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) takeFailure(r *http.Request) (reply, bool) {
	i := slices.IndexFunc(s.fails, func(f failure) bool {
		return f.method == r.Method && f.path == r.URL.Path
	})
	if i < 0 {
		return reply{}, false
	}
	f := s.fails[i]
	s.fails = slices.Delete(s.fails, i, i+1)
	return reply{status: f.status, body: f.body}, true
}

func (s *Server) takeHold(r *http.Request) chan struct{} {
	i := slices.IndexFunc(s.holds, func(h hold) bool {
		return h.method == r.Method && h.path == r.URL.Path
	})
	if i < 0 {
		return nil
	}
	h := s.holds[i]
	s.holds = slices.Delete(s.holds, i, i+1)
	return h.release
}

func (s *Server) HoldNext(method, path string) (release func()) {
	h := hold{method: strings.ToUpper(method), path: path, release: make(chan struct{})}
	s.mu.Lock()
	s.holds = append(s.holds, h)
	s.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { close(h.release) }) }
}

func (s *Server) FailNext(method, path string, status int, body any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fails = append(s.fails, failure{method: strings.ToUpper(method), path: path, status: status, body: body})
}

func (s *Server) Calls(method, pathPrefix string) []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Call
	for _, c := range s.calls {
		if method != "" && !strings.EqualFold(c.Method, method) {
			continue
		}
		if !strings.HasPrefix(c.Path, pathPrefix) {
			continue
		}
		c.Body = slices.Clone(c.Body)
		out = append(out, c)
	}
	return out
}

type authMode int

const (
	authNone authMode = iota
	authUser
	authPlugins
)

type request struct {
	r *http.Request
}

func (q *request) param(name string) string {
	return q.r.PathValue(name)
}

func (q *request) query(name string) string {
	return q.r.URL.Query().Get(name)
}

func (q *request) body() []byte {
	data, _ := io.ReadAll(q.r.Body)
	return data
}

func (q *request) decode(v any) (reply, bool) {
	data := q.body()
	if len(bytes.TrimSpace(data)) == 0 {
		return errReply(http.StatusBadRequest, "EOF"), false
	}
	if err := json.Unmarshal(data, v); err != nil {
		return errReply(http.StatusBadRequest, err.Error()), false
	}
	return reply{}, true
}

func (q *request) decodeOptional(v any) (reply, bool) {
	data := q.body()
	if len(bytes.TrimSpace(data)) == 0 {
		return reply{}, true
	}
	if err := json.Unmarshal(data, v); err != nil {
		return errReply(http.StatusBadRequest, err.Error()), false
	}
	return reply{}, true
}

type handler func(q *request) reply

func (s *Server) route(pattern string, mode authMode, h handler) {
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		out := func() reply {
			s.mu.Lock()
			defer s.mu.Unlock()
			if denied := s.guard(mode, r); denied.status != 0 {
				return denied
			}
			return h(&request{r: r})
		}()
		out.write(w)
	})
}

func (s *Server) raw(pattern string, mode authMode, h http.HandlerFunc) {
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		denied := func() reply {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.guard(mode, r)
		}()
		if denied.status != 0 {
			denied.write(w)
			return
		}
		h(w, r)
	})
}

type reply struct {
	status int
	body   any
}

func (p reply) write(w http.ResponseWriter) {
	switch body := p.body.(type) {
	case nil:
		w.WriteHeader(p.status)
	case string:
		writeText(w, p.status, []byte(body))
	case []byte:
		writeText(w, p.status, body)
	default:
		data, err := json.Marshal(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(p.status)
		_, _ = w.Write(data)
	}
}

func writeText(w http.ResponseWriter, status int, data []byte) {
	contentType := "text/plain; charset=utf-8"
	if json.Valid(data) {
		contentType = "application/json; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func ok(body any) reply {
	return reply{status: http.StatusOK, body: body}
}

func created(body any) reply {
	return reply{status: http.StatusCreated, body: body}
}

func accepted(body any) reply {
	return reply{status: http.StatusAccepted, body: body}
}

func noContent() reply {
	return reply{status: http.StatusNoContent}
}

func errReply(status int, message string) reply {
	return reply{status: status, body: map[string]any{"error": message}}
}

func codedReply(status int, code, message string, params map[string]any) reply {
	body := map[string]any{"error": message, "code": code}
	if len(params) > 0 {
		body["params"] = params
	}
	return reply{status: status, body: body}
}

func validatorMessage(object, field, tag string) string {
	return fmt.Sprintf("Key: '%s.%s' Error:Field validation for '%s' failed on the '%s' tag", object, field, field, tag)
}
