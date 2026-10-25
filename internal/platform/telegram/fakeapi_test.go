package telegram

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/require"
)

type apiCall struct {
	method string
	form   map[string]string
	files  []string
}

type fakeAPI struct {
	mu      sync.Mutex
	calls   []apiCall
	respond func(c apiCall) string
}

func newFakeBot(t *testing.T, respond func(c apiCall) string, handler bot.HandlerFunc) (*bot.Bot, *fakeAPI) {
	t.Helper()
	f := &fakeAPI{respond: respond}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	if handler == nil {
		handler = func(context.Context, *bot.Bot, *models.Update) {}
	}
	b, err := newBot("1:TOKEN", handler, bot.WithServerURL(srv.URL))
	require.NoError(t, err)
	return b, f
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	c := apiCall{method: r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:], form: map[string]string{}}
	for k := range r.Form {
		c.form[k] = r.Form.Get(k)
	}
	if r.MultipartForm != nil {
		for name := range r.MultipartForm.File {
			c.files = append(c.files, name)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if c.method == "getMe" {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"bot","username":"bot"}}`))
		return
	}
	f.mu.Lock()
	f.calls = append(f.calls, c)
	resp := `{"ok":true,"result":{"message_id":1,"chat":{"id":1,"type":"private"},"date":0}}`
	if f.respond != nil {
		if custom := f.respond(c); custom != "" {
			resp = custom
		}
	}
	f.mu.Unlock()
	_, _ = w.Write([]byte(resp))
}

func (f *fakeAPI) recorded() []apiCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]apiCall(nil), f.calls...)
}

type migrationCall struct{ from, to string }

type fakeMigrator struct {
	mu     sync.Mutex
	calls  []migrationCall
	onCall func()
}

func (m *fakeMigrator) MigrateSessionKeyPrefix(_ context.Context, from, to string) (int64, error) {
	m.mu.Lock()
	m.calls = append(m.calls, migrationCall{from, to})
	m.mu.Unlock()
	if m.onCall != nil {
		m.onCall()
	}
	return 1, nil
}

func (m *fakeMigrator) recorded() []migrationCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]migrationCall(nil), m.calls...)
}
