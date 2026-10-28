package fakeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const (
	reservedEnvKey = "OPENBEE_API_KEY"
	scopeGlobal    = "global"
	maskedLength   = 8
	maskedEdge     = 4
)

var (
	envScopes     = []string{scopeGlobal, "bee", "worker"}
	configKeys    = []string{"default_engine", "engine_args_global", "engine_args_bee"}
	engineArgKeys = []string{"engine_args_global", "engine_args_bee"}
)

func (s *Server) envRoutes() {
	s.route("GET /api/envs", authUser, s.listEnvs)
	s.route("POST /api/envs", authUser, s.createEnv)
	s.route("PUT /api/envs/{id}", authUser, s.updateEnv)
	s.route("DELETE /api/envs/{id}", authUser, s.deleteEnv)
	s.route("GET /api/system-configs", authUser, s.getSystemConfigs)
	s.route("PUT /api/system-configs/{key}", authUser, s.setSystemConfig)
}

func maskValue(value string) string {
	runes := []rune(value)
	if len(runes) < maskedLength {
		return "****"
	}
	return string(runes[:maskedEdge]) + "****" + string(runes[len(runes)-maskedEdge:])
}

func reservedKeyReply(message string) reply {
	return codedReply(http.StatusBadRequest, "env_reserved_key", message, map[string]any{"key": reservedEnvKey})
}

func envNotFound(id string) reply {
	return codedReply(http.StatusNotFound, "env_not_found", fmt.Sprintf("env config not found: %s", id), nil)
}

func (s *Server) envIndex(id string) int {
	return slices.IndexFunc(s.st.Envs, func(e api.EnvConfig) bool { return e.ID == id })
}

func matchesScopeID(have *string, want string) bool {
	if want == "" {
		return have == nil
	}
	return have != nil && *have == want
}

func (s *Server) listEnvs(q *request) reply {
	scope := q.query("scope")
	if scope == "" {
		return errReply(http.StatusBadRequest, "scope query parameter is required")
	}
	if !slices.Contains(envScopes, scope) {
		return codedReply(http.StatusBadRequest, "env_invalid_scope",
			fmt.Sprintf("invalid scope %q: must be one of global, bee, worker", scope), map[string]any{"scope": scope})
	}
	scopeID := q.query("scope_id")
	var out []api.EnvConfig
	for _, e := range s.st.Envs {
		if e.Scope != scope {
			continue
		}
		if !matchesScopeID(e.ScopeID, scopeID) {
			continue
		}
		out = append(out, e)
	}
	return ok(out)
}

func (s *Server) createEnv(q *request) reply {
	var req api.CreateEnvRequest
	if bad, good := q.decode(&req); !good {
		return bad
	}
	switch {
	case req.Scope == "":
		return errReply(http.StatusBadRequest, validatorMessage("createEnvRequest", "Scope", "required"))
	case !slices.Contains(envScopes, req.Scope):
		return errReply(http.StatusBadRequest, validatorMessage("createEnvRequest", "Scope", "oneof"))
	case req.Key == "":
		return errReply(http.StatusBadRequest, validatorMessage("createEnvRequest", "Key", "required"))
	case req.Value == "":
		return errReply(http.StatusBadRequest, validatorMessage("createEnvRequest", "Value", "required"))
	case req.Key == reservedEnvKey:
		return reservedKeyReply(reservedEnvKey + " is reserved and cannot be set")
	case req.Scope != scopeGlobal && req.ScopeID == "":
		return codedReply(http.StatusBadRequest, "env_scope_id_required",
			fmt.Sprintf("scope_id is required for scope %q", req.Scope), map[string]any{"scope": req.Scope})
	}
	var scopeID *string
	if req.Scope != scopeGlobal {
		id := req.ScopeID
		scopeID = &id
		duplicate := slices.ContainsFunc(s.st.Envs, func(e api.EnvConfig) bool {
			return e.Scope == req.Scope && e.ScopeID != nil && *e.ScopeID == id && e.Key == req.Key
		})
		if duplicate {
			return errReply(http.StatusInternalServerError,
				"create env config: constraint failed: UNIQUE constraint failed: env_configs.scope, env_configs.scope_id, env_configs.key")
		}
	}
	now := s.now()
	env := api.EnvConfig{
		ID:        s.nextID("env"),
		Scope:     req.Scope,
		ScopeID:   scopeID,
		Key:       req.Key,
		Masked:    maskValue(req.Value),
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.st.Envs = append(s.st.Envs, env)
	return created(env)
}

func (s *Server) updateEnv(q *request) reply {
	id := q.param("id")
	var req struct {
		Value string `json:"value"`
	}
	if bad, good := q.decode(&req); !good {
		return bad
	}
	if req.Value == "" {
		return errReply(http.StatusBadRequest, validatorMessage("updateEnvRequest", "Value", "required"))
	}
	i := s.envIndex(id)
	if i < 0 {
		return envNotFound(id)
	}
	if s.st.Envs[i].Key == reservedEnvKey {
		return reservedKeyReply(reservedEnvKey + " is reserved")
	}
	s.st.Envs[i].Masked = maskValue(req.Value)
	s.st.Envs[i].UpdatedAt = s.now()
	return ok(map[string]bool{"ok": true})
}

func (s *Server) deleteEnv(q *request) reply {
	id := q.param("id")
	i := s.envIndex(id)
	if i < 0 {
		return envNotFound(id)
	}
	if s.st.Envs[i].Key == reservedEnvKey {
		return reservedKeyReply(reservedEnvKey + " is reserved")
	}
	s.st.Envs = slices.Delete(s.st.Envs, i, i+1)
	return noContent()
}

func (s *Server) getSystemConfigs(*request) reply {
	out := make(map[string]string, len(configKeys))
	for _, key := range configKeys {
		out[key] = s.st.SystemConfigs[key]
	}
	return ok(out)
}

func (s *Server) setSystemConfig(q *request) reply {
	key := q.param("key")
	var req struct {
		Value string `json:"value"`
	}
	if bad, good := q.decode(&req); !good {
		return bad
	}
	switch {
	case key == "default_engine":
		if err := s.validateEngine(req.Value); err != nil {
			return errReply(http.StatusBadRequest, err.Error())
		}
	case slices.Contains(engineArgKeys, key):
		if req.Value == "{}" {
			req.Value = ""
		}
		if req.Value != "" {
			var args map[string]string
			if err := json.Unmarshal([]byte(req.Value), &args); err != nil {
				return errReply(http.StatusBadRequest, "value must be a JSON object mapping engine to CLI args string")
			}
			if err := s.validateEngineArgs(args); err != nil {
				return errReply(http.StatusBadRequest, err.Error())
			}
		}
	default:
		return errReply(http.StatusBadRequest, "unknown config key")
	}
	s.st.SystemConfigs[key] = req.Value
	return ok(map[string]bool{"ok": true})
}
