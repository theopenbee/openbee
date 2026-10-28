package store

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/sse"
)

const (
	setupBackoffBase = time.Second
	setupBackoffMax  = 10 * time.Second
	setupAttempts    = 5
	setupEventLine   = "line"
	setupEventDone   = "done"
)

var errNoSetupSession = errors.New("plugin setup did not return a session id")

type SetupSession struct {
	ID           string
	Lines        []string
	Done, OK     bool
	Active       bool
	Reconnecting bool
	Lost         bool
	Err          string
	Starting     bool

	sess      *session
	connected bool
	closed    bool
	stop      context.CancelFunc
	onUpdate  func(*SetupSession)
}

func (s *SetupSession) changed() {
	s.Active = s.ID != "" && !s.Done
	s.Reconnecting = s.Active && !s.connected && !s.Lost && len(s.Lines) > 0
	s.onUpdate(s)
}

func (p *PluginsState) StartSetup(pluginID string, onUpdate func(*SetupSession)) *SetupSession {
	p.CancelSetup(p.setup)
	if onUpdate == nil {
		onUpdate = func(*SetupSession) {}
	}
	s := &SetupSession{Starting: true, sess: p.s.sess, onUpdate: onUpdate}
	p.setup = s
	p.s.async(func(ctx context.Context, c *api.Client) func() {
		id, err := c.StartPluginSetup(ctx, pluginID)
		if err == nil && id == "" {
			err = errNoSetupSession
		}
		return func() {
			if p.setup != s {
				if id != "" {
					p.cancelRemote(id)
				}
				return
			}
			s.Starting = false
			if err != nil {
				s.Err = l10n.ErrorText(err)
			} else {
				s.ID = id
				p.streamSetup(s)
			}
			s.changed()
		}
	}, nil)
	return s
}

func (p *PluginsState) SendSetupInput(s *SetupSession, text string, done func(error)) {
	done = orNop(done)
	if s == nil || s.ID == "" {
		done(errNoSetupSession)
		return
	}
	id := s.ID
	p.s.async(func(ctx context.Context, c *api.Client) func() {
		err := c.PluginSetupInput(ctx, id, text)
		return func() { done(err) }
	}, canceledBy(done))
}

func (p *PluginsState) CancelSetup(s *SetupSession) {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	if p.setup == s {
		p.setup = nil
	}
	if s.stop != nil {
		s.stop()
	}
	if s.ID != "" && !s.Done && s.sess == p.s.sess {
		p.cancelRemote(s.ID)
	}
}

func (p *PluginsState) cancelRemote(id string) {
	p.s.async(func(ctx context.Context, c *api.Client) func() {
		_ = c.CancelPluginSetup(ctx, id)
		return func() {}
	}, nil)
}

func (p *PluginsState) streamSetup(s *SetupSession) {
	sess := p.s.sess
	client := sess.client
	ctx, cancel := context.WithCancel(sess.ctx)
	s.stop = cancel
	path := api.PluginSetupEventsPath(s.ID)
	post := func(fn func()) {
		p.s.o.Post(func() {
			if p.s.sess != sess || p.setup != s || s.closed {
				return
			}
			fn()
			s.changed()
		})
	}
	p.s.busy.Add(1)
	go func() {
		connecting := true
		settle := func() {
			if connecting {
				connecting = false
				p.s.busy.Add(-1)
			}
		}
		defer settle()
		err := sse.Run(ctx, sse.Options{
			Open: func(ctx context.Context) (io.ReadCloser, error) {
				return openStream(ctx, client, path)
			},
			OnOpen: func() {
				post(func() {
					s.Lines = nil
					s.connected = true
				})
				settle()
			},
			OnEvent: func(e sse.Event) {
				var ev api.SetupEvent
				if json.Unmarshal([]byte(e.Data), &ev) != nil {
					return
				}
				switch ev.Type {
				case setupEventLine:
					if ev.Text != "" {
						post(func() { s.Lines = append(s.Lines, ev.Text) })
					}
				case setupEventDone:
					cancel()
					post(func() { p.finishSetup(s, ev) })
				}
			},
			OnDrop: func(error, int) {
				post(func() { s.connected = false })
				settle()
			},
			Backoff:     sse.Backoff{Base: setupBackoffBase, Max: setupBackoffMax},
			MaxAttempts: setupAttempts,
			Sleep: func(ctx context.Context, d time.Duration, wake <-chan struct{}) error {
				err := p.s.o.Sleep(ctx, d, wake)
				if err == nil {
					connecting = true
					p.s.busy.Add(1)
				}
				return err
			},
		})
		if errors.Is(err, sse.ErrGaveUp) {
			post(func() {
				s.Lost = true
				s.connected = false
			})
		}
	}()
}

func (p *PluginsState) finishSetup(s *SetupSession, ev api.SetupEvent) {
	s.Done, s.OK, s.Err = true, ev.OK, ev.Error
	s.connected = false
	p.invalidatePlugins()
}
