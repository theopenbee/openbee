package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeTokens struct {
	mu      sync.Mutex
	access  string
	refresh string
	sets    int
}

func (f *fakeTokens) Access() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.access
}

func (f *fakeTokens) Refresh() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.refresh
}

func (f *fakeTokens) SetAccess(token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.access = token
	f.sets++
}

func (f *fakeTokens) setCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sets
}

func (f *fakeTokens) replace(access, refresh string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.access = access
	f.refresh = refresh
}

func makeJWT(exp time.Time) string {
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := enc.EncodeToString([]byte(fmt.Sprintf(`{"type":"access","exp":%d}`, exp.Unix())))
	return header + "." + payload + ".signature"
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func newTestClient(t *testing.T, srv *httptest.Server, tokens TokenSource, onUnauthorized func()) *Client {
	t.Helper()
	return New(Options{
		BaseURL:        srv.URL,
		HTTP:           srv.Client(),
		Tokens:         tokens,
		Language:       func() string { return "zh" },
		OnUnauthorized: onUnauthorized,
	})
}

func TestDoSendsHeaders(t *testing.T) {
	var got http.Header
	var gotBody []byte
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		gotMethod = r.Method
		gotPath = r.URL.RequestURI()
		gotBody, _ = io.ReadAll(r.Body)
		writeJSON(w, 200, `{"ok":true}`)
	}))
	defer srv.Close()

	tokens := &fakeTokens{access: "tok", refresh: "r"}
	c := newTestClient(t, srv, tokens, nil)

	var out struct {
		OK bool `json:"ok"`
	}
	err := c.Do(context.Background(), http.MethodPost, "/workers?x=1", map[string]string{"name": "a"}, &out)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !out.OK {
		t.Fatalf("response not decoded: %+v", out)
	}
	if got.Get("Authorization") != "Bearer tok" {
		t.Fatalf("Authorization = %q", got.Get("Authorization"))
	}
	if got.Get("Accept-Language") != "zh" {
		t.Fatalf("Accept-Language = %q", got.Get("Accept-Language"))
	}
	if got.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q", got.Get("Content-Type"))
	}
	if gotMethod != http.MethodPost || gotPath != "/api/workers?x=1" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if string(gotBody) != `{"name":"a"}` {
		t.Fatalf("body = %s", gotBody)
	}

	got = nil
	if err := c.Do(context.Background(), http.MethodGet, "/workers", nil, &out); err != nil {
		t.Fatalf("Do GET: %v", err)
	}
	if got.Get("Content-Type") != "" {
		t.Fatalf("GET carried Content-Type %q", got.Get("Content-Type"))
	}
}

func TestDoDecodesGzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != "gzip" {
			writeJSON(w, 200, `{"v":"plain"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_, _ = zw.Write([]byte(`{"v":"gzipped"}`))
		_ = zw.Close()
	}))
	defer srv.Close()

	c := newTestClient(t, srv, &fakeTokens{access: "tok"}, nil)
	var out struct {
		V string `json:"v"`
	}
	if err := c.Do(context.Background(), http.MethodGet, "/x", nil, &out); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if out.V != "gzipped" {
		t.Fatalf("V = %q", out.V)
	}
}

func TestDoDecodesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 400, `{"error":"bad","code":"env_reserved_key","params":{"key":"PATH"},"active_tasks":[1]}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv, &fakeTokens{access: "tok"}, nil)
	err := c.Do(context.Background(), http.MethodPost, "/env", map[string]string{}, nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if apiErr.Status != 400 || apiErr.Message != "bad" || apiErr.Code != "env_reserved_key" {
		t.Fatalf("error = %+v", apiErr)
	}
	if !reflect.DeepEqual(apiErr.Params, map[string]any{"key": "PATH"}) {
		t.Fatalf("Params = %#v", apiErr.Params)
	}
	if !reflect.DeepEqual(apiErr.Details["active_tasks"], []any{float64(1)}) {
		t.Fatalf("Details = %#v", apiErr.Details)
	}
	if !IsStatus(err, 400) || IsStatus(err, 404) {
		t.Fatalf("IsStatus mismatch for %v", err)
	}
	if apiErr.Error() != "bad" {
		t.Fatalf("Error() = %q", apiErr.Error())
	}
}

func TestDoErrorFallsBackToStatusText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := newTestClient(t, srv, &fakeTokens{access: "tok"}, nil)
	err := c.Do(context.Background(), http.MethodGet, "/local/media/x.png", nil, nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if apiErr.Status != 404 || apiErr.Message != "Not Found" || apiErr.Code != "" {
		t.Fatalf("error = %+v", apiErr)
	}
}

func TestDoRejectsHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><html></html>")
	}))
	defer srv.Close()

	c := newTestClient(t, srv, &fakeTokens{access: "tok"}, nil)
	var out map[string]any
	if err := c.Do(context.Background(), http.MethodGet, "/workers", nil, &out); !errors.Is(err, ErrNotOpenBee) {
		t.Fatalf("with out: err = %v, want ErrNotOpenBee", err)
	}
	if err := c.Do(context.Background(), http.MethodPost, "/workers/1/read", nil, nil); !errors.Is(err, ErrNotOpenBee) {
		t.Fatalf("without out: err = %v, want ErrNotOpenBee", err)
	}
}

func TestDo204(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(t, srv, &fakeTokens{access: "tok"}, nil)
	out := struct{ A int }{A: 7}
	if err := c.Do(context.Background(), http.MethodDelete, "/workers/1", nil, &out); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if out.A != 7 {
		t.Fatalf("out modified: %+v", out)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDoAppliesDefaultTimeout(t *testing.T) {
	var deadline time.Time
	var hasDeadline bool
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		deadline, hasDeadline = r.Context().Deadline()
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{}`))),
			Request:    r,
		}, nil
	})}
	c := New(Options{BaseURL: "http://example.invalid", HTTP: hc, Tokens: &fakeTokens{access: "tok"}})

	var out map[string]any
	start := time.Now()
	if err := c.Do(context.Background(), http.MethodGet, "/x", nil, &out); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !hasDeadline {
		t.Fatal("no default deadline applied")
	}
	if d := deadline.Sub(start); d < 25*time.Second || d > 31*time.Second {
		t.Fatalf("deadline in %v, want about 30s", d)
	}

	own, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	if err := c.Do(own, http.MethodGet, "/x", nil, &out); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if d := time.Until(deadline); d < 50*time.Minute {
		t.Fatalf("caller deadline replaced: %v left", d)
	}
}

type refreshServer struct {
	*httptest.Server
	refreshCalls atomic.Int32
	protected    atomic.Int32
}

func newRefreshServer(t *testing.T, refreshDelay time.Duration, refreshStatus int) *refreshServer {
	t.Helper()
	rs := &refreshServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		rs.refreshCalls.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Errorf("refresh carried Authorization %q", r.Header.Get("Authorization"))
		}
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.RefreshToken != "r1" {
			t.Errorf("refresh_token = %q", body.RefreshToken)
		}
		time.Sleep(refreshDelay)
		if refreshStatus != 200 {
			writeJSON(w, refreshStatus, `{"error":"invalid or expired refresh token"}`)
			return
		}
		writeJSON(w, 200, `{"access_token":"new","expires_in":7200}`)
	})
	mux.HandleFunc("/api/workers", func(w http.ResponseWriter, r *http.Request) {
		rs.protected.Add(1)
		if r.Header.Get("Authorization") != "Bearer new" {
			writeJSON(w, 401, `{"error":"unauthorized"}`)
			return
		}
		writeJSON(w, 200, `{"ok":true}`)
	})
	rs.Server = httptest.NewServer(mux)
	t.Cleanup(rs.Close)
	return rs
}

func TestRefreshOn401RetriesOnce(t *testing.T) {
	rs := newRefreshServer(t, 0, 200)
	tokens := &fakeTokens{access: "old", refresh: "r1"}
	var unauthorized atomic.Int32
	c := newTestClient(t, rs.Server, tokens, func() { unauthorized.Add(1) })

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.Do(context.Background(), http.MethodGet, "/workers", nil, &out); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !out.OK {
		t.Fatal("retry result not decoded")
	}
	if rs.protected.Load() != 2 {
		t.Fatalf("protected hits = %d, want 2", rs.protected.Load())
	}
	if rs.refreshCalls.Load() != 1 {
		t.Fatalf("refresh calls = %d, want 1", rs.refreshCalls.Load())
	}
	if tokens.setCount() != 1 || tokens.Access() != "new" {
		t.Fatalf("SetAccess calls = %d, access = %q", tokens.setCount(), tokens.Access())
	}
	if unauthorized.Load() != 0 {
		t.Fatalf("OnUnauthorized called %d times", unauthorized.Load())
	}
}

func TestRetryStill401IsUnauthorized(t *testing.T) {
	var protected, refreshes atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		refreshes.Add(1)
		writeJSON(w, 200, `{"access_token":"new"}`)
	})
	mux.HandleFunc("/api/workers", func(w http.ResponseWriter, r *http.Request) {
		protected.Add(1)
		writeJSON(w, 401, `{"error":"unauthorized"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var unauthorized atomic.Int32
	c := newTestClient(t, srv, &fakeTokens{access: "old", refresh: "r1"}, func() { unauthorized.Add(1) })
	err := c.Do(context.Background(), http.MethodGet, "/workers", nil, nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if protected.Load() != 2 || refreshes.Load() != 1 {
		t.Fatalf("protected = %d, refreshes = %d", protected.Load(), refreshes.Load())
	}
	if unauthorized.Load() != 1 {
		t.Fatalf("OnUnauthorized called %d times, want 1", unauthorized.Load())
	}
}

func TestConcurrentRefreshSingleFlight(t *testing.T) {
	rs := newRefreshServer(t, 80*time.Millisecond, 200)
	tokens := &fakeTokens{access: "old", refresh: "r1"}
	var unauthorized atomic.Int32
	c := newTestClient(t, rs.Server, tokens, func() { unauthorized.Add(1) })

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			var out map[string]any
			errs <- c.Do(context.Background(), http.MethodGet, "/workers", nil, &out)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
	}
	if rs.refreshCalls.Load() != 1 {
		t.Fatalf("refresh calls = %d, want 1", rs.refreshCalls.Load())
	}
	if tokens.setCount() != 1 {
		t.Fatalf("SetAccess calls = %d, want 1", tokens.setCount())
	}
	if unauthorized.Load() != 0 {
		t.Fatalf("OnUnauthorized called %d times", unauthorized.Load())
	}
}

func TestRefreshFailureCallsOnUnauthorizedOnce(t *testing.T) {
	rs := newRefreshServer(t, 80*time.Millisecond, 401)
	tokens := &fakeTokens{access: "old", refresh: "r1"}
	var unauthorized atomic.Int32
	c := newTestClient(t, rs.Server, tokens, func() { unauthorized.Add(1) })

	runBurst := func() {
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make(chan error, 5)
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs <- c.Do(context.Background(), http.MethodGet, "/workers", nil, nil)
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("err = %v, want ErrUnauthorized", err)
			}
		}
	}

	runBurst()
	if unauthorized.Load() != 1 {
		t.Fatalf("OnUnauthorized called %d times after first burst, want 1", unauthorized.Load())
	}
	if rs.refreshCalls.Load() != 1 {
		t.Fatalf("refresh calls = %d after first burst, want 1", rs.refreshCalls.Load())
	}

	if err := c.Do(context.Background(), http.MethodGet, "/workers", nil, nil); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("late caller err = %v, want ErrUnauthorized", err)
	}
	if unauthorized.Load() != 1 || rs.refreshCalls.Load() != 1 {
		t.Fatalf("late caller retriggered: callbacks = %d, refreshes = %d", unauthorized.Load(), rs.refreshCalls.Load())
	}

	tokens.replace("old2", "r1")
	runBurst()
	if unauthorized.Load() != 2 {
		t.Fatalf("OnUnauthorized called %d times after second burst, want 2", unauthorized.Load())
	}
}

func TestRefreshWithoutRefreshTokenIsUnauthorized(t *testing.T) {
	rs := newRefreshServer(t, 0, 200)
	var unauthorized atomic.Int32
	c := newTestClient(t, rs.Server, &fakeTokens{access: "old"}, func() { unauthorized.Add(1) })

	err := c.Do(context.Background(), http.MethodGet, "/workers", nil, nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if rs.refreshCalls.Load() != 0 {
		t.Fatalf("refresh endpoint hit %d times", rs.refreshCalls.Load())
	}
	if unauthorized.Load() != 1 {
		t.Fatalf("OnUnauthorized called %d times, want 1", unauthorized.Load())
	}
}

func TestRefreshServerErrorKeepsSession(t *testing.T) {
	rs := newRefreshServer(t, 0, 500)
	var unauthorized atomic.Int32
	c := newTestClient(t, rs.Server, &fakeTokens{access: "old", refresh: "r1"}, func() { unauthorized.Add(1) })

	err := c.Do(context.Background(), http.MethodGet, "/workers", nil, nil)
	if errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, must not be ErrUnauthorized", err)
	}
	if !IsStatus(err, 500) {
		t.Fatalf("err = %v, want status 500", err)
	}
	if unauthorized.Load() != 0 {
		t.Fatalf("OnUnauthorized called %d times", unauthorized.Load())
	}
}

func TestPublicEndpointsSkipAuth(t *testing.T) {
	var refreshes atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		refreshes.Add(1)
		writeJSON(w, 200, `{"access_token":"new"}`)
	})
	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("login carried Authorization %q", r.Header.Get("Authorization"))
		}
		writeJSON(w, 401, `{"error":"invalid username or password"}`)
	})
	mux.HandleFunc("/api/setup/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("setup status carried Authorization %q", r.Header.Get("Authorization"))
		}
		writeJSON(w, 200, `{"initialized":true}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var unauthorized atomic.Int32
	c := newTestClient(t, srv, &fakeTokens{access: "old", refresh: "r1"}, func() { unauthorized.Add(1) })

	err := c.Do(context.Background(), http.MethodPost, "/auth/login", map[string]string{"username": "a"}, nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != 401 || apiErr.Message != "invalid username or password" {
		t.Fatalf("login err = %v", err)
	}
	if errors.Is(err, ErrUnauthorized) {
		t.Fatal("login 401 must not be ErrUnauthorized")
	}
	var status struct {
		Initialized bool `json:"initialized"`
	}
	if err := c.Do(context.Background(), http.MethodGet, "/setup/status", nil, &status); err != nil || !status.Initialized {
		t.Fatalf("setup status: %v, %+v", err, status)
	}
	if refreshes.Load() != 0 || unauthorized.Load() != 0 {
		t.Fatalf("refreshes = %d, unauthorized = %d", refreshes.Load(), unauthorized.Load())
	}
}

func TestDoRawRefreshesAndReplaysBody(t *testing.T) {
	var bodies []string
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"access_token":"new"}`)
	})
	mux.HandleFunc("/api/local/media", func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, r.Header.Get("Authorization")+"|"+string(data))
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer new" {
			writeJSON(w, 401, `{"error":"unauthorized"}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {}\n\n")
	})
	mux.HandleFunc("/api/local/missing", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 404, `{"error":"gone","code":"worker_not_found"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tokens := &fakeTokens{access: "old", refresh: "r1"}
	c := newTestClient(t, srv, tokens, nil)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/local/media", bytes.NewReader([]byte("payload")))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.DoRaw(context.Background(), req)
	if err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(data) != "data: {}\n\n" {
		t.Fatalf("body = %q", data)
	}
	mu.Lock()
	got := append([]string(nil), bodies...)
	mu.Unlock()
	want := []string{"Bearer old|payload", "Bearer new|payload"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}

	missing, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/local/missing", nil)
	resp, err = c.DoRaw(context.Background(), missing)
	if resp != nil {
		resp.Body.Close()
		t.Fatal("expected nil response on 404")
	}
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != 404 || apiErr.Code != "worker_not_found" {
		t.Fatalf("err = %v", err)
	}
}

func TestEnsureFresh(t *testing.T) {
	cases := []struct {
		name        string
		access      string
		wantRefresh int32
	}{
		{"expires in 30s", makeJWT(time.Now().Add(30 * time.Second)), 1},
		{"expires in 10m", makeJWT(time.Now().Add(10 * time.Minute)), 0},
		{"unparseable", "not-a-jwt", 1},
		{"expired", makeJWT(time.Now().Add(-time.Hour)), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := newRefreshServer(t, 0, 200)
			tokens := &fakeTokens{access: tc.access, refresh: "r1"}
			c := newTestClient(t, rs.Server, tokens, nil)
			if err := c.EnsureFresh(context.Background()); err != nil {
				t.Fatalf("EnsureFresh: %v", err)
			}
			if rs.refreshCalls.Load() != tc.wantRefresh {
				t.Fatalf("refresh calls = %d, want %d", rs.refreshCalls.Load(), tc.wantRefresh)
			}
			if tc.wantRefresh == 1 && tokens.Access() != "new" {
				t.Fatalf("access = %q, want new", tokens.Access())
			}
		})
	}
}

func TestRefreshNowFailureReportsUnauthorized(t *testing.T) {
	rs := newRefreshServer(t, 0, 401)
	var unauthorized atomic.Int32
	c := newTestClient(t, rs.Server, &fakeTokens{access: "old", refresh: "r1"}, func() { unauthorized.Add(1) })
	if err := c.RefreshNow(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if unauthorized.Load() != 1 {
		t.Fatalf("OnUnauthorized called %d times, want 1", unauthorized.Load())
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	good := map[string]string{
		"localhost:8080":          "http://localhost:8080",
		"  localhost:8080  ":      "http://localhost:8080",
		"https://x.y/":            "https://x.y",
		"https://x.y/api":         "https://x.y",
		"https://x.y/api/":        "https://x.y",
		"HTTP://x.y:9000":         "http://x.y:9000",
		"https://x.y/openbee/api": "https://x.y/openbee",
		"https://x.y/myapi":       "https://x.y/myapi",
		"[::1]:8080":              "http://[::1]:8080",
		"http://x.y?a=1#frag":     "http://x.y",
	}
	for in, want := range good {
		got, err := NormalizeBaseURL(in)
		if err != nil {
			t.Errorf("NormalizeBaseURL(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
	for _, in := range []string{"ftp://a", "", "   ", "http://", "http://:8080"} {
		if got, err := NormalizeBaseURL(in); err == nil {
			t.Errorf("NormalizeBaseURL(%q) = %q, want error", in, got)
		}
	}
}

func TestProbe(t *testing.T) {
	t.Run("json config", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/config" {
				t.Errorf("path = %s", r.URL.Path)
			}
			if r.Header.Get("Authorization") != "" {
				t.Errorf("probe carried Authorization")
			}
			writeJSON(w, 200, `{"language":"zh","enabled_engines":["claude","pi"]}`)
		}))
		defer srv.Close()
		cfg, err := Probe(context.Background(), srv.Client(), srv.URL+"/")
		if err != nil {
			t.Fatalf("Probe: %v", err)
		}
		if cfg.Language != "zh" || !reflect.DeepEqual(cfg.EnabledEngines, []string{"claude", "pi"}) {
			t.Fatalf("cfg = %+v", cfg)
		}
	})

	t.Run("html", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<html></html>")
		}))
		defer srv.Close()
		if _, err := Probe(context.Background(), srv.Client(), srv.URL); !errors.Is(err, ErrNotOpenBee) {
			t.Fatalf("err = %v, want ErrNotOpenBee", err)
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := srv.URL
		srv.Close()
		_, err := Probe(context.Background(), nil, url)
		if err == nil {
			t.Fatal("expected error")
		}
		if errors.Is(err, ErrNotOpenBee) {
			t.Fatalf("err = %v, must not be ErrNotOpenBee", err)
		}
	})
}

func TestNeedsRefreshParsesExpiry(t *testing.T) {
	now := time.Now()
	if !needsRefresh(makeJWT(now.Add(59*time.Second)), now) {
		t.Fatal("59s left should refresh")
	}
	if needsRefresh(makeJWT(now.Add(61*time.Second)), now) {
		t.Fatal("61s left should not refresh")
	}
	for _, bad := range []string{"", "a.b", "a.!!!.c", "a." + base64.RawURLEncoding.EncodeToString([]byte(`{}`)) + ".c"} {
		if !needsRefresh(bad, now) {
			t.Fatalf("%q should refresh", bad)
		}
	}
	padded := "h." + base64.URLEncoding.EncodeToString([]byte(`{"exp":`+strconv.FormatInt(now.Add(time.Hour).Unix(), 10)+`}`)) + ".s"
	if needsRefresh(padded, now) {
		t.Fatal("padded payload should parse")
	}
}
