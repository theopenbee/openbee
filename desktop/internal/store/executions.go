package store

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/model"
)

const (
	executionPollEvery = 2 * time.Second
	cachedRuns         = 4
)

func (s *Store) initExecutions() {
	s.executions = map[string]*Resource[api.ExecutionDetail]{}
	s.resets = append(s.resets, s.resetExecutions)
}

func (s *Store) Execution(id string) *Resource[api.ExecutionDetail] {
	s.runOrder = append(slices.DeleteFunc(s.runOrder, func(v string) bool { return v == id }), id)
	if r, ok := s.executions[id]; ok {
		return r
	}
	r := newResource(s, func(ctx context.Context, c *api.Client) (api.ExecutionDetail, error) {
		return c.Execution(ctx, id)
	})
	r.settled = s.scheduleExecution
	s.executions[id] = r
	s.pruneRuns()
	return r
}

func (s *Store) pruneRuns() {
	for {
		others := len(s.runOrder)
		if slices.Contains(s.runOrder, s.watchedRun) {
			others--
		}
		if others <= cachedRuns {
			return
		}
		i := slices.IndexFunc(s.runOrder, func(id string) bool { return id != s.watchedRun })
		id := s.runOrder[i]
		s.runOrder = slices.Delete(s.runOrder, i, i+1)
		s.executions[id].Reset()
		delete(s.executions, id)
	}
}

func (s *Store) WatchExecution(id string) {
	if s.watchedRun == id {
		return
	}
	s.stopExecutionPoll()
	s.watchedRun = id
	s.pruneRuns()
	if id == "" {
		return
	}
	r := s.Execution(id)
	if executionRunning(r) {
		r.Invalidate()
		return
	}
	refetchIfStale(r)
	r.Ensure()
}

func (s *Store) scheduleExecution() {
	r, ok := s.executions[s.watchedRun]
	if !ok || !s.foreground || s.stopRunPoll != nil || r.Loading || !executionRunning(r) || api.IsStatus(r.Err, http.StatusNotFound) {
		return
	}
	s.stopRunPoll = s.after(executionPollEvery, func() {
		s.stopRunPoll = nil
		if r, ok := s.executions[s.watchedRun]; ok {
			r.Invalidate()
		}
	})
}

func (s *Store) stopExecutionPoll() {
	if s.stopRunPoll != nil {
		s.stopRunPoll()
		s.stopRunPoll = nil
	}
}

func (s *Store) resumeExecution() {
	if r, ok := s.executions[s.watchedRun]; ok && !r.Loading && executionRunning(r) && !api.IsStatus(r.Err, http.StatusNotFound) {
		r.Invalidate()
	}
}

func (s *Store) resetExecutions() {
	s.stopExecutionPoll()
	s.watchedRun = ""
	for _, r := range s.executions {
		r.Reset()
	}
	s.executions = map[string]*Resource[api.ExecutionDetail]{}
	s.runOrder = nil
}

func executionRunning(r *Resource[api.ExecutionDetail]) bool {
	return r.Loaded && r.Data.Status == model.ExecutionRunning
}

func (s *Store) refreshHandoffs(taskID string) {
	for cid, ch := range s.chats {
		if !ch.Loaded || !model.ShowsHandoffTask(ch.List.Msgs, taskID) {
			continue
		}
		if cid == s.active {
			ch.Refresh()
		} else {
			ch.stale = true
		}
	}
}
