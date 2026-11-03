package aitest

import (
	"context"
	"slices"
	"sort"
	"sync"

	ai "github.com/theopenbee/openbee/internal/ai"
)

type Engine struct {
	RunFunc   func(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.RunResult, error)
	UsageFunc func(ctx context.Context, sessionIDs []string) (map[string][]ai.TokenUsage, error)

	mu         sync.Mutex
	runs       []ai.RunOptions
	usageCalls [][]string
}

func (e *Engine) Run(ctx context.Context, workDir, prompt string, opts ai.RunOptions, logPath string) (ai.RunResult, error) {
	e.mu.Lock()
	e.runs = append(e.runs, opts)
	e.mu.Unlock()
	if e.RunFunc != nil {
		return e.RunFunc(ctx, workDir, prompt, opts, logPath)
	}
	return ai.RunResult{}, nil
}

func (e *Engine) CollectTokenUsage(ctx context.Context, sessionIDs []string) (map[string][]ai.TokenUsage, error) {
	e.mu.Lock()
	e.usageCalls = append(e.usageCalls, slices.Clone(sessionIDs))
	e.mu.Unlock()
	if e.UsageFunc != nil {
		return e.UsageFunc(ctx, sessionIDs)
	}
	return nil, nil
}

func (e *Engine) Runs() []ai.RunOptions {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.runs)
}

func (e *Engine) UsageCalls() [][]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.usageCalls)
}

type Source struct {
	mu      sync.RWMutex
	engines map[string]entry
}

type entry struct {
	name   map[string]string
	engine ai.EngineAdapter
}

func NewSource(engines map[string]ai.EngineAdapter) *Source {
	s := &Source{engines: make(map[string]entry, len(engines))}
	for id, e := range engines {
		s.Add(id, map[string]string{"en": id}, e)
	}
	return s
}

func (s *Source) Add(id string, name map[string]string, e ai.EngineAdapter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.engines[id] = entry{name: name, engine: e}
}

func (s *Source) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.engines, id)
}

func (s *Source) Engine(id string) (ai.EngineAdapter, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.engines[id]
	return e.engine, ok
}

func (s *Source) Engines() []ai.EngineInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ai.EngineInfo, 0, len(s.engines))
	for id, e := range s.engines {
		out = append(out, ai.EngineInfo{ID: id, Name: e.name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Source) IDs() []string {
	return ai.EngineIDs(s)
}

func IDs(ids ...string) func() []string {
	return func() []string { return ids }
}
