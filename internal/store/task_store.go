package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/model"
)

// TaskStore handles persistence for bee tasks.
type TaskStore struct {
	db *sql.DB
}

// NewTaskStore creates a TaskStore backed by db.
func NewTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{db: db}
}

// Create inserts a new task and returns its generated ID.
func (s *TaskStore) Create(ctx context.Context, t model.Task) (string, error) {
	id := uuid.New().String()
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO tasks
            (id, message_id, worker_id, instruction, type, status,
             scheduled_at, cron_expr, next_run_at, execution_id,
             created_at, updated_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, t.MessageID, t.WorkerID, t.Instruction, t.Type, t.Status,
		t.ScheduledAt, t.CronExpr, t.NextRunAt, "",
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
               created_at, updated_at
        FROM tasks WHERE id = ?`, id)
	return scanTask(row)
}

// appendCSVFilter appends an IN clause for a comma-separated filter on the given column.
// If value is empty, nothing is appended.
func appendCSVFilter(q string, args []any, column, value string) (string, []any) {
	if value == "" {
		return q, args
	}
	values := strings.Split(value, ",")
	placeholders := strings.Repeat("?,", len(values))
	placeholders = placeholders[:len(placeholders)-1]
	q += " AND t." + column + " IN (" + placeholders + ")"
	for _, v := range values {
		args = append(args, strings.TrimSpace(v))
	}
	return q, args
}

// ListByMessageID returns tasks for a given message, optionally filtered by status and/or type.
func (s *TaskStore) ListByMessageID(ctx context.Context, messageID, status, taskType string) ([]model.Task, error) {
	q := `SELECT t.id, t.message_id, t.worker_id, t.instruction, t.type, t.status,
	             t.scheduled_at, t.cron_expr, t.next_run_at, t.execution_id,
	             t.created_at, t.updated_at
	      FROM tasks t WHERE t.message_id = ?`
	args := []any{messageID}
	q, args = appendCSVFilter(q, args, "status", status)
	q, args = appendCSVFilter(q, args, "type", taskType)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// ListBySessionKey returns tasks whose originating message belongs to the given session.
// status and taskType support comma-separated values (e.g., "pending,running"); empty means all.
func (s *TaskStore) ListBySessionKey(ctx context.Context, sessionKey, status, taskType string) ([]model.Task, error) {
	q := `SELECT t.id, t.message_id, t.worker_id, t.instruction, t.type, t.status,
	             t.scheduled_at, t.cron_expr, t.next_run_at, t.execution_id,
	             t.created_at, t.updated_at
	      FROM tasks t
	      JOIN platform_messages pm ON t.message_id = pm.id
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
// marks them running (immediate/countdown) or advances their next_run_at (scheduled),
// and returns them joined with their source platform_message data.
func (s *TaskStore) ClaimDueTasks(ctx context.Context, nowMS int64) ([]model.ClaimedTask, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx, `
        SELECT t.id, t.message_id, t.worker_id, t.instruction, t.type, t.status,
               t.scheduled_at, t.cron_expr, t.next_run_at,
               t.execution_id, t.created_at, t.updated_at,
               pm.session_key, pm.platform
        FROM tasks t
        JOIN platform_messages pm ON pm.id = t.message_id
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
			&ct.CreatedAt, &ct.UpdatedAt,
			&ct.MessageSessionKey, &ct.MessagePlatform,
		)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan task: %w", err)
		}
		if scheduledAt.Valid {
			v := scheduledAt.Int64
			ct.ScheduledAt = &v
		}
		if nextRunAt.Valid {
			v := nextRunAt.Int64
			ct.NextRunAt = &v
		}
		claimed = append(claimed, ct)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	now := time.Now().UnixMilli()
	for i, ct := range claimed {
		if ct.Type == model.TaskTypeScheduled {
			_, err = tx.ExecContext(ctx,
				`UPDATE tasks SET next_run_at = ?, updated_at = ? WHERE id = ?`,
				nowMS+24*60*60*1000, now, ct.ID) // +24h sentinel; overwritten by scheduler
		} else {
			_, err = tx.ExecContext(ctx,
				`UPDATE tasks SET status = 'running', updated_at = ? WHERE id = ?`,
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

// SetExecution writes execution_id and status back to a task.
func (s *TaskStore) SetExecution(ctx context.Context, taskID, executionID, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET execution_id = ?, status = ?, updated_at = ? WHERE id = ?`,
		executionID, status, time.Now().UnixMilli(), taskID)
	return err
}

// CancelTask sets a task status to cancelled.
func (s *TaskStore) CancelTask(ctx context.Context, taskID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = 'cancelled', updated_at = ? WHERE id = ?`,
		time.Now().UnixMilli(), taskID)
	return err
}

// UpdateStatus sets only the status of a task. Unlike SetExecution, it does
// not touch execution_id.
func (s *TaskStore) UpdateStatus(ctx context.Context, taskID, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UnixMilli(), taskID)
	return err
}

// CancelByWorkerID cancels all pending/running tasks for a given worker.
func (s *TaskStore) CancelByWorkerID(ctx context.Context, workerID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = 'cancelled', updated_at = ?
         WHERE worker_id = ? AND status IN ('pending','running')`,
		time.Now().UnixMilli(), workerID)
	return err
}

// CancelBySessionKey cancels all pending/running tasks for a given session.
// Returns the number of tasks cancelled.
func (s *TaskStore) CancelBySessionKey(ctx context.Context, sessionKey string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = 'cancelled', updated_at = ?
		 WHERE message_id IN (SELECT id FROM platform_messages WHERE session_key = ?)
		   AND status IN ('pending', 'running')`,
		time.Now().UnixMilli(), sessionKey)
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
	placeholders := strings.Repeat("?,", len(messageIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(messageIDs))
	for i, id := range messageIDs {
		args[i] = id
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM tasks WHERE message_id IN (`+placeholders+`) AND status = 'pending'`,
		args...)
	return err
}

// ResetRunningToPending resets all running tasks back to pending.
func (s *TaskStore) ResetRunningToPending(ctx context.Context) (int64, error) {
	now := time.Now().UnixMilli()
	// Scheduled tasks: clear next_run_at so scheduler recomputes via cron
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = 'pending', next_run_at = NULL, updated_at = ?
         WHERE status = 'running' AND type = 'scheduled'`, now)
	if err != nil {
		return 0, err
	}
	// Immediate / countdown tasks: just reset status
	res, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = 'pending', updated_at = ?
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
		`UPDATE tasks SET status = 'pending', updated_at = ? WHERE id = ? AND status != 'cancelled'`,
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

// UpdateNextRunAt sets next_run_at for a scheduled task after dispatch.
func (s *TaskStore) UpdateNextRunAt(ctx context.Context, taskID string, nextRunAt int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET next_run_at = ?, updated_at = ? WHERE id = ?`,
		nextRunAt, time.Now().UnixMilli(), taskID)
	return err
}

// CountPendingByWorkerID returns the number of pending tasks for a given worker.
func (s *TaskStore) CountPendingByWorkerID(ctx context.Context, workerID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tasks WHERE worker_id = ? AND status = 'pending'`,
		workerID,
	).Scan(&count)
	return count, err
}

// CountAllByStatus returns a map of task status to count across all tasks.
func (s *TaskStore) CountAllByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM tasks GROUP BY status`)
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

// CountScheduledActive returns the number of pending scheduled tasks.
func (s *TaskStore) CountScheduledActive(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tasks WHERE type = 'scheduled' AND status = 'pending'`,
	).Scan(&count)
	return count, err
}

// GetTaskByExecutionID returns the task with the given execution_id, or nil if not found.
func (s *TaskStore) GetTaskByExecutionID(ctx context.Context, executionID string) (*model.Task, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, message_id, worker_id, instruction, type, status, scheduled_at, cron_expr, next_run_at, execution_id, created_at, updated_at
		 FROM tasks WHERE execution_id = ?`,
		executionID,
	)
	var t model.Task
	var scheduledAt, nextRunAt sql.NullInt64
	err := row.Scan(&t.ID, &t.MessageID, &t.WorkerID, &t.Instruction, &t.Type, &t.Status,
		&scheduledAt, &t.CronExpr, &nextRunAt, &t.ExecutionID, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if scheduledAt.Valid {
		t.ScheduledAt = &scheduledAt.Int64
	}
	if nextRunAt.Valid {
		t.NextRunAt = &nextRunAt.Int64
	}
	return &t, nil
}

// scanTask scans a single task row.
func scanTask(row *sql.Row) (model.Task, error) {
	var t model.Task
	var scheduledAt, nextRunAt sql.NullInt64
	err := row.Scan(
		&t.ID, &t.MessageID, &t.WorkerID, &t.Instruction,
		&t.Type, &t.Status, &scheduledAt, &t.CronExpr,
		&nextRunAt, &t.ExecutionID,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return model.Task{}, fmt.Errorf("scan task: %w", err)
	}
	if scheduledAt.Valid {
		v := scheduledAt.Int64
		t.ScheduledAt = &v
	}
	if nextRunAt.Valid {
		v := nextRunAt.Int64
		t.NextRunAt = &v
	}
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
			&t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan task row: %w", err)
		}
		if scheduledAt.Valid {
			v := scheduledAt.Int64
			t.ScheduledAt = &v
		}
		if nextRunAt.Valid {
			v := nextRunAt.Int64
			t.NextRunAt = &v
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
