package pluginmgr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/plugin"
)

const (
	setupTimeout      = 10 * time.Minute
	setupPartialFlush = 300 * time.Millisecond
	setupKeepFinished = 10 * time.Minute
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

type SetupEvent struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	OK    bool   `json:"ok,omitempty"`
	Error string `json:"error,omitempty"`
}

type setupSession struct {
	ID       string
	PluginID string

	mu       sync.Mutex
	events   []SetupEvent
	subs     map[chan SetupEvent]struct{}
	finished bool
	stdin    io.WriteCloser
	cancel   context.CancelFunc
	stopped  bool
	done     chan struct{}
}

func (s *setupSession) publish(ev SetupEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return
	}
	s.events = append(s.events, ev)
	for ch := range s.subs {
		select {
		case ch <- ev:
		default:
		}
	}
	if ev.Type == "done" {
		s.finished = true
		for ch := range s.subs {
			close(ch)
		}
		s.subs = nil
		close(s.done)
	}
}

func (s *setupSession) subscribe() ([]SetupEvent, <-chan SetupEvent, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	history := append([]SetupEvent(nil), s.events...)
	ch := make(chan SetupEvent, 256)
	if s.finished {
		close(ch)
		return history, ch, func() {}
	}
	s.subs[ch] = struct{}{}
	return history, ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
	}
}

func (s *setupSession) input(text string) error {
	s.mu.Lock()
	stdin, finished := s.stdin, s.finished
	s.mu.Unlock()
	if finished {
		return errSetupFinished(s.ID)
	}
	text, _, _ = strings.Cut(text, "\n")
	if _, err := io.WriteString(stdin, strings.TrimRight(text, "\r")+"\n"); err != nil {
		return errSetupFinished(s.ID)
	}
	return nil
}

func (s *setupSession) cancelRun() {
	s.mu.Lock()
	s.stopped = true
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *setupSession) wasStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

func cleanSetupLine(line string) (string, bool) {
	line = strings.TrimRight(ansiPattern.ReplaceAllString(line, ""), "\r \t")
	if strings.TrimSpace(line) == "" {
		return "", false
	}
	for _, r := range line {
		switch r {
		case ' ', '▀', '▄', '█', '▌', '▐', '░', '▒', '▓':
			continue
		}
		return line, true
	}
	return "", false
}

func (m *Manager) StartSetup(ctx context.Context, id string) (string, error) {
	e, err := m.entry(id)
	if err != nil {
		return "", err
	}
	man, err := m.manifest(e)
	if err != nil {
		return "", err
	}
	if man.Setup == nil || len(man.Setup.Command) == 0 {
		return "", errNoSetup(id)
	}
	if m.deps.SetupCommand == nil {
		return "", errNoSetup(id)
	}

	m.cancelSetup(id)

	m.mu.Lock()
	root := m.rootCtx
	m.mu.Unlock()
	if root == nil {
		root = context.Background()
	}
	runCtx, cancel := context.WithTimeout(root, setupTimeout)
	cmd, err := m.deps.SetupCommand(runCtx, man, m.deps.Language)
	if err != nil {
		cancel()
		return "", err
	}
	ai.ConfigureCmd(cmd)
	proc := ai.NewCmdProcess(cmd)
	cmd.Cancel = proc.Stop
	cmd.WaitDelay = 2 * time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return "", err
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Start(); err != nil {
		cancel()
		return "", fmt.Errorf("start setup: %w", err)
	}

	s := &setupSession{
		ID:       uuid.NewString(),
		PluginID: id,
		subs:     map[chan SetupEvent]struct{}{},
		stdin:    stdin,
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	m.mu.Lock()
	m.setups[s.ID] = s
	m.setupByPlugin[id] = s
	m.mu.Unlock()

	go func() {
		defer cancel()
		lastLine := m.pumpSetupOutput(s, stderr)
		waitErr := cmd.Wait()
		ev := SetupEvent{Type: "done"}
		switch {
		case s.wasStopped():
			ev.Error = "setup was cancelled"
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			ev.Error = "setup timed out"
		case waitErr != nil:
			ev.Error = strings.TrimSpace(lastLine)
			if ev.Error == "" {
				ev.Error = waitErr.Error()
			}
		default:
			values, err := plugin.ParseSetupOutput(man, stdout.Bytes())
			if err == nil {
				err = m.SaveSetupValues(m.opContext(), id, values)
			}
			if err != nil {
				ev.Error = err.Error()
			} else {
				ev.OK = true
			}
		}
		if !ev.OK {
			log.Warn("plugin setup failed", zap.String("plugin", id), zap.String("error", ev.Error))
		}
		s.publish(ev)
		time.AfterFunc(setupKeepFinished, func() { m.dropSetup(s) })
	}()
	return s.ID, nil
}

func (m *Manager) pumpSetupOutput(s *setupSession, r io.Reader) string {
	chunks := make(chan []byte)
	go func() {
		defer close(chunks)
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				chunks <- chunk
			}
			if err != nil {
				return
			}
		}
	}()
	var pending strings.Builder
	last := ""
	emit := func(line string) {
		if text, ok := cleanSetupLine(line); ok {
			last = text
			s.publish(SetupEvent{Type: "line", Text: text})
		}
	}
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				if pending.Len() > 0 {
					emit(pending.String())
				}
				return last
			}
			pending.Write(chunk)
			text := pending.String()
			for {
				i := strings.IndexByte(text, '\n')
				if i < 0 {
					break
				}
				emit(text[:i])
				text = text[i+1:]
			}
			pending.Reset()
			pending.WriteString(text)
			if pending.Len() > 0 {
				timer.Reset(setupPartialFlush)
			}
		case <-timer.C:
			if pending.Len() > 0 {
				emit(pending.String())
				pending.Reset()
			}
		}
	}
}

func (m *Manager) cancelSetup(id string) {
	m.mu.Lock()
	s := m.setupByPlugin[id]
	m.mu.Unlock()
	if s == nil {
		return
	}
	s.cancelRun()
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
	}
}

func (m *Manager) dropSetup(s *setupSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.setups, s.ID)
	if m.setupByPlugin[s.PluginID] == s {
		delete(m.setupByPlugin, s.PluginID)
	}
}

func (m *Manager) setupByID(sid string) (*setupSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.setups[sid]
	if !ok {
		return nil, errSetupNotFound(sid)
	}
	return s, nil
}

func (m *Manager) SubscribeSetup(sid string) ([]SetupEvent, <-chan SetupEvent, func(), error) {
	s, err := m.setupByID(sid)
	if err != nil {
		return nil, nil, nil, err
	}
	history, ch, unsubscribe := s.subscribe()
	return history, ch, unsubscribe, nil
}

func (m *Manager) SetupInput(sid, text string) error {
	s, err := m.setupByID(sid)
	if err != nil {
		return err
	}
	return s.input(text)
}

func (m *Manager) CancelSetup(sid string) error {
	s, err := m.setupByID(sid)
	if err != nil {
		return err
	}
	s.cancelRun()
	return nil
}

func (m *Manager) SaveSetupValues(ctx context.Context, id string, values map[string]any) error {
	unlock := m.lockID(id)
	defer unlock()
	if _, err := m.entry(id); err != nil {
		return err
	}
	cur := Settings{Config: map[string]any{}}
	row, found, err := m.deps.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if found {
		cur.Enabled, cur.BotName = row.Enabled, row.BotName
		if s, err := m.deps.Codec.Decode(row); err == nil {
			cur.Config = s.Config
		}
	}
	for k, v := range values {
		cur.Config[k] = v
	}
	enc, err := m.deps.Codec.Encode(cur, id)
	if err != nil {
		return err
	}
	if err := m.deps.Store.Upsert(ctx, enc); err != nil {
		return err
	}
	if !cur.Enabled {
		return nil
	}
	m.stopLocked(id)
	return m.startLocked(ctx, id)
}
