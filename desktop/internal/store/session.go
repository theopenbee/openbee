package store

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/prefs"
)

var ErrInvalidURL = errors.New("invalid server address")

type LocalServerError struct {
	Err error
}

func (e *LocalServerError) Error() string {
	return "start the built-in server: " + e.Err.Error()
}

func (e *LocalServerError) Unwrap() error {
	return e.Err
}

type session struct {
	gen    uint64
	ctx    context.Context
	cancel context.CancelFunc
	tokens *tokenSource
	client *api.Client
}

type tokenSource struct {
	s      *Store
	server string

	mu     sync.Mutex
	tokens prefs.Tokens
	closed bool
}

func (t *tokenSource) Access() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tokens.Access
}

func (t *tokenSource) Refresh() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tokens.Refresh
}

func (t *tokenSource) SetAccess(token string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.tokens.Access = token
	t.s.saveTokens(t.server, t.tokens)
}

func (t *tokenSource) set(tokens prefs.Tokens) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.tokens = tokens
	}
}

func (t *tokenSource) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	t.tokens = prefs.Tokens{}
}

func (s *Store) open(server string, tokens prefs.Tokens, gen uint64) *session {
	ctx, cancel := context.WithCancel(context.Background())
	sess := &session{
		gen:    gen,
		ctx:    ctx,
		cancel: cancel,
		tokens: &tokenSource{s: s, server: server, tokens: tokens},
	}
	sess.client = api.New(api.Options{
		BaseURL:  server,
		HTTP:     s.o.HTTP,
		Tokens:   sess.tokens,
		Language: s.language,
		OnUnauthorized: func() {
			s.o.Post(func() { s.expire(sess) })
		},
	})
	return sess
}

func (s *Store) switchSession(server string, tokens prefs.Tokens) {
	old := s.sess
	old.cancel()
	old.tokens.close()
	s.sess = s.open(server, tokens, old.gen+1)
	s.Server = server
	for _, reset := range s.resets {
		reset()
	}
	s.onReset.each(func(fn func()) { fn() })
}

func (s *Store) expire(sess *session) {
	if s.sess != sess || s.Phase != PhaseReady {
		return
	}
	s.Logout()
}

func (s *Store) Client() *api.Client {
	return s.sess.client
}

func (s *Store) LocalServer() string {
	return s.local
}

func (s *Store) Start() {
	s.Prefs = s.o.Prefs.Load()
	s.o.SetThemeSource(s.ThemePref())
	server := cmp.Or(s.Prefs.ServerURL, s.local)
	local := server != "" && server == s.local
	s.switchSession(server, prefs.Tokens{})
	if _, chosen := normalizeLanguage(s.Prefs.Language); chosen || !local {
		s.resolveLanguage()
	}
	if server == "" {
		s.Phase = PhaseConnect
		return
	}
	s.Phase = PhaseBoot
	s.StartingLocal = local
	ts := s.sess.tokens
	s.async(func(ctx context.Context, _ *api.Client) func() {
		startErr := s.startLocal(ctx, server)
		tokens, err := s.o.Tokens.Load(server)
		if err != nil {
			log.Printf("store: load tokens: %v", err)
		}
		return func() {
			s.StartingLocal = false
			if local {
				s.resolveLanguage()
			}
			if startErr != nil {
				s.LocalErr = startErr
				s.Phase = PhaseConnect
				return
			}
			if tokens == (prefs.Tokens{}) {
				s.checkSetup(nil)
				return
			}
			ts.set(tokens)
			s.Phase = PhaseReady
			s.startStream()
		}
	}, nil)
}

func (s *Store) Connect(rawURL string, done func(error)) {
	done = orNop(done)
	base, err := api.NormalizeBaseURL(rawURL)
	if err != nil {
		invalid := fmt.Errorf("%w: %w", ErrInvalidURL, err)
		s.o.Post(func() { done(invalid) })
		return
	}
	hc := s.o.HTTP
	current := s.Server
	s.async(func(ctx context.Context, _ *api.Client) func() {
		err := s.startLocal(ctx, base)
		var cfg api.Config
		if err == nil {
			cfg, err = api.Probe(ctx, hc, base)
		}
		var tokens prefs.Tokens
		if err == nil && base == current {
			var loadErr error
			if tokens, loadErr = s.o.Tokens.Load(base); loadErr != nil {
				log.Printf("store: load tokens: %v", loadErr)
			}
		}
		return func() {
			if err != nil {
				done(err)
				return
			}
			s.LocalErr = nil
			previous := s.Server
			s.Prefs.ServerURL = base
			s.Prefs.LastServer = ""
			s.savePrefs()
			s.switchSession(base, tokens)
			if previous != "" && previous != base {
				s.clearTokens(previous)
			}
			s.Config.Set(cfg)
			s.configSettled()
			if tokens != (prefs.Tokens{}) {
				s.Phase = PhaseReady
				s.startStream()
				done(nil)
				return
			}
			s.checkSetup(done)
		}
	}, canceledBy(done))
}

func (s *Store) startLocal(ctx context.Context, server string) error {
	if s.o.StartLocal == nil || server != s.local {
		return nil
	}
	if err := s.o.StartLocal(ctx); err != nil {
		return &LocalServerError{Err: err}
	}
	return nil
}

func (s *Store) checkSetup(done func(error)) {
	done = orNop(done)
	s.async(func(ctx context.Context, c *api.Client) func() {
		status, err := c.SetupStatus(ctx)
		return func() {
			if err == nil && !status.Initialized {
				s.Phase = PhaseSetup
			} else {
				s.Phase = PhaseLogin
			}
			done(nil)
		}
	}, canceledBy(done))
}

func (s *Store) Login(username, password string, done func(error)) {
	done = orNop(done)
	s.async(func(ctx context.Context, c *api.Client) func() {
		pair, err := c.Login(ctx, username, password)
		return func() {
			if err == nil {
				s.signIn(username, pair)
			}
			done(err)
		}
	}, canceledBy(done))
}

func (s *Store) Setup(req api.SetupRequest, done func(error)) {
	done = orNop(done)
	s.async(func(ctx context.Context, c *api.Client) func() {
		pair, err := c.Setup(ctx, req)
		return func() {
			if err == nil {
				s.signIn(req.Username, pair)
			}
			done(err)
		}
	}, canceledBy(done))
}

func (s *Store) signIn(username string, pair api.TokenPair) {
	server := s.Server
	tokens := prefs.Tokens{Access: pair.AccessToken, Refresh: pair.RefreshToken}
	s.switchSession(server, tokens)
	s.saveTokens(server, tokens)
	s.Prefs.Username = username
	s.savePrefs()
	s.Phase = PhaseReady
	s.startStream()
	s.resolveLanguage()
}

func (s *Store) Logout() {
	s.signOut(s.Server)
}

func (s *Store) ChangeServer() {
	if s.Server != "" {
		s.Prefs.LastServer = s.Server
	}
	s.Prefs.ServerURL = ""
	s.savePrefs()
	s.signOut("")
}

func (s *Store) signOut(next string) {
	previous := s.Server
	s.switchSession(next, prefs.Tokens{})
	if previous != "" {
		s.clearTokens(previous)
	}
	if next == "" {
		s.Phase = PhaseConnect
	} else {
		s.Phase = PhaseLogin
		s.checkSetup(nil)
	}
}

func (s *Store) ChangePassword(oldPassword, newPassword string, done func(error)) {
	done = orNop(done)
	s.async(func(ctx context.Context, c *api.Client) func() {
		err := c.ChangePassword(ctx, oldPassword, newPassword)
		return func() { done(err) }
	}, canceledBy(done))
}

func (s *Store) saveTokens(server string, tokens prefs.Tokens) {
	s.persist(func() error {
		return wrap("save tokens", s.o.Tokens.Save(server, tokens))
	})
}

func (s *Store) clearTokens(server string) {
	s.persist(func() error {
		return wrap("clear tokens", s.o.Tokens.Clear(server))
	})
}

func orNop(done func(error)) func(error) {
	if done == nil {
		return func(error) {}
	}
	return done
}

func canceledBy(done func(error)) func() {
	return func() { done(context.Canceled) }
}

func wrap(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}
