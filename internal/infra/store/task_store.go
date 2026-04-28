package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/model"
)

// TaskStore handles persistence for bee tasks.
type TaskStore struct {
	db *sql.DB
}

// NewTaskStore creates a TaskStore backed by db.
func NewTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{db: db}
}

// DB returns the underlying *sql.DB. Used in tests for seeding prerequisite data.
func (s *TaskStore) DB() *sql.DB {
	return s.db
}

// Create inserts a new task and returns its generated ID.
func (s *TaskStore) Create(ctx context.Context, t model.Task) (string, error) {
	id := uuid.New().String()
	now := time.Now().UnixMilli()
	if t.AgentKind == "" {
		t.AgentKind = model.AgentKindWorker
	}
	if t.RootTaskID == "" {
		t.RootTaskID = id // root tasks self-reference
	}
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO bee_tasks
            (id, message_id, worker_id, instruction, type, status,
             scheduled_at, cron_expr, next_run_at, execution_id,
             parent_task_id, root_task_id, agent_kind,
             created_at, updated_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, t.MessageID, t.WorkerID, t.Instruction, t.Type, t.Status,
		t.ScheduledAt, t.CronExpr, t.NextRunAt, "",
		t.ParentTaskID, t.RootTaskID, t.AgentKind,
		now, now,
	)
	if err != nil {
		return "", fmt.Errorf("create task: %w", err)
	}
	return id, nil
}

// GetByID fetches a single task by ID.
func (s *TaskStore) GetByID(ctx context.Context, id string) (model.Task, error) {
	row := s.db.QueryRowContext(ctx, `
        SELECT id, message_id, worker_id, instruction, type, status,
               scheduled_at, cron_expr, next_run_at, execution_id,
               parent_task_id, root_task_id, agent_kind,
               created_at, updated_at
        FROM bee_tasks WHERE id = ?`, id)
	return scanTask(row)
}

// appendCSVFilter appends an IN clause for a comma-separated filter on the given column.
// If value is empty, nothing is appended.
func appendCSVFilter(q string, args []any, column, value string) (string, []any) {
	if value == "" {
		return q, args
	}
	values := splitTrimmed(value)
	q += " AND t." + column + " IN (" + inPlaceholders(len(values)) + ")"
	for _, v := range values {
		args = append(args, v)
	}
	return q, args
}

// splitTrimmed splits a comma-separated string and trims whitespace from each element.
func splitTrimmed(s string) []string {
	parts := make([]string, 0)
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			v := strings.TrimSpace(s[start:i])
			if v != "" {
				parts = append(parts, v)
			}
			start = i + 1
		}
	}
	return parts
}

// TaskFilter specifies filtering criteria for List and CountTasks.
// message_id and session_key are mutually exclusive.
type TaskFilter struct {
	MessageID  string
	SessionKey string
	WorkerID   string
	Status     string // comma-separated, e.g. "pending,running"
	Type       string // comma-separated, e.g. "immediate,countdown"
	Limit      int    // 0 means no limit
	Offset     int
}

// buildFilterWhere appends the JOIN (if needed) and WHERE clauses for a TaskFilter
// to the given query prefix, returning the extended query and bound args.
func buildFilterWhere(q string, f TaskFilter) (string, []any) {
	if f.SessionKey != "" {
		q += ` JOIN bee_platform_messages pm ON t.message_id = pm.id`
	}
	q += ` WHERE 1=1`
	var args []any
	if f.MessageID != "" {
		q += ` AND t.message_id = ?`
		args = append(args, f.MessageID)
	}
	if f.SessionKey != "" {
		q += ` AND pm.session_key = ?`
		args = append(args, f.SessionKey)
	}
	if f.WorkerID != "" {
		q += ` AND t.worker_id = ?`
		args = append(args, f.WorkerID)
	}
	q, args = appendCSVFilter(q, args, "status", f.Status)
	q, args = appendCSVFilter(q, args, "type", f.Type)
	return q, args
}

// List returns tasks matching the given filter. If session_key is set, tasks are
// joined with bee_platform_messages to resolve the session. Results are ordered
// by created_at DESC.
func (s *TaskStore) List(ctx context.Context, f TaskFilter) ([]model.Task, error) {
	q, args := buildFilterWhere(`SELECT t.id, t.message_id, t.worker_id, t.instruction, t.type, t.status,
	             t.scheduled_at, t.cron_expr, t.next_run_at, t.execution_id,
	             t.parent_task_id, t.root_task_id, t.agent_kind,
	             t.created_at, t.updated_at
	      FROM bee_tasks t`, f)
	q += ` ORDER BY t.created_at DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
		if f.Offset > 0 {
			q += ` OFFSET ?`
			args = append(args, f.Offset)
		}
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// CountTasks returns the number of tasks matching the given filter (ignores Limit/Offset).
func (s *TaskStore) CountTasks(ctx context.Context, f TaskFilter) (int, error) {
	q, args := buildFilterWhere(`SELECT COUNT(*) FROM bee_tasks t`, f)
	var count int
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&count)
	return count, err
}

// ListBySessionKey returns tasks whose originating message belongs to the given session.
// status and taskType support comma-separated values (e.g., "pending,running"); empty means all.
func (s *TaskStore) ListBySessionKey(ctx context.Context, sessionKey, status, taskType string) ([]model.Task, error) {
	q := `SELECT t.id, t.message_id, t.worker_id, t.instruction, t.type, t.status,
	             t.scheduled_at, t.cron_expr, t.next_run_at, t.execution_id,
	             t.parent_task_id, t.root_task_id, t.agent_kind,
	             t.created_at, t.updated_at
	      FROM bee_tasks t
	      JOIN bee_platform_messages pm ON t.message_id = pm.id
	      WHERE pm.session_key = ?`
	args := []any{sessionKey}
	q, args = appendCSVFilter(q, args, "status", status)
	q, args = appendCSVFilter(q, args, "type", taskType)
	q += " ORDER BY t.created_at DESC"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks by session key: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// ClaimDueTasks atomically selects all pending tasks that are due at or before nowMS,
// marks immediate/countdown tasks as running, and sets scheduled tasks' next_run_at
// to the pre-computed value from scheduledNextRuns (keyed by task ID).
// scheduledNextRuns may be nil if there are no due scheduled tasks.
func (s *TaskStore) ClaimDueTasks(ctx context.Context, nowMS int64, scheduledNextRuns map[string]int64) ([]model.ClaimedTask, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx, `
        SELECT t.id, t.message_id, t.worker_id, t.instruction, t.type, t.status,
               t.scheduled_at, t.cron_expr, t.next_run_at,
               t.execution_id, t.parent_task_id, t.root_task_id, t.agent_kind,
               t.created_at, t.updated_at,
               pm.session_key, pm.platform
        FROM bee_tasks t
        JOIN bee_platform_messages pm ON pm.id = t.message_id
        WHERE t.status = 'pending'
          AND (
            t.type = 'immediate'
            OR (t.type = 'countdown' AND t.scheduled_at <= ?)
            OR (t.type = 'scheduled' AND (t.next_run_at IS NULL OR t.next_run_at <= ?))
          )`, nowMS, nowMS)
	if err != nil {
		return nil, fmt.Errorf("query due tasks: %w", err)
	}

	var claimed []model.ClaimedTask
	for rows.Next() {
		var ct model.ClaimedTask
		var scheduledAt, nextRunAt sql.NullInt64
		err := rows.Scan(
			&ct.ID, &ct.MessageID, &ct.WorkerID, &ct.Instruction,
			&ct.Type, &ct.Status, &scheduledAt, &ct.CronExpr,
			&nextRunAt, &ct.ExecutionID,
			&ct.ParentTaskID, &ct.RootTaskID, &ct.AgentKind,
			&ct.CreatedAt, &ct.UpdatedAt,
			&ct.MessageSessionKey, &ct.MessagePlatform,
		)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan task: %w", err)
		}
		ct.ScheduledAt = nullInt64Ptr(scheduledAt)
		ct.NextRunAt = nullInt64Ptr(nextRunAt)
		claimed = append(claimed, ct)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	now := time.Now().UnixMilli()
	for i, ct := range claimed {
		if ct.Type == model.TaskTypeScheduled {
			nextRun, ok := scheduledNextRuns[ct.ID]
			if !ok {
				// Fallback: keep next_run_at unchanged (will be re-evaluated next poll)
				continue
			}
			_, err = tx.ExecContext(ctx,
				`UPDATE bee_tasks SET next_run_at = ?, updated_at = ? WHERE id = ?`,
				nextRun, now, ct.ID)
		} else {
			_, err = tx.ExecContext(ctx,
				`UPDATE bee_tasks SET status = 'running', updated_at = ? WHERE id = ?`,
				now, ct.ID)
			claimed[i].Status = model.TaskStatusRunning
		}
		if err != nil {
			return nil, fmt.Errorf("update task %s: %w", ct.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return claimed, nil
}

// PeekDueScheduledTasks returns all pending scheduled tasks whose next_run_at
// is at or before nowMS (or NULL). Read-only — no updates, no locking.
// Used by Scheduler.poll to compute real next_run_at values before claiming.
func (s *TaskStore) PeekDueScheduledTasks(ctx context.Context, nowMS int64) ([]model.Task, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, message_id, worker_id, instruction, type, status,
		       scheduled_at, cron_expr, next_run_at, execution_id,
		       parent_task_id, root_task_id, agent_kind,
		       created_at, updated_at
		FROM bee_tasks
		WHERE type = 'scheduled'
		  AND status = 'pending'
		  AND (next_run_at IS NULL OR next_run_at <= ?)`, nowMS)
	if err != nil {
		return nil, fmt.Errorf("peek due scheduled tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// SetExecution writes execution_id and status back to a task.
func (s *TaskStore) SetExecution(ctx context.Context, taskID, executionID, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE bee_tasks SET execution_id = ?, status = ?, updated_at = ? WHERE id = ?`,
		executionID, status, time.Now().UnixMilli(), taskID)
	return err
}

// CancelTask sets a task status to cancelled.
func (s *TaskStore) CancelTask(ctx context.Context, taskID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE bee_tasks SET status = 'cancelled', updated_at = ? WHERE id = ? AND status IN ('pending', 'running', 'waiting_subtasks')`,
		time.Now().UnixMilli(), taskID)
	return err
}

// UpdateStatus sets only the status of a task. Unlike SetExecution, it does
// not touch execution_id.
func (s *TaskStore) UpdateStatus(ctx context.Context, taskID, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE bee_tasks SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UnixMilli(), taskID)
	return err
}

// CancelByWorkerID cancels all pending/running tasks for a given worker.
func (s *TaskStore) CancelByWorkerID(ctx context.Context, workerID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE bee_tasks SET status = 'cancelled', updated_at = ?
         WHERE worker_id = ? AND status IN ('pending','running')`,
		time.Now().UnixMilli(), workerID)
	return err
}

// CancelBySessionKey cancels pending/running tasks for a session; empty taskType matches all types.
func (s *TaskStore) CancelBySessionKey(ctx context.Context, sessionKey, taskType string) (int64, error) {
	q := `UPDATE bee_tasks SET status = 'cancelled', updated_at = ?
	      WHERE message_id IN (SELECT id FROM bee_platform_messages WHERE session_key = ?)
	        AND status IN ('pending', 'running')`
	args := []any{time.Now().UnixMilli(), sessionKey}
	if taskType != "" {
		q += " AND type = ?"
		args = append(args, taskType)
	}
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("cancel tasks by session key: %w", err)
	}
	return res.RowsAffected()
}

// DeletePendingByMessageIDs removes pending tasks belonging to the given message IDs.
func (s *TaskStore) DeletePendingByMessageIDs(ctx context.Context, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return nil
	}
	args := make([]any, len(messageIDs))
	for i, id := range messageIDs {
		args[i] = id
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM bee_tasks WHERE message_id IN (`+inPlaceholders(len(messageIDs))+`) AND status = 'pending'`,
		args...)
	return err
}

// ResetRunningToPending resets all running tasks back to pending.
func (s *TaskStore) ResetRunningToPending(ctx context.Context) (int64, error) {
	now := time.Now().UnixMilli()
	// Scheduled tasks: clear next_run_at so scheduler recomputes via cron
	_, err := s.db.ExecContext(ctx,
		`UPDATE bee_tasks SET status = 'pending', next_run_at = NULL, updated_at = ?
         WHERE status = 'running' AND type = 'scheduled'`, now)
	if err != nil {
		return 0, err
	}
	// Immediate / countdown tasks: just reset status
	res, err := s.db.ExecContext(ctx,
		`UPDATE bee_tasks SET status = 'pending', updated_at = ?
         WHERE status = 'running' AND type IN ('immediate','countdown')`, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CompleteScheduledTask resets a scheduled task back to pending so it can be
// picked up again by the next cron cycle. If the task has been cancelled, the
// status is preserved and the method returns false.
func (s *TaskStore) CompleteScheduledTask(ctx context.Context, taskID string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE bee_tasks SET status = 'pending', updated_at = ? WHERE id = ? AND status != 'cancelled'`,
		time.Now().UnixMilli(), taskID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// FailTask marks a task as failed. For scheduled tasks with a cron expression,
// it resets to pending instead so the task retries on the next scheduled run.
// Called by the dispatcher when a worker process exits abnormally.
func (s *TaskStore) FailTask(ctx context.Context, taskID string) error {
	task, err := s.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task.Type == model.TaskTypeScheduled && task.CronExpr != "" {
		_, err := s.CompleteScheduledTask(ctx, taskID)
		return err
	}
	return s.UpdateStatus(ctx, taskID, model.TaskStatusFailed)
}

// CompleteTask marks a task as completed on successful worker exit.
// For scheduled tasks with a cron expression, it resets to pending instead
// so the task is picked up again on the next scheduled run.
func (s *TaskStore) CompleteTask(ctx context.Context, taskID string) error {
	task, err := s.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}
	if task.Type == model.TaskTypeScheduled && task.CronExpr != "" {
		_, err := s.CompleteScheduledTask(ctx, taskID)
		return err
	}
	return s.UpdateStatus(ctx, taskID, model.TaskStatusCompleted)
}

// CountPendingByWorkerID returns the number of pending tasks for a given worker.
func (s *TaskStore) CountPendingByWorkerID(ctx context.Context, workerID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bee_tasks WHERE worker_id = ? AND status = 'pending'`,
		workerID,
	).Scan(&count)
	return count, err
}

// CountAllByStatus returns a map of task status to count across all tasks.
func (s *TaskStore) CountAllByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM bee_tasks GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

// HasActiveImmediateTasks reports whether any immediate tasks with status pending or running exist.
func (s *TaskStore) HasActiveImmediateTasks(ctx context.Context) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM bee_tasks WHERE type = ? AND status IN (?, ?))`,
		model.TaskTypeImmediate, model.TaskStatusPending, model.TaskStatusRunning,
	).Scan(&exists)
	return exists == 1, err
}

// CountScheduledActive returns the number of pending scheduled tasks.
func (s *TaskStore) CountScheduledActive(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bee_tasks WHERE type = 'scheduled' AND status = 'pending'`,
	).Scan(&count)
	return count, err
}

// GetTaskByExecutionID returns the task with the given execution_id, or nil if not found.
func (s *TaskStore) GetTaskByExecutionID(ctx context.Context, executionID string) (*model.Task, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, message_id, worker_id, instruction, type, status,
		        scheduled_at, cron_expr, next_run_at, execution_id,
		        parent_task_id, root_task_id, agent_kind,
		        created_at, updated_at
		 FROM bee_tasks WHERE execution_id = ?`,
		executionID,
	)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// nullInt64Ptr converts a sql.NullInt64 to a *int64; returns nil if not valid.
func nullInt64Ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// scanTask scans a single task row.
func scanTask(row *sql.Row) (model.Task, error) {
	var t model.Task
	var scheduledAt, nextRunAt sql.NullInt64
	err := row.Scan(
		&t.ID, &t.MessageID, &t.WorkerID, &t.Instruction,
		&t.Type, &t.Status, &scheduledAt, &t.CronExpr,
		&nextRunAt, &t.ExecutionID,
		&t.ParentTaskID, &t.RootTaskID, &t.AgentKind,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return model.Task{}, fmt.Errorf("scan task: %w", err)
	}
	t.ScheduledAt = nullInt64Ptr(scheduledAt)
	t.NextRunAt = nullInt64Ptr(nextRunAt)
	return t, nil
}

func scanTasks(rows *sql.Rows) ([]model.Task, error) {
	var result []model.Task
	for rows.Next() {
		var t model.Task
		var scheduledAt, nextRunAt sql.NullInt64
		err := rows.Scan(
			&t.ID, &t.MessageID, &t.WorkerID, &t.Instruction,
			&t.Type, &t.Status, &scheduledAt, &t.CronExpr,
			&nextRunAt, &t.ExecutionID,
			&t.ParentTaskID, &t.RootTaskID, &t.AgentKind,
			&t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan task row: %w", err)
		}
		t.ScheduledAt = nullInt64Ptr(scheduledAt)
		t.NextRunAt = nullInt64Ptr(nextRunAt)
		result = append(result, t)
	}
	return result, rows.Err()
}

// ListByRoot returns the entire task tree (root + subtasks) for a given root_task_id.
// Order: root first (created_at ASC).
func (s *TaskStore) ListByRoot(ctx context.Context, rootID string) ([]model.Task, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, message_id, worker_id, instruction, type, status,
               scheduled_at, cron_expr, next_run_at, execution_id,
               parent_task_id, root_task_id, agent_kind,
               created_at, updated_at
        FROM bee_tasks
        WHERE root_task_id = ?
        ORDER BY created_at ASC`, rootID)
	if err != nil {
		return nil, fmt.Errorf("list by root: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// MarkWaitingSubtasks transitions a group root task to waiting_subtasks.
// Returns sql.ErrNoRows if the task does not exist.
func (s *TaskStore) MarkWaitingSubtasks(ctx context.Context, taskID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE bee_tasks SET status = ?, updated_at = ? WHERE id = ?`,
		model.TaskStatusWaitingSubtasks, time.Now().UnixMilli(), taskID,
	)
	if err != nil {
		return fmt.Errorf("mark waiting subtasks: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetParent returns the parent task, or sql.ErrNoRows if the task is a root.
func (s *TaskStore) GetParent(ctx context.Context, taskID string) (model.Task, error) {
	t, err := s.GetByID(ctx, taskID)
	if err != nil {
		return model.Task{}, err
	}
	if t.ParentTaskID == "" {
		return model.Task{}, sql.ErrNoRows
	}
	return s.GetByID(ctx, t.ParentTaskID)
}

// SessionKeyForTask returns the originating platform session key for a task.
func (s *TaskStore) SessionKeyForTask(ctx context.Context, taskID string) (string, error) {
	var sessionKey string
	err := s.db.QueryRowContext(ctx, `
        SELECT pm.session_key
        FROM bee_tasks t
        JOIN bee_platform_messages pm ON pm.id = t.message_id
        WHERE t.id = ?`, taskID).Scan(&sessionKey)
	if err != nil {
		return "", fmt.Errorf("session key for task: %w", err)
	}
	return sessionKey, nil
}

// ListWaitingGroupRoots returns tasks where agent_kind='group' and status IN
// ('waiting_subtasks','running'). Used at startup to recover ongoing group tasks.
func (s *TaskStore) ListWaitingGroupRoots(ctx context.Context) ([]model.Task, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, message_id, worker_id, instruction, type, status,
               scheduled_at, cron_expr, next_run_at, execution_id,
               parent_task_id, root_task_id, agent_kind,
               created_at, updated_at
        FROM bee_tasks
        WHERE agent_kind = 'group'
          AND status IN ('running','waiting_subtasks')
          AND parent_task_id = ''`)
	if err != nil {
		return nil, fmt.Errorf("list waiting group roots: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}
