package store_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/store/storetest"
)

func updateWorker(t *testing.T, st *store.Store, fix *storetest.Fixture, id string, req api.UpdateWorkerRequest) (api.Worker, error) {
	t.Helper()
	var got api.Worker
	err := call(t, fix, func(done func(error)) {
		st.UpdateWorker(id, req, func(w api.Worker, err error) {
			got = w
			done(err)
		})
	})
	return got, err
}

func randomName(t *testing.T, st *store.Store, fix *storetest.Fixture) (api.RandomName, error) {
	t.Helper()
	var got api.RandomName
	err := call(t, fix, func(done func(error)) {
		st.RandomName(func(n api.RandomName, err error) {
			got = n
			done(err)
		})
	})
	return got, err
}

func TestUpdateWorkerRefetchesListAndDetail(t *testing.T) {
	st, fake, fix := loaded(t, team)
	detail := st.WorkerDetail("w1")
	detail.Ensure()
	fix.Settle(t)
	listCalls := countCalls(fake, http.MethodGet, "/api/workers")

	description := "Writes the release notes"
	w, err := updateWorker(t, st, fix, "w1", api.UpdateWorkerRequest{Description: &description})
	if err != nil {
		t.Fatalf("UpdateWorker: %v", err)
	}
	if w.ID != "w1" || w.Description != description {
		t.Fatalf("UpdateWorker returned %+v, want w1 with the new description", w)
	}
	var body map[string]any
	lastBody(t, fake, http.MethodPut, "/api/workers/w1", &body)
	if len(body) != 1 || body["description"] != description {
		t.Fatalf("PUT body = %v, want only the description", body)
	}
	if got, _ := st.Worker("w1"); got.Description != description {
		t.Fatalf("workers list description = %q, want %q", got.Description, description)
	}
	if detail.Data.Description != description {
		t.Fatalf("worker detail description = %q, want %q", detail.Data.Description, description)
	}
	if n := countCalls(fake, http.MethodGet, "/api/workers"); n != listCalls+1 {
		t.Fatalf("worker list calls = %d, want %d (one refetch)", n, listCalls+1)
	}
	if n := countCalls(fake, http.MethodGet, "/api/workers/w2"); n != 0 {
		t.Fatalf("detail calls for w2 = %d, want none", n)
	}
}

func TestUpdateWorkerErrorKeepsData(t *testing.T) {
	st, fake, fix := loaded(t, team)
	listCalls := countCalls(fake, http.MethodGet, "/api/workers")
	fake.FailNext(http.MethodPut, "/api/workers/w1", http.StatusInternalServerError, map[string]string{"error": "boom"})

	description := "lost"
	_, err := updateWorker(t, st, fix, "w1", api.UpdateWorkerRequest{Description: &description})
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusInternalServerError {
		t.Fatalf("UpdateWorker error = %v, want the 500", err)
	}
	if got, _ := st.Worker("w1"); got.Description != "Writes docs" {
		t.Fatalf("workers list description = %q after a failed update", got.Description)
	}
	if n := countCalls(fake, http.MethodGet, "/api/workers"); n != listCalls {
		t.Fatalf("worker list calls = %d after a failed update, want %d", n, listCalls)
	}
}

func TestUpdateWorkerCanceledBySignOut(t *testing.T) {
	st, _, fix := loaded(t, team)
	var got error
	calls := 0
	description := "late"
	st.UpdateWorker("w1", api.UpdateWorkerRequest{Description: &description}, func(_ api.Worker, err error) {
		got = err
		calls++
	})
	st.Logout()
	fix.Settle(t)
	if calls != 1 || !errors.Is(got, context.Canceled) {
		t.Fatalf("done calls = %d err = %v, want one context.Canceled", calls, got)
	}
}

func TestRandomNameThenExhausted(t *testing.T) {
	st, fake, fix := loaded(t, func(s *fakeapi.State) {
		team(s)
		s.RandomNames = []string{"Alice", "Nova"}
	})
	got, err := randomName(t, st, fix)
	if err != nil || got.Name != "Nova" || got.Exhausted {
		t.Fatalf("RandomName = %+v, %v, want Nova", got, err)
	}
	fake.Update(func(s *fakeapi.State) { s.RandomNames = []string{"Alice"} })
	got, err = randomName(t, st, fix)
	if err != nil || got.Name != "" || !got.Exhausted {
		t.Fatalf("RandomName = %+v, %v, want exhausted", got, err)
	}
}
