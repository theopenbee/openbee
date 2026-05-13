package adapters

import (
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/infra/store"
)

type fakeExecStore struct {
	path        string
	gotID       string
	gotStartedAt *int64
}

func (f *fakeExecStore) PrepareLogPath(id string, startedAt *int64) (string, error) {
	f.gotID = id
	f.gotStartedAt = startedAt
	return f.path + "/" + id, nil
}

// Compile-time check: the real store satisfies execLogPathPreparer.
var _ execLogPathPreparer = (*store.ExecutionStore)(nil)

func TestLogPathProviderDelegatesAndConvertsTime(t *testing.T) {
	fe := &fakeExecStore{path: "/var/log"}
	p := NewLogPathProvider(fe)

	at := time.UnixMilli(1747087200000) // 2026-05-12 sometime
	got, err := p.PrepareForWorker("e1", at)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/var/log/e1" {
		t.Fatalf("got %q", got)
	}
	if fe.gotID != "e1" {
		t.Fatalf("id forwarded wrong: %q", fe.gotID)
	}
	if fe.gotStartedAt == nil || *fe.gotStartedAt != 1747087200000 {
		t.Fatalf("startedAt ms forwarded wrong: %v", fe.gotStartedAt)
	}
}

func TestLogPathProviderZeroTimeSendsNil(t *testing.T) {
	fe := &fakeExecStore{path: "/var/log"}
	p := NewLogPathProvider(fe)
	if _, err := p.PrepareForWorker("e1", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if fe.gotStartedAt != nil {
		t.Fatalf("zero time should produce nil *int64, got %v", *fe.gotStartedAt)
	}
}
