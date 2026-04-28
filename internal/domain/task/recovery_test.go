package task

import (
	"context"
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/model"
)

type fakeRecoverySessStore struct {
	// map key: sessionKey+"|"+agentID+"|"+engine → sessionID
	data map[string]string
}

func newFakeRecoverySessStore(data map[string]string) *fakeRecoverySessStore {
	return &fakeRecoverySessStore{data: data}
}

func (f *fakeRecoverySessStore) GetSessionContextForEngine(_ context.Context, sessionKey, agentID, engine string) (string, error) {
	key := sessionKey + "|" + agentID + "|" + engine
	if sid, ok := f.data[key]; ok {
		return sid, nil
	}
	return "", nil
}

func TestRecoverGroupTasks_ResumeWaitingRoots(t *testing.T) {
	rootID := "r1"
	fq := newFakeTaskQuerier(map[string]model.Task{
		rootID: {
			ID:         rootID,
			WorkerID:   "g1",
			AgentKind:  model.AgentKindGroup,
			Status:     model.TaskStatusWaitingSubtasks,
			MessageID:  "m1",
			RootTaskID: rootID,
		},
	})
	fs := newFakeRecoverySessStore(map[string]string{
		"session-x|g1|claude": "session-id",
	})
	out := make(chan DispatchTask, 4)
	err := RecoverGroupTasks(context.Background(), fq, fs, out, "claude")
	if err != nil {
		t.Fatalf("RecoverGroupTasks: %v", err)
	}
	select {
	case ev := <-out:
		if ev.TaskID != rootID {
			t.Errorf("expected taskID=%s, got %s", rootID, ev.TaskID)
		}
		if !strings.Contains(ev.Instruction, "<recovery_event>") {
			t.Errorf("expected <recovery_event> in instruction, got %q", ev.Instruction)
		}
	default:
		t.Fatal("no recovery event emitted")
	}
}

func TestRecoverGroupTasks_NoSessionLost(t *testing.T) {
	rootID := "r2"
	fq := newFakeTaskQuerier(map[string]model.Task{
		rootID: {
			ID:         rootID,
			WorkerID:   "g2",
			AgentKind:  model.AgentKindGroup,
			Status:     model.TaskStatusWaitingSubtasks,
			MessageID:  "m2",
			RootTaskID: rootID,
		},
	})
	// No session exists for g2
	fs := newFakeRecoverySessStore(map[string]string{})
	out := make(chan DispatchTask, 4)
	err := RecoverGroupTasks(context.Background(), fq, fs, out, "claude")
	if err != nil {
		t.Fatalf("RecoverGroupTasks: %v", err)
	}
	select {
	case ev := <-out:
		t.Errorf("expected no event when session is lost, got %+v", ev)
	default:
		// Expected: no event emitted
	}
}

func TestRecoverGroupTasks_UsesRootTaskSessionKey(t *testing.T) {
	rootA := "root-a"
	rootB := "root-b"
	fq := newFakeTaskQuerier(map[string]model.Task{
		rootA: {
			ID:         rootA,
			WorkerID:   "g1",
			AgentKind:  model.AgentKindGroup,
			Status:     model.TaskStatusWaitingSubtasks,
			MessageID:  "m1",
			RootTaskID: rootA,
		},
		rootB: {
			ID:         rootB,
			WorkerID:   "g1",
			AgentKind:  model.AgentKindGroup,
			Status:     model.TaskStatusWaitingSubtasks,
			MessageID:  "m2",
			RootTaskID: rootB,
		},
	})
	fq.sessionKeys = map[string]string{
		rootA: "session-a",
		rootB: "session-b",
	}
	fs := newFakeRecoverySessStore(map[string]string{
		"session-a|g1|claude": "session-a-id",
		"session-b|g1|claude": "session-b-id",
	})
	out := make(chan DispatchTask, 4)
	if err := RecoverGroupTasks(context.Background(), fq, fs, out, "claude"); err != nil {
		t.Fatalf("RecoverGroupTasks: %v", err)
	}

	got := map[string]string{}
	for i := 0; i < 2; i++ {
		select {
		case ev := <-out:
			got[ev.TaskID] = ev.SessionKey
		default:
			t.Fatalf("expected two recovery events, got %d", len(got))
		}
	}
	if got[rootA] != "session-a" || got[rootB] != "session-b" {
		t.Fatalf("unexpected recovered session keys: %#v", got)
	}
}
