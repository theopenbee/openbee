package store_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/store/storetest"
)

const envsPath = "/api/envs"

func envOf(id, scope, scopeID, key string) api.EnvConfig {
	env := api.EnvConfig{ID: id, Scope: scope, Key: key, Masked: "****"}
	if scopeID != "" {
		env.ScopeID = &scopeID
	}
	return env
}

func withEnvs(st *fakeapi.State) {
	team(st)
	st.Envs = []api.EnvConfig{
		envOf("e1", "global", "", "TOKEN"),
		envOf("e2", "worker", "w1", "DB_URL"),
		envOf("e3", "worker", "w2", "OTHER"),
		envOf("e4", "bee", "default", "BEE_ONLY"),
	}
}

func envKeys(r *store.Resource[[]api.EnvConfig]) []string {
	keys := make([]string, len(r.Data))
	for i, e := range r.Data {
		keys[i] = e.Key
	}
	return keys
}

func envQueries(fake *fakeapi.Server) []string {
	var out []string
	for _, c := range fake.Calls(http.MethodGet, envsPath) {
		out = append(out, c.Query)
	}
	slices.Sort(out)
	return out
}

func TestEnvsLoadPerScope(t *testing.T) {
	st, fake, fix := loaded(t, withEnvs)
	global := st.Envs("global", "")
	worker := st.Envs("worker", "w1")
	other := st.Envs("worker", "w2")
	if global == worker || worker == other || global == other {
		t.Fatal("Envs returned a shared resource for different scopes")
	}
	if st.Envs("worker", "w1") != worker {
		t.Fatal("Envs created a new resource for the same scope")
	}
	if worker.Loaded || len(fake.Calls(http.MethodGet, envsPath)) != 0 {
		t.Fatal("Envs fetched before Ensure")
	}

	global.Ensure()
	worker.Ensure()
	other.Ensure()
	fix.Settle(t)

	if got := envKeys(global); !slices.Equal(got, []string{"TOKEN"}) {
		t.Fatalf("global keys = %v, want [TOKEN]", got)
	}
	if got := envKeys(worker); !slices.Equal(got, []string{"DB_URL"}) {
		t.Fatalf("worker w1 keys = %v, want [DB_URL]", got)
	}
	if got := envKeys(other); !slices.Equal(got, []string{"OTHER"}) {
		t.Fatalf("worker w2 keys = %v, want [OTHER]", got)
	}
	want := []string{"scope=global", "scope=worker&scope_id=w1", "scope=worker&scope_id=w2"}
	if got := envQueries(fake); !slices.Equal(got, want) {
		t.Fatalf("env queries = %q, want %q", got, want)
	}

	empty := st.Envs("bee", "missing")
	empty.Ensure()
	fix.Settle(t)
	if !empty.Loaded || empty.Data == nil || len(empty.Data) != 0 {
		t.Fatalf("empty scope Loaded=%v Data=%#v, want a loaded empty list", empty.Loaded, empty.Data)
	}
}

func TestEnvsResetOnSignOut(t *testing.T) {
	st, _, fix := loaded(t, withEnvs)
	worker := st.Envs("worker", "w1")
	worker.Ensure()
	fix.Settle(t)
	st.Logout()
	fix.Settle(t)
	if worker.Loaded || len(worker.Data) != 0 {
		t.Fatalf("env list survived sign-out: Loaded=%v Data=%v", worker.Loaded, worker.Data)
	}
}

func createEnv(t *testing.T, st *store.Store, fix *storetest.Fixture, req api.CreateEnvRequest) error {
	t.Helper()
	return call(t, fix, func(done func(error)) { st.CreateEnv(req, done) })
}

func TestCreateEnvInvalidatesScope(t *testing.T) {
	st, fake, fix := loaded(t, withEnvs)
	global, worker, other := st.Envs("global", ""), st.Envs("worker", "w1"), st.Envs("worker", "w2")
	for _, r := range []*store.Resource[[]api.EnvConfig]{global, worker, other} {
		r.Ensure()
	}
	fix.Settle(t)
	before := len(fake.Calls(http.MethodGet, envsPath))

	if err := createEnv(t, st, fix, api.CreateEnvRequest{Scope: "worker", ScopeID: "w1", Key: "API_KEY", Value: "secret-value"}); err != nil {
		t.Fatalf("CreateEnv: %v", err)
	}
	var body map[string]string
	lastBody(t, fake, http.MethodPost, envsPath, &body)
	want := map[string]string{"scope": "worker", "scope_id": "w1", "key": "API_KEY", "value": "secret-value"}
	if len(body) != len(want) || body["scope"] != "worker" || body["scope_id"] != "w1" || body["key"] != "API_KEY" || body["value"] != "secret-value" {
		t.Fatalf("POST body = %v, want %v", body, want)
	}
	if got := envKeys(worker); !slices.Equal(got, []string{"DB_URL", "API_KEY"}) {
		t.Fatalf("worker w1 keys = %v, want the new key appended", got)
	}
	if got := envKeys(global); !slices.Equal(got, []string{"TOKEN"}) {
		t.Fatalf("global keys = %v, want unchanged", got)
	}
	if got := envKeys(other); !slices.Equal(got, []string{"OTHER"}) {
		t.Fatalf("worker w2 keys = %v, want unchanged", got)
	}
	if n := len(fake.Calls(http.MethodGet, envsPath)); n != before+1 {
		t.Fatalf("env list calls = %d, want %d (one refetch of the created scope)", n, before+1)
	}
}

func TestCreateEnvOmitsScopeIDForGlobal(t *testing.T) {
	st, fake, fix := loaded(t, withEnvs)
	if err := createEnv(t, st, fix, api.CreateEnvRequest{Scope: "global", Key: "REGION", Value: "eu"}); err != nil {
		t.Fatalf("CreateEnv: %v", err)
	}
	var body map[string]string
	lastBody(t, fake, http.MethodPost, envsPath, &body)
	if _, ok := body["scope_id"]; ok {
		t.Fatalf("POST body = %v, want no scope_id for the global scope", body)
	}
}

func TestCreateEnvErrorKeepsData(t *testing.T) {
	st, fake, fix := loaded(t, withEnvs)
	worker := st.Envs("worker", "w1")
	worker.Ensure()
	fix.Settle(t)
	before := len(fake.Calls(http.MethodGet, envsPath))

	err := createEnv(t, st, fix, api.CreateEnvRequest{Scope: "worker", ScopeID: "w1", Key: "OPENBEE_API_KEY", Value: "x"})
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "env_reserved_key" {
		t.Fatalf("CreateEnv error = %v, want env_reserved_key", err)
	}
	if n := len(fake.Calls(http.MethodGet, envsPath)); n != before {
		t.Fatalf("env list calls = %d after a failed create, want %d", n, before)
	}
	if got := envKeys(worker); !slices.Equal(got, []string{"DB_URL"}) {
		t.Fatalf("worker keys = %v after a failed create", got)
	}
}

func TestUpdateEnvInvalidatesScope(t *testing.T) {
	st, fake, fix := loaded(t, withEnvs)
	global, worker := st.Envs("global", ""), st.Envs("worker", "w1")
	global.Ensure()
	worker.Ensure()
	fix.Settle(t)
	before := len(fake.Calls(http.MethodGet, envsPath))
	updatedAt := worker.Data[0].UpdatedAt

	err := call(t, fix, func(done func(error)) { st.UpdateEnv("worker", "w1", "e2", "new", done) })
	if err != nil {
		t.Fatalf("UpdateEnv: %v", err)
	}
	var body map[string]string
	lastBody(t, fake, http.MethodPut, "/api/envs/e2", &body)
	if len(body) != 1 || body["value"] != "new" {
		t.Fatalf("PUT body = %v, want only the value", body)
	}
	if n := len(fake.Calls(http.MethodGet, envsPath)); n != before+1 {
		t.Fatalf("env list calls = %d, want %d", n, before+1)
	}
	if worker.Data[0].UpdatedAt == updatedAt || worker.Data[0].Masked != "****" {
		t.Fatalf("worker env = %+v, want the refetched update", worker.Data[0])
	}
}

func TestDeleteEnvInvalidatesScope(t *testing.T) {
	st, fake, fix := loaded(t, withEnvs)
	global, worker := st.Envs("global", ""), st.Envs("worker", "w1")
	global.Ensure()
	worker.Ensure()
	fix.Settle(t)
	before := len(fake.Calls(http.MethodGet, envsPath))

	err := call(t, fix, func(done func(error)) { st.DeleteEnv("worker", "w1", "e2", done) })
	if err != nil {
		t.Fatalf("DeleteEnv: %v", err)
	}
	if len(fake.Calls(http.MethodDelete, "/api/envs/e2")) != 1 {
		t.Fatal("DELETE /api/envs/e2 was not called once")
	}
	if len(worker.Data) != 0 || !worker.Loaded {
		t.Fatalf("worker envs = %v Loaded=%v, want a loaded empty list", worker.Data, worker.Loaded)
	}
	if got := envKeys(global); !slices.Equal(got, []string{"TOKEN"}) {
		t.Fatalf("global keys = %v, want unchanged", got)
	}
	if n := len(fake.Calls(http.MethodGet, envsPath)); n != before+1 {
		t.Fatalf("env list calls = %d, want %d", n, before+1)
	}
}

func TestEnvMutationErrorsKeepData(t *testing.T) {
	st, fake, fix := loaded(t, withEnvs)
	worker := st.Envs("worker", "w1")
	worker.Ensure()
	fix.Settle(t)
	before := len(fake.Calls(http.MethodGet, envsPath))
	fake.FailNext(http.MethodPut, "/api/envs/e2", http.StatusInternalServerError, map[string]string{"error": "boom"})
	fake.FailNext(http.MethodDelete, "/api/envs/e2", http.StatusInternalServerError, map[string]string{"error": "boom"})

	for name, action := range map[string]func(done func(error)){
		"update": func(done func(error)) { st.UpdateEnv("worker", "w1", "e2", "new", done) },
		"delete": func(done func(error)) { st.DeleteEnv("worker", "w1", "e2", done) },
	} {
		var apiErr *api.Error
		if err := call(t, fix, action); !errors.As(err, &apiErr) || apiErr.Status != http.StatusInternalServerError {
			t.Fatalf("%s error = %v, want the 500", name, err)
		}
	}
	if n := len(fake.Calls(http.MethodGet, envsPath)); n != before {
		t.Fatalf("env list calls = %d after failed mutations, want %d", n, before)
	}
	if got := envKeys(worker); !slices.Equal(got, []string{"DB_URL"}) {
		t.Fatalf("worker keys = %v after failed mutations", got)
	}
}

func TestEnvMutationCanceledBySignOut(t *testing.T) {
	st, _, fix := loaded(t, withEnvs)
	var got error
	calls := 0
	st.DeleteEnv("worker", "w1", "e2", func(err error) {
		got = err
		calls++
	})
	st.Logout()
	fix.Settle(t)
	if calls != 1 || !errors.Is(got, context.Canceled) {
		t.Fatalf("done calls = %d err = %v, want one context.Canceled", calls, got)
	}
}
