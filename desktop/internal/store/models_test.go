package store_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
)

const claudeModelsPath = "/api/engines/claude/models"

func withModels(st *fakeapi.State) {
	team(st)
	st.Models = map[string][]api.EngineModel{"claude": {{ID: "opus", Name: "Opus"}, {ID: "sonnet"}}}
}

func modelIDs(models []api.EngineModel) []string {
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	return ids
}

func TestEngineModelsAreCachedPerEngineAndWorker(t *testing.T) {
	st, fake, fix := loaded(t, withModels)
	global := st.EngineModels("claude", "")
	if st.EngineModels("claude", "") != global {
		t.Fatal("EngineModels created a new resource for the same engine")
	}
	scoped := st.EngineModels("claude", "w1")
	pi := st.EngineModels("pi", "")
	if scoped == global || pi == global {
		t.Fatal("EngineModels shared a resource across engines or workers")
	}
	for range 3 {
		global.Ensure()
		scoped.Ensure()
		pi.Ensure()
		fix.Settle(t)
	}
	if got := modelIDs(global.Data); !slices.Equal(got, []string{"opus", "sonnet"}) {
		t.Fatalf("claude models = %v, want opus, sonnet", got)
	}
	if !pi.Loaded || len(pi.Data) != 0 {
		t.Fatalf("pi models Loaded=%v Data=%v, want a loaded empty list", pi.Loaded, pi.Data)
	}
	var queries []string
	for _, call := range fake.Calls(http.MethodGet, claudeModelsPath) {
		queries = append(queries, call.Query)
	}
	if slices.Sort(queries); !slices.Equal(queries, []string{"", "worker_id=w1"}) {
		t.Fatalf("claude model queries = %q, want one global and one for w1", queries)
	}
}

func TestEngineModelsReloadAfterAnEnvChange(t *testing.T) {
	st, fake, fix := loaded(t, withModels)
	models := st.EngineModels("claude", "")
	models.Ensure()
	fix.Settle(t)
	if err := createEnv(t, st, fix, api.CreateEnvRequest{Scope: "global", Key: "ANTHROPIC_API_KEY", Value: "sk-ant"}); err != nil {
		t.Fatalf("CreateEnv: %v", err)
	}
	models.Ensure()
	fix.Settle(t)
	if n := len(fake.Calls(http.MethodGet, claudeModelsPath)); n != 2 {
		t.Fatalf("model list calls = %d, want a reload after the env change", n)
	}
}

func TestEngineModelsErrorKeepsTheMessage(t *testing.T) {
	st, fake, fix := loaded(t, withModels)
	fake.FailNext(http.MethodGet, claudeModelsPath, http.StatusBadGateway, map[string]string{"error": "plugin timed out"})
	models := st.EngineModels("claude", "")
	models.Ensure()
	fix.Settle(t)
	if models.Loaded || models.Err == nil || models.Err.Error() != "plugin timed out" {
		t.Fatalf("Loaded=%v Err=%v, want the plugin error", models.Loaded, models.Err)
	}
	models.Invalidate()
	fix.Settle(t)
	if !models.Loaded || models.Err != nil || len(models.Data) != 2 {
		t.Fatalf("Loaded=%v Err=%v Data=%v after a retry, want the list", models.Loaded, models.Err, models.Data)
	}
}

func TestEngineModelsDropRepeatedIDs(t *testing.T) {
	st, _, fix := loaded(t, func(st *fakeapi.State) {
		team(st)
		st.Models = map[string][]api.EngineModel{"claude": {{ID: "opus", Name: "Opus"}, {ID: "sonnet"}, {ID: "opus", Name: "Opus again"}, {ID: "sonnet"}}}
	})
	models := st.EngineModels("claude", "")
	models.Ensure()
	fix.Settle(t)
	if want := []api.EngineModel{{ID: "opus", Name: "Opus"}, {ID: "sonnet"}}; !slices.Equal(models.Data, want) {
		t.Fatalf("models = %v, want %v", models.Data, want)
	}
}
