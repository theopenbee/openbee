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

func createWorker(t *testing.T, st *store.Store, fix *storetest.Fixture, req api.CreateWorkerRequest) (api.Worker, error) {
	t.Helper()
	var got api.Worker
	err := call(t, fix, func(done func(error)) {
		st.CreateWorker(req, func(w api.Worker, err error) {
			got = w
			done(err)
		})
	})
	return got, err
}

func deleteWorker(t *testing.T, st *store.Store, fix *storetest.Fixture, id string, deleteWorkDir bool) error {
	t.Helper()
	return call(t, fix, func(done func(error)) { st.DeleteWorker(id, deleteWorkDir, done) })
}

func TestCreateWorkerRefetchesListAndConversations(t *testing.T) {
	st, fake, fix := loaded(t, team)
	workerCalls := countCalls(fake, http.MethodGet, "/api/workers")
	conversationCalls := countCalls(fake, http.MethodGet, "/api/local/conversations")

	w, err := createWorker(t, st, fix, api.CreateWorkerRequest{Name: "Nova", Engine: "pi", Description: "Reviews code"})
	if err != nil {
		t.Fatalf("CreateWorker: %v", err)
	}
	if w.ID == "" || w.Name != "Nova" || w.Engine != "pi" {
		t.Fatalf("CreateWorker returned %+v, want the new Nova worker", w)
	}
	var body map[string]any
	lastBody(t, fake, http.MethodPost, "/api/workers", &body)
	if body["name"] != "Nova" || body["engine"] != "pi" || body["description"] != "Reviews code" {
		t.Fatalf("POST body = %v", body)
	}
	if got, ok := st.Worker(w.ID); !ok || got.Name != "Nova" {
		t.Fatalf("workers list lacks the new worker: %+v", st.Workers.Data)
	}
	summaryOf(t, st, api.WorkerConversation(w.ID))
	if n := countCalls(fake, http.MethodGet, "/api/workers"); n != workerCalls+1 {
		t.Fatalf("worker list calls = %d, want %d (one refetch)", n, workerCalls+1)
	}
	if n := countCalls(fake, http.MethodGet, "/api/local/conversations"); n != conversationCalls+1 {
		t.Fatalf("conversation list calls = %d, want %d (one refetch)", n, conversationCalls+1)
	}
}

func TestCreateWorkerErrorKeepsData(t *testing.T) {
	st, fake, fix := loaded(t, team)
	workerCalls := countCalls(fake, http.MethodGet, "/api/workers")
	conversationCalls := countCalls(fake, http.MethodGet, "/api/local/conversations")
	_, err := createWorker(t, st, fix, api.CreateWorkerRequest{Name: "Alice"})
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("CreateWorker error = %v, want the duplicate-name API error", err)
	}
	if len(st.Workers.Data) != 2 {
		t.Fatalf("workers = %d after a failed create, want 2", len(st.Workers.Data))
	}
	if n := countCalls(fake, http.MethodGet, "/api/workers"); n != workerCalls {
		t.Fatalf("worker list calls = %d after a failed create, want %d", n, workerCalls)
	}
	if n := countCalls(fake, http.MethodGet, "/api/local/conversations"); n != conversationCalls {
		t.Fatalf("conversation list calls = %d after a failed create, want %d", n, conversationCalls)
	}
}

func TestCreateWorkerCanceledBySignOut(t *testing.T) {
	st, _, fix := loaded(t, team)
	var got error
	calls := 0
	st.CreateWorker(api.CreateWorkerRequest{Name: "Late"}, func(_ api.Worker, err error) {
		got = err
		calls++
	})
	st.Logout()
	fix.Settle(t)
	if calls != 1 || !errors.Is(got, context.Canceled) {
		t.Fatalf("done calls = %d err = %v, want one context.Canceled", calls, got)
	}
}

func TestDeleteWorkerDropsDetailAndRefetches(t *testing.T) {
	st, fake, fix := loaded(t, team)
	for _, id := range []string{"w1", "w2"} {
		st.WorkerDetail(id).Ensure()
	}
	fix.Settle(t)
	workerCalls := countCalls(fake, http.MethodGet, "/api/workers")
	conversationCalls := countCalls(fake, http.MethodGet, "/api/local/conversations")
	deletedDetailCalls := countCalls(fake, http.MethodGet, "/api/workers/w1")
	keptDetailCalls := countCalls(fake, http.MethodGet, "/api/workers/w2")

	if err := deleteWorker(t, st, fix, "w1", true); err != nil {
		t.Fatalf("DeleteWorker: %v", err)
	}
	calls := fake.Calls(http.MethodDelete, "/api/workers/w1")
	if len(calls) != 1 || calls[0].Query != "delete_work_dir=true" {
		t.Fatalf("DELETE calls = %+v, want one with delete_work_dir=true", calls)
	}
	if _, ok := st.Worker("w1"); ok {
		t.Fatalf("workers list still has w1: %+v", st.Workers.Data)
	}
	if n := countCalls(fake, http.MethodGet, "/api/workers"); n != workerCalls+1 {
		t.Fatalf("worker list calls = %d, want %d (one refetch)", n, workerCalls+1)
	}
	if n := countCalls(fake, http.MethodGet, "/api/local/conversations"); n != conversationCalls+1 {
		t.Fatalf("conversation list calls = %d, want %d (one refetch)", n, conversationCalls+1)
	}
	if n := countCalls(fake, http.MethodGet, "/api/workers/w1"); n != deletedDetailCalls {
		t.Fatalf("deleted worker detail calls = %d, want %d (dropped, not refetched)", n, deletedDetailCalls)
	}
	if n := countCalls(fake, http.MethodGet, "/api/workers/w2"); n != keptDetailCalls {
		t.Fatalf("other worker detail calls = %d, want %d (marked stale, not refetched)", n, keptDetailCalls)
	}
	st.WorkerDetail("w2").Ensure()
	fix.Settle(t)
	if n := countCalls(fake, http.MethodGet, "/api/workers/w2"); n != keptDetailCalls+1 {
		t.Fatalf("other worker detail calls after Ensure = %d, want %d (refetched)", n, keptDetailCalls+1)
	}
	if detail := st.WorkerDetail("w1"); detail.Loaded {
		t.Fatal("the deleted worker detail is still loaded")
	}
}

func TestDeleteWorkerKeepsWorkDirByDefault(t *testing.T) {
	st, fake, fix := loaded(t, team)
	if err := deleteWorker(t, st, fix, "w2", false); err != nil {
		t.Fatalf("DeleteWorker: %v", err)
	}
	calls := fake.Calls(http.MethodDelete, "/api/workers/w2")
	if len(calls) != 1 || calls[0].Query != "" {
		t.Fatalf("DELETE calls = %+v, want one without a query", calls)
	}
}

func TestDeleteWorkerErrorKeepsData(t *testing.T) {
	st, fake, fix := loaded(t, team)
	workerCalls := countCalls(fake, http.MethodGet, "/api/workers")
	fake.FailNext(http.MethodDelete, "/api/workers/w1", http.StatusInternalServerError, map[string]string{"error": "busy"})
	err := deleteWorker(t, st, fix, "w1", false)
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusInternalServerError {
		t.Fatalf("DeleteWorker error = %v, want the 500", err)
	}
	if _, ok := st.Worker("w1"); !ok {
		t.Fatal("w1 left the list after a failed delete")
	}
	if n := countCalls(fake, http.MethodGet, "/api/workers"); n != workerCalls {
		t.Fatalf("worker list calls = %d after a failed delete, want %d", n, workerCalls)
	}
	if ids := workerIDs(st); !slices.Equal(ids, []string{"w1", "w2"}) {
		t.Fatalf("worker ids = %v, want w1 and w2", ids)
	}
}

func TestDeleteWorkerCanceledBySignOut(t *testing.T) {
	st, _, fix := loaded(t, team)
	var got error
	calls := 0
	st.DeleteWorker("w1", false, func(err error) {
		got = err
		calls++
	})
	st.Logout()
	fix.Settle(t)
	if calls != 1 || !errors.Is(got, context.Canceled) {
		t.Fatalf("done calls = %d err = %v, want one context.Canceled", calls, got)
	}
}

func workerIDs(st *store.Store) []string {
	ids := make([]string, len(st.Workers.Data))
	for i, w := range st.Workers.Data {
		ids[i] = w.ID
	}
	return ids
}
