package store

import (
	"context"
	"net/http"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/model"
)

const executionPollEvery = 2 * time.Second

func (s *Store) initExecutions() {
	s.executions = map[string]*Resource[api.ExecutionDetail]{}
	s.resets = append(s.resets, s.resetExecutions)
}

func (s *Store) Execution(id string) *Resource[api.ExecutionDetail] {
	if r, ok := s.executions[id]; ok {
		return r
	}
	r := NewResource(s, func(ctx context.Context, c *api.Client) (api.ExecutionDetail, error) {
		return c.Execution(ctx, id)
	})
	r.settled = s.scheduleExecution
	s.executions[id] = r
	return r
}

func (s *Store) WatchExecution(id string) {
	if s.watchedRun == id {
		return
	}
	s.stopExecutionPoll()
	s.watchedRun = id
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
