package task

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/model"
)

// recoveryStore is the subset of TaskStore needed for crash recovery.
type recoveryStore interface {
	ListWaitingGroupRoots(ctx context.Context) ([]model.Task, error)
	ListByRoot(ctx context.Context, rootID string) ([]model.Task, error)
}

// recoverySessionStore is the subset of SessionStore needed for crash recovery.
type recoverySessionStore interface {
	SessionKeyForAgent(ctx context.Context, agentID, engine string) (string, string, bool, error)
}

// RecoverGroupTasks scans the task store for any group tasks that were in
// `waiting_subtasks` or `running` state at the time of the last shutdown,
// and re-enqueues them so the dispatcher can resume them.
func RecoverGroupTasks(ctx context.Context, ts recoveryStore, ss recoverySessionStore, out chan<- DispatchTask, engineName string) error {
	roots, err := ts.ListWaitingGroupRoots(ctx)
	if err != nil {
		return fmt.Errorf("list waiting roots: %w", err)
	}
	for _, root := range roots {
		sessionKey, _, ok, err := ss.SessionKeyForAgent(ctx, root.WorkerID, engineName)
		if err != nil || !ok {
			// Session lost; skip this root (could be failed separately in a future enhancement).
			continue
		}
		snapshot := buildRecoveryEventXML(ctx, ts, root)
		select {
		case out <- DispatchTask{
			TaskID:      root.ID,
			WorkerID:    root.WorkerID,
			SessionKey:  sessionKey,
			Instruction: snapshot,
			TaskType:    model.TaskTypeImmediate,
			MessageID:   root.MessageID,
		}:
		default:
			log.Warn("recovery channel full, dropping event", zap.String("rootTaskID", root.ID))
		}
	}
	return nil
}

func buildRecoveryEventXML(ctx context.Context, ts recoveryStore, root model.Task) string {
	list, _ := ts.ListByRoot(ctx, root.ID)
	var sb strings.Builder
	sb.WriteString("<recovery_event>\n")
	fmt.Fprintf(&sb, "<root_task id=\"%s\" status=\"%s\"/>\n", root.ID, root.Status)
	sb.WriteString("<subtasks>\n")
	for _, t := range list {
		if t.ID == root.ID {
			continue
		}
		fmt.Fprintf(&sb, "  <subtask id=\"%s\" worker=\"%s\" status=\"%s\"/>\n", t.ID, t.WorkerID, t.Status)
	}
	sb.WriteString("</subtasks>\n</recovery_event>\n")
	return sb.String()
}
