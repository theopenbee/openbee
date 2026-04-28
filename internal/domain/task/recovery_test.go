package task

import (
	"context"
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/model"
)

type fakeRecoverySessStore struct {
	// map key: agentID+"|"+engine → sessionKey
	data map[string]string
}

func newFakeRecoverySessStore(data map[string]string) *fakeRecoverySessStore {
	return &fakeRecoverySessStore{data: data}
}

func (f *fakeRecoverySessStore) SessionKeyForAgent(_ context.Context, agentID, engine string) (string, string, bool, error) {
	key := agentID + "|" + engine
	if sk, ok := f.data[key]; ok {
		return sk, "session-id", true, nil
	}
	return "", "", false, nil
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
		"g1|claude": "session-key-for-g1",
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
