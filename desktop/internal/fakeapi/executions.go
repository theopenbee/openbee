package fakeapi

import (
	"net/http"
	"slices"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func (s *Server) executionRoutes() {
	s.route("GET /api/executions/{id}", authUser, s.getExecution)
}

func (s *Server) getExecution(q *request) reply {
	e, found := s.st.Executions[q.param("id")]
	if !found {
		return codedReply(http.StatusNotFound, "execution_not_found", "execution not found", nil)
	}
	e = cloneExecution(e)
	if e.Steps == nil {
		e.Steps = []api.ExecutionStep{}
	}
	return ok(e)
}

func cloneExecution(e api.ExecutionDetail) api.ExecutionDetail {
	e.Steps = slices.Clone(e.Steps)
	e.Runs = slices.Clone(e.Runs)
	return e
}
