package fakeapi

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

var permissionScopes = []string{"read:workers", "read:tasks", "read:messages"}

func (s *Server) workerRoutes() {
	s.route("GET /api/workers", authUser, s.listWorkers)
	s.route("POST /api/workers", authUser, s.createWorker)
	s.route("GET /api/workers/random-name", authUser, s.randomName)
	s.route("GET /api/workers/{id}", authUser, s.getWorker)
	s.route("PUT /api/workers/{id}", authUser, s.updateWorker)
	s.route("DELETE /api/workers/{id}", authUser, s.deleteWorker)
}

func workerNotFound() reply {
	return codedReply(http.StatusNotFound, "worker_not_found", "worker not found", nil)
}

func workerInvalid(format string, args ...any) reply {
	return codedReply(http.StatusBadRequest, "worker_validation", fmt.Sprintf(format, args...)+": validation error", nil)
}

func (s *Server) workerIndex(id string) int {
	return slices.IndexFunc(s.st.Workers, func(w api.Worker) bool { return w.ID == id })
}

func outWorker(w api.Worker) api.Worker {
	w.EngineArgs = maps.Clone(w.EngineArgs)
	if w.EngineArgs == nil {
		w.EngineArgs = map[string]string{}
	}
	return w
}

func (s *Server) engineEnabled(engine string) bool {
	return slices.ContainsFunc(s.st.Engines, func(e api.EngineInfo) bool { return e.ID == engine })
}

func (s *Server) validateEngine(engine string) error {
	if engine != "" && !s.engineEnabled(engine) {
		return fmt.Errorf("engine %q is not enabled", engine)
	}
	return nil
}

func (s *Server) validateEngineArgs(args map[string]string) error {
	for _, engine := range slices.Sorted(maps.Keys(args)) {
		if engine == "" {
			return fmt.Errorf("invalid engine_args: empty engine name")
		}
		if err := s.validateEngine(engine); err != nil {
			return fmt.Errorf("engine_args[%q]: %w", engine, err)
		}
	}
	return nil
}

func changedEngineArgs(stored, submitted map[string]string) map[string]string {
	changed := map[string]string{}
	for engine, args := range submitted {
		if args != "" && args != stored[engine] {
			changed[engine] = args
		}
	}
	return changed
}

func validatePermissionScopes(scopes string) error {
	if scopes == "" {
		return nil
	}
	var invalid []string
	for _, scope := range strings.Split(scopes, ",") {
		scope = strings.TrimSpace(scope)
		if scope != "" && !slices.Contains(permissionScopes, scope) {
			invalid = append(invalid, scope)
		}
	}
	if len(invalid) == 0 {
		return nil
	}
	return fmt.Errorf("invalid permission scope(s): %s; allowed values: %s",
		strings.Join(invalid, ", "), strings.Join(permissionScopes, ", "))
}

func (s *Server) rejectWorkerName(name, excludeID string) (reply, bool) {
	if name == "" {
		return workerInvalid("worker name cannot be empty"), true
	}
	for _, p := range s.st.Plugins {
		if p.BotName != "" && strings.EqualFold(p.BotName, name) {
			return workerInvalid("worker name %q conflicts with bot name", name), true
		}
	}
	for _, w := range s.st.Workers {
		if w.ID != excludeID && w.Name == name {
			return workerInvalid("worker name %q is already taken", name), true
		}
	}
	return reply{}, false
}

func (s *Server) listWorkers(*request) reply {
	out := make([]api.Worker, 0, len(s.st.Workers))
	for _, w := range s.st.Workers {
		out = append(out, outWorker(w))
	}
	return ok(out)
}

func (s *Server) getWorker(q *request) reply {
	i := s.workerIndex(q.param("id"))
	if i < 0 {
		return workerNotFound()
	}
	return ok(outWorker(s.st.Workers[i]))
}

func (s *Server) createWorker(q *request) reply {
	var req api.CreateWorkerRequest
	if bad, good := q.decode(&req); !good {
		return bad
	}
	if req.Name == "" {
		return errReply(http.StatusBadRequest, validatorMessage("createWorkerRequest", "Name", "required"))
	}
	if err := validatePermissionScopes(req.PermissionScopes); err != nil {
		return errReply(http.StatusBadRequest, err.Error())
	}
	if err := s.validateEngine(req.Engine); err != nil {
		return errReply(http.StatusBadRequest, err.Error())
	}
	if err := s.validateEngineArgs(req.EngineArgs); err != nil {
		return errReply(http.StatusBadRequest, err.Error())
	}
	name := strings.TrimSpace(req.Name)
	if bad, rejected := s.rejectWorkerName(name, ""); rejected {
		return bad
	}
	id := s.nextID("worker")
	workDir := strings.TrimSpace(req.WorkDir)
	if workDir == "" {
		workDir = "/tmp/openbee/workers/" + id
	}
	now := s.now()
	w := api.Worker{
		ID:               id,
		Name:             name,
		Description:      req.Description,
		Constraints:      req.Constraints,
		WorkDir:          workDir,
		Engine:           req.Engine,
		EngineArgs:       maps.Clone(req.EngineArgs),
		Status:           "idle",
		PermissionScopes: req.PermissionScopes,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	s.st.Workers = append(s.st.Workers, w)
	s.ensureSummary(api.WorkerConversation(id))
	return created(outWorker(w))
}

func (s *Server) updateWorker(q *request) reply {
	id := q.param("id")
	var req api.UpdateWorkerRequest
	if bad, good := q.decode(&req); !good {
		return bad
	}
	if req.WorkDir != nil && strings.TrimSpace(*req.WorkDir) == "" {
		return workerInvalid("work_dir cannot be empty")
	}
	if req.PermissionScopes != nil {
		if err := validatePermissionScopes(*req.PermissionScopes); err != nil {
			return errReply(http.StatusInternalServerError, err.Error())
		}
	}
	if req.Engine != nil {
		if err := s.validateEngine(*req.Engine); err != nil {
			return errReply(http.StatusInternalServerError, err.Error())
		}
	}
	if req.EngineArgs != nil {
		var stored map[string]string
		if i := s.workerIndex(id); i >= 0 {
			stored = s.st.Workers[i].EngineArgs
		}
		if err := s.validateEngineArgs(changedEngineArgs(stored, *req.EngineArgs)); err != nil {
			return errReply(http.StatusInternalServerError, err.Error())
		}
	}
	i := s.workerIndex(id)
	if i < 0 {
		return workerNotFound()
	}
	w := s.st.Workers[i]
	changed := false
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name != w.Name {
			if bad, rejected := s.rejectWorkerName(name, id); rejected {
				return bad
			}
			w.Name = name
			changed = true
		}
	}
	if req.Description != nil {
		w.Description = *req.Description
		changed = true
	}
	if req.Constraints != nil {
		w.Constraints = *req.Constraints
		changed = true
	}
	if req.WorkDir != nil {
		w.WorkDir = strings.TrimSpace(*req.WorkDir)
		changed = true
	}
	if req.PermissionScopes != nil {
		w.PermissionScopes = *req.PermissionScopes
		changed = true
	}
	if req.Engine != nil {
		w.Engine = *req.Engine
		changed = true
	}
	if req.EngineArgs != nil {
		w.EngineArgs = mergeEngineArgs(w.EngineArgs, *req.EngineArgs)
		changed = true
	}
	if changed {
		w.UpdatedAt = s.now()
		s.st.Workers[i] = w
	}
	return ok(outWorker(w))
}

func mergeEngineArgs(current, update map[string]string) map[string]string {
	if len(update) == 0 {
		return map[string]string{}
	}
	out := maps.Clone(current)
	if out == nil {
		out = map[string]string{}
	}
	for engine, args := range update {
		if args == "" {
			delete(out, engine)
			continue
		}
		out[engine] = args
	}
	return out
}

func (s *Server) deleteWorker(q *request) reply {
	id := q.param("id")
	i := s.workerIndex(id)
	if i < 0 {
		return workerNotFound()
	}
	s.st.Workers = slices.Delete(s.st.Workers, i, i+1)
	cid := api.WorkerConversation(id)
	s.st.Conversations = slices.DeleteFunc(s.st.Conversations, func(c api.ConversationSummary) bool { return c.ID == cid })
	delete(s.st.Messages, cid)
	delete(s.st.ActiveTasks, cid)
	delete(s.reads, cid)
	return ok(map[string]string{"status": "deleted"})
}

func (s *Server) randomName(*request) reply {
	for _, name := range s.st.RandomNames {
		if !slices.ContainsFunc(s.st.Workers, func(w api.Worker) bool { return w.Name == name }) {
			return ok(api.RandomName{Name: name})
		}
	}
	return ok(api.RandomName{Exhausted: true})
}
