package e2e

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
)

const (
	URLEnv      = "OPENBEE_E2E_URL"
	UserEnv     = "OPENBEE_E2E_USER"
	PasswordEnv = "OPENBEE_E2E_PASSWORD"
	RestartEnv  = "OPENBEE_E2E_RESTART_CMD"

	windowW, windowH = 1440, 900
	narrowW          = 700
	pollStep         = 250 * time.Millisecond
	apiTimeout       = 30 * time.Second
	besideGap        = 40
	paneX, paneY     = 900, 500
	paneTop          = 200
	paneBottom       = 840
	navStep          = 38
	chatTop          = 80
	chatBottom       = 180
	scrollStep       = 300
	scrollTries      = 8
	namePrefix       = "e2e-desktop-"
	envPrefix        = "E2E_DESKTOP_"
	shotPrefix       = "live-"
	workerNote       = "Desktop live acceptance worker"
)

type config struct {
	url, user, pass, restart string
}

func loadConfig(t *testing.T) config {
	t.Helper()
	cfg := config{url: os.Getenv(URLEnv), user: os.Getenv(UserEnv), pass: os.Getenv(PasswordEnv), restart: os.Getenv(RestartEnv)}
	if cfg.url == "" {
		t.Skip(URLEnv + " is not set")
	}
	if cfg.user == "" || cfg.pass == "" {
		t.Fatalf("%s needs %s and %s", URLEnv, UserEnv, PasswordEnv)
	}
	return cfg
}

type flow struct {
	*testkit.Harness
	t     *testing.T
	cfg   config
	admin *admin
}

func start(t *testing.T, setup func(a *admin)) *flow {
	t.Helper()
	cfg := loadConfig(t)
	return launch(t, cfg, cfg.url, setup)
}

func launch(t *testing.T, cfg config, server string, setup func(a *admin)) *flow {
	t.Helper()
	ad := newAdmin(t, cfg)
	ad.sweep()
	if setup != nil {
		setup(ad)
	}
	h := testkit.Live(t, server, cfg.user, cfg.pass, windowW, windowH)
	return &flow{Harness: h, t: t, cfg: cfg, admin: ad}
}

func uniqueName(kind string) string {
	var b [3]byte
	rand.Read(b[:])
	return namePrefix + kind + "-" + hex.EncodeToString(b[:])
}

func (f *flow) click(name string) {
	f.t.Helper()
	f.Click(name)
	f.Motion()
}

func (f *flow) clickBeside(name string) {
	f.t.Helper()
	r := f.rect(name)
	f.T.ClickAt(r.X+r.W+besideGap, r.Y+r.H/2)
	f.Motion()
}

func (f *flow) enter() {
	f.t.Helper()
	f.T.Key(0, ui.KeyEnter)
	f.Motion()
}

func (f *flow) rect(name string) ui.Rect {
	f.t.Helper()
	r, ok := f.T.Find(name)
	if !ok {
		f.t.Fatalf("no %q on screen; texts %q", name, f.T.Texts())
	}
	return r
}

type band struct {
	x, y, top, bottom float32
}

var (
	settingsPane = band{x: paneX, y: paneY, top: paneTop, bottom: paneBottom}
	narrowChat   = band{x: narrowW / 2, y: windowH / 2, top: chatTop, bottom: windowH - chatBottom}
)

func (f *flow) reveal(name string, b band) {
	f.t.Helper()
	for range scrollTries {
		r := f.rect(name)
		switch {
		case r.Y < b.top:
			f.T.Scroll(b.x, b.y, 0, -scrollStep)
		case r.Y+r.H > b.bottom:
			f.T.Scroll(b.x, b.y, 0, scrollStep)
		default:
			return
		}
		f.Motion()
		if f.rect(name) == r {
			return
		}
	}
	f.t.Fatalf("%q does not scroll into view: %+v", name, f.rect(name))
}

func (f *flow) shows(text string) bool {
	_, ok := f.T.Find(text)
	return ok
}

func (f *flow) wantShown(texts ...string) {
	f.t.Helper()
	for _, text := range texts {
		if !f.shows(text) {
			f.t.Fatalf("screen does not show %q; texts %q", text, f.T.Texts())
		}
	}
}

func (f *flow) wantHidden(texts ...string) {
	f.t.Helper()
	for _, text := range texts {
		if f.shows(text) {
			f.t.Fatalf("screen still shows %q; texts %q", text, f.T.Texts())
		}
	}
}

func (f *flow) countTexts(match func(text string) bool) int {
	n := 0
	for _, text := range f.T.Texts() {
		if match(text) {
			n++
		}
	}
	return n
}

func (f *flow) await(what string, limit time.Duration, cond func() bool) {
	f.t.Helper()
	began := time.Now()
	for {
		f.Settle()
		f.T.Advance(pollStep)
		f.T.Frame()
		if cond() {
			f.t.Logf("%s after %v", what, time.Since(began).Round(time.Millisecond))
			return
		}
		if time.Since(began) > limit {
			f.t.Fatalf("%s: not within %v; texts %q", what, limit, f.T.Texts())
		}
		f.Fix.Wait(pollStep)
	}
}

func (f *flow) shot(name string) {
	f.t.Helper()
	f.Shot(shotPrefix + name)
}

func (f *flow) path() string {
	return f.Root.Nav().Path()
}

func (f *flow) worker(name string) (api.Worker, bool) {
	for _, w := range f.Store.Workers.Data {
		if w.Name == name {
			return w, true
		}
	}
	return api.Worker{}, false
}

func (f *flow) summary(cid string) api.ConversationSummary {
	for _, cs := range f.Store.Conversations.Data {
		if cs.ID == cid {
			return cs
		}
	}
	return api.ConversationSummary{}
}

func (f *flow) message(cid string, match func(m model.Message) bool) (model.Message, bool) {
	for _, m := range f.Store.Chat(cid).List.Msgs {
		if match(m) {
			return m, true
		}
	}
	return model.Message{}, false
}

func containsFold(text, sub string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(sub))
}

type tokens struct {
	mu              sync.Mutex
	access, refresh string
}

func (k *tokens) Access() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.access
}

func (k *tokens) Refresh() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.refresh
}

func (k *tokens) SetAccess(token string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.access = token
}

type admin struct {
	t *testing.T
	c *api.Client
}

func newAdmin(t *testing.T, cfg config) *admin {
	t.Helper()
	keys := &tokens{}
	a := &admin{t: t, c: api.New(api.Options{BaseURL: cfg.url, Tokens: keys})}
	ctx, cancel := a.ctx()
	defer cancel()
	pair, err := a.c.Login(ctx, cfg.user, cfg.pass)
	if err != nil {
		t.Fatalf("admin sign in to %s: %v", cfg.url, err)
	}
	keys.access, keys.refresh = pair.AccessToken, pair.RefreshToken
	return a
}

func (a *admin) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), apiTimeout)
}

func (a *admin) must(what string, err error) {
	a.t.Helper()
	if err != nil {
		a.t.Fatalf("admin: %s: %v", what, err)
	}
}

func (a *admin) workers() []api.Worker {
	a.t.Helper()
	ctx, cancel := a.ctx()
	defer cancel()
	list, err := a.c.Workers(ctx)
	a.must("list workers", err)
	return list
}

func (a *admin) createWorker(name string) api.Worker {
	a.t.Helper()
	a.cleanupWorker(name)
	ctx, cancel := a.ctx()
	defer cancel()
	w, err := a.c.CreateWorker(ctx, api.CreateWorkerRequest{Name: name, Description: workerNote})
	a.must("create worker "+name, err)
	return w
}

func (a *admin) cleanupWorker(name string) {
	a.t.Cleanup(func() { a.removeWorkers(func(w api.Worker) bool { return w.Name == name }) })
}

func (a *admin) removeWorkers(match func(w api.Worker) bool) {
	ctx, cancel := a.ctx()
	defer cancel()
	list, err := a.c.Workers(ctx)
	if err != nil {
		a.t.Errorf("admin: list workers for cleanup: %v", err)
		return
	}
	for _, w := range list {
		if !match(w) {
			continue
		}
		if err := a.c.DeleteWorker(ctx, w.ID, true); err != nil && !api.IsStatus(err, http.StatusNotFound) {
			a.t.Errorf("admin: delete worker %s: %v", w.Name, err)
		}
	}
}

func (a *admin) envs(scope, scopeID string) []api.EnvConfig {
	a.t.Helper()
	ctx, cancel := a.ctx()
	defer cancel()
	list, err := a.c.Envs(ctx, scope, scopeID)
	a.must("list "+scope+" envs", err)
	return list
}

func (a *admin) cleanupEnv(scope, scopeID, key string) {
	a.t.Cleanup(func() { a.removeEnvs(scope, scopeID, func(e api.EnvConfig) bool { return e.Key == key }) })
}

func (a *admin) removeEnvs(scope, scopeID string, match func(e api.EnvConfig) bool) {
	ctx, cancel := a.ctx()
	defer cancel()
	list, err := a.c.Envs(ctx, scope, scopeID)
	if err != nil {
		a.t.Errorf("admin: list %s envs for cleanup: %v", scope, err)
		return
	}
	for _, e := range list {
		if !match(e) {
			continue
		}
		if err := a.c.DeleteEnv(ctx, e.ID); err != nil && !api.IsStatus(err, http.StatusNotFound) {
			a.t.Errorf("admin: delete env %s: %v", e.Key, err)
		}
	}
}

func (a *admin) sweep() {
	a.removeWorkers(func(w api.Worker) bool { return strings.HasPrefix(w.Name, namePrefix) })
	a.removeEnvs("global", "", func(e api.EnvConfig) bool { return strings.HasPrefix(e.Key, envPrefix) })
}

func (a *admin) systemConfigs() map[string]string {
	a.t.Helper()
	ctx, cancel := a.ctx()
	defer cancel()
	configs, err := a.c.SystemConfigs(ctx)
	a.must("read system configs", err)
	return configs
}

func (a *admin) restoreSystemConfigs(saved map[string]string) {
	a.t.Cleanup(func() {
		ctx, cancel := a.ctx()
		defer cancel()
		now, err := a.c.SystemConfigs(ctx)
		if err != nil {
			a.t.Errorf("admin: read system configs for cleanup: %v", err)
			return
		}
		for key, value := range saved {
			if now[key] == value {
				continue
			}
			if err := a.c.SetSystemConfig(ctx, key, value); err != nil {
				a.t.Errorf("admin: restore %s: %v", key, err)
			}
		}
	})
}

func (a *admin) send(cid, text string) {
	a.t.Helper()
	ctx, cancel := a.ctx()
	defer cancel()
	_, err := a.c.SendMessage(ctx, cid, api.SendMessageRequest{ID: model.NewMessageID(), Content: text})
	a.must("send to "+cid, err)
}

func (a *admin) clear(cid string) {
	a.t.Helper()
	a.must("clear "+cid, a.reset(cid))
}

func (a *admin) cleanupConversation(cid string) {
	a.t.Cleanup(func() {
		if err := a.reset(cid); err != nil {
			a.t.Errorf("admin: clear %s for cleanup: %v", cid, err)
		}
	})
}

func (a *admin) reset(cid string) error {
	ctx, cancel := a.ctx()
	defer cancel()
	err := a.c.ClearConversation(ctx, cid, false)
	if api.IsStatus(err, http.StatusConflict) {
		err = a.c.ClearConversation(ctx, cid, true)
	}
	return err
}

func (a *admin) recorded(cid, content string) bool {
	a.t.Helper()
	ctx, cancel := a.ctx()
	defer cancel()
	page, err := a.c.Messages(ctx, cid, 0, 0)
	a.must("list messages in "+cid, err)
	for _, m := range page.Messages {
		if m.Content == content {
			return true
		}
	}
	return false
}

func (a *admin) catalog() api.PluginCatalog {
	a.t.Helper()
	ctx, cancel := a.ctx()
	defer cancel()
	catalog, err := a.c.PluginCatalog(ctx, false)
	a.must("read the plugin catalog", err)
	return catalog
}

func (a *admin) plugins() []api.PluginStatus {
	a.t.Helper()
	ctx, cancel := a.ctx()
	defer cancel()
	list, err := a.c.Plugins(ctx)
	a.must("list plugins", err)
	return list
}

type relay struct {
	t      *testing.T
	target string
	addr   string
	url    string

	mu    sync.Mutex
	ln    net.Listener
	conns map[net.Conn]struct{}
}

func newRelay(t *testing.T, server string) *relay {
	t.Helper()
	u, err := url.Parse(server)
	if err != nil || u.Host == "" {
		t.Fatalf("relay: parse %q: %v", server, err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("relay: listen: %v", err)
	}
	r := &relay{t: t, target: u.Host, addr: ln.Addr().String(), conns: map[net.Conn]struct{}{}}
	r.url = u.Scheme + "://" + r.addr
	r.serve(ln)
	t.Cleanup(r.cut)
	return r
}

func (r *relay) serve(ln net.Listener) {
	r.mu.Lock()
	r.ln = ln
	r.mu.Unlock()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go r.pipe(c)
		}
	}()
}

func (r *relay) pipe(client net.Conn) {
	server, err := net.Dial("tcp", r.target)
	if err != nil {
		client.Close()
		return
	}
	if !r.track(client, server) {
		client.Close()
		server.Close()
		return
	}
	done := make(chan struct{}, 2)
	go func() { io.Copy(server, client); done <- struct{}{} }()
	go func() { io.Copy(client, server); done <- struct{}{} }()
	<-done
	client.Close()
	server.Close()
	<-done
	r.mu.Lock()
	delete(r.conns, client)
	delete(r.conns, server)
	r.mu.Unlock()
}

func (r *relay) track(conns ...net.Conn) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ln == nil {
		return false
	}
	for _, c := range conns {
		r.conns[c] = struct{}{}
	}
	return true
}

func (r *relay) cut() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ln != nil {
		r.ln.Close()
		r.ln = nil
	}
	for c := range r.conns {
		c.Close()
	}
	clear(r.conns)
}

func (r *relay) restore() {
	r.t.Helper()
	ln, err := net.Listen("tcp", r.addr)
	if err != nil {
		r.t.Fatalf("relay: listen again on %s: %v", r.addr, err)
	}
	r.serve(ln)
}
