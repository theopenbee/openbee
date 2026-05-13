package adapters

import (
	"testing"
	"time"
)

type fakeExecStore struct{ path string }

func (f fakeExecStore) PrepareLogPath(execID string, startedAt time.Time) (string, error) {
	return f.path + "/" + execID, nil
}

func TestLogPathProviderDelegates(t *testing.T) {
	p := NewLogPathProvider(fakeExecStore{path: "/var/log"})
	got, err := p.PrepareForWorker("e1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got != "/var/log/e1" {
		t.Fatalf("got %q", got)
	}
}
