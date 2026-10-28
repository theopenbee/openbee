package fakeapi

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

type State struct {
	Initialized    bool
	Username       string
	Password       string
	Language       string
	EnabledEngines []string

	Workers       []api.Worker
	Conversations []api.ConversationSummary
	Messages      map[string][]api.ChatMessage
	ActiveTasks   map[string][]api.ActiveTask

	Envs          []api.EnvConfig
	SystemConfigs map[string]string

	Plugins    []api.PluginDetail
	Catalog    api.PluginCatalog
	Operations []api.PluginOperation

	Media       map[string][]byte
	RandomNames []string
}

func seedState() State {
	st := State{
		Initialized:    true,
		Username:       "admin",
		Password:       "secret",
		Language:       "en",
		EnabledEngines: []string{"claude", "codex", "pi"},
		Conversations:  []api.ConversationSummary{newSummary(api.BeeConversation)},
		Catalog:        api.PluginCatalog{SchemaVersion: 1},
	}
	st.normalize()
	return st
}

func (st *State) normalize() {
	if st.Messages == nil {
		st.Messages = map[string][]api.ChatMessage{}
	}
	if st.ActiveTasks == nil {
		st.ActiveTasks = map[string][]api.ActiveTask{}
	}
	if st.SystemConfigs == nil {
		st.SystemConfigs = map[string]string{}
	}
	if st.Media == nil {
		st.Media = map[string][]byte{}
	}
}

func cloneState(st State) State {
	out := st
	out.EnabledEngines = slices.Clone(st.EnabledEngines)
	out.Workers = make([]api.Worker, len(st.Workers))
	for i, w := range st.Workers {
		w.EngineArgs = maps.Clone(w.EngineArgs)
		out.Workers[i] = w
	}
	out.Conversations = slices.Clone(st.Conversations)
	out.Messages = make(map[string][]api.ChatMessage, len(st.Messages))
	for cid, msgs := range st.Messages {
		out.Messages[cid] = slices.Clone(msgs)
	}
	out.ActiveTasks = make(map[string][]api.ActiveTask, len(st.ActiveTasks))
	for cid, tasks := range st.ActiveTasks {
		out.ActiveTasks[cid] = slices.Clone(tasks)
	}
	out.Envs = slices.Clone(st.Envs)
	out.SystemConfigs = maps.Clone(st.SystemConfigs)
	out.Plugins = make([]api.PluginDetail, len(st.Plugins))
	for i, p := range st.Plugins {
		p.Config = maps.Clone(p.Config)
		p.Fields = slices.Clone(p.Fields)
		out.Plugins[i] = p
	}
	out.Catalog.Categories = slices.Clone(st.Catalog.Categories)
	out.Catalog.Tags = slices.Clone(st.Catalog.Tags)
	out.Catalog.Plugins = slices.Clone(st.Catalog.Plugins)
	out.Operations = slices.Clone(st.Operations)
	out.Media = maps.Clone(st.Media)
	out.RandomNames = slices.Clone(st.RandomNames)
	return out
}

func (s *Server) Update(fn func(st *State)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.st)
	s.st.normalize()
}

func (s *Server) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneState(s.st)
}

func (s *Server) now() int64 {
	ts := time.Now().UnixMilli()
	if ts <= s.lastTS {
		ts = s.lastTS + 1
	}
	s.lastTS = ts
	return ts
}

func (s *Server) nextID(prefix string) string {
	s.idSeq++
	return fmt.Sprintf("%s-%d", prefix, s.idSeq)
}
