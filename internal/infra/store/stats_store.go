// internal/infra/store/stats_store.go
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// StatsStore provides aggregated statistics queries.
type StatsStore struct {
	db *sql.DB
}

// NewStatsStore constructs a StatsStore.
func NewStatsStore(db *sql.DB) *StatsStore {
	return &StatsStore{db: db}
}

// ExecStats holds today's execution counts.
type ExecStats struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

// StatsOverview holds all numeric dashboard card data.
type StatsOverview struct {
	Departments            int       `json:"departments"`
	Workers                int       `json:"workers"`
	ActiveWorkersToday     int       `json:"active_workers_today"`
	ActiveWorkersYesterday int       `json:"active_workers_yesterday"`
	ActiveWorkersChange    *float64  `json:"active_workers_change"`
	MessagesReceivedToday  int       `json:"messages_received_today"`
	MessagesSentToday      int       `json:"messages_sent_today"`
	SessionsNewToday       int       `json:"sessions_new_today"`
	ExecutionsToday        ExecStats `json:"executions_today"`
	ScheduledTasks         int       `json:"scheduled_tasks"`
}

// TrendPoint is one day's data point in the activity trend.
type TrendPoint struct {
	Date          string `json:"date"`
	ActiveWorkers int    `json:"active_workers"`
}

// dayBounds returns Unix-millisecond boundaries for the day at the given offset from today
// (0 = today, -1 = yesterday, etc.), in local time.
func dayBounds(offset int) (startMS, endMS int64) {
	now := time.Now()
	y, m, d := now.Date()
	loc := now.Location()
	start := time.Date(y, m, d+offset, 0, 0, 0, 0, loc)
	end := start.Add(24 * time.Hour)
	return start.UnixMilli(), end.UnixMilli()
}

// GetOverview returns all numeric card statistics.
func (s *StatsStore) GetOverview(ctx context.Context) (StatsOverview, error) {
	var ov StatsOverview

	todayStart, todayEnd := dayBounds(0)
	yestStart, yestEnd := dayBounds(-1)

	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bee_departments`).Scan(&ov.Departments); err != nil {
		return ov, fmt.Errorf("count departments: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bee_workers`).Scan(&ov.Workers); err != nil {
		return ov, fmt.Errorf("count workers: %w", err)
	}

	activeWorkerQuery := `
		SELECT COUNT(DISTINCT worker_id)
		FROM bee_executions
		WHERE worker_id IS NOT NULL
		  AND started_at >= ? AND started_at < ?`

	if err := s.db.QueryRowContext(ctx, activeWorkerQuery, todayStart, todayEnd).Scan(&ov.ActiveWorkersToday); err != nil {
		return ov, fmt.Errorf("active workers today: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, activeWorkerQuery, yestStart, yestEnd).Scan(&ov.ActiveWorkersYesterday); err != nil {
		return ov, fmt.Errorf("active workers yesterday: %w", err)
	}

	if ov.ActiveWorkersYesterday > 0 {
		change := float64(ov.ActiveWorkersToday-ov.ActiveWorkersYesterday) / float64(ov.ActiveWorkersYesterday)
		ov.ActiveWorkersChange = &change
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bee_platform_messages WHERE received_at >= ? AND received_at < ?`,
		todayStart, todayEnd,
	).Scan(&ov.MessagesReceivedToday); err != nil {
		return ov, fmt.Errorf("messages received today: %w", err)
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bee_outbound_messages WHERE sent_at >= ? AND sent_at < ?`,
		todayStart, todayEnd,
	).Scan(&ov.MessagesSentToday); err != nil {
		return ov, fmt.Errorf("messages sent today: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM (
			SELECT session_key
			FROM bee_platform_messages
			GROUP BY session_key
			HAVING MIN(received_at) >= ? AND MIN(received_at) < ?
		)`, todayStart, todayEnd,
	).Scan(&ov.SessionsNewToday); err != nil {
		return ov, fmt.Errorf("new sessions today: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM bee_executions
		WHERE worker_id IS NOT NULL
		  AND started_at >= ? AND started_at < ?
		GROUP BY status`, todayStart, todayEnd)
	if err != nil {
		return ov, fmt.Errorf("executions today: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var cnt int
		if err := rows.Scan(&status, &cnt); err != nil {
			return ov, err
		}
		ov.ExecutionsToday.Total += cnt
		switch status {
		case "completed":
			ov.ExecutionsToday.Success += cnt
		case "failed":
			ov.ExecutionsToday.Failed += cnt
		}
	}
	if err := rows.Err(); err != nil {
		return ov, fmt.Errorf("executions today rows: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM bee_tasks
		WHERE type IN ('countdown','scheduled')
		  AND status NOT IN ('completed','cancelled','failed')`,
	).Scan(&ov.ScheduledTasks); err != nil {
		return ov, fmt.Errorf("scheduled tasks: %w", err)
	}

	return ov, nil
}

// GetTrend returns active-worker counts for each of the last `days` days (local time),
// filling missing days with zero.
func (s *StatsStore) GetTrend(ctx context.Context, days int) ([]TrendPoint, error) {
	now := time.Now()
	y, m, d := now.Date()
	loc := now.Location()

	startOfToday := time.Date(y, m, d, 0, 0, 0, 0, loc)
	startOfRange := startOfToday.AddDate(0, 0, -(days - 1))
	startMS := startOfRange.UnixMilli()
	endMS := startOfToday.Add(24 * time.Hour).UnixMilli()

	rows, err := s.db.QueryContext(ctx, `
		SELECT DATE(started_at/1000, 'unixepoch', 'localtime') AS day,
		       COUNT(DISTINCT worker_id) AS active
		FROM bee_executions
		WHERE worker_id IS NOT NULL
		  AND started_at >= ? AND started_at < ?
		GROUP BY day
		ORDER BY day ASC`, startMS, endMS)
	if err != nil {
		return nil, fmt.Errorf("trend query: %w", err)
	}
	defer rows.Close()

	dbCounts := make(map[string]int, days)
	for rows.Next() {
		var day string
		var cnt int
		if err := rows.Scan(&day, &cnt); err != nil {
			return nil, err
		}
		dbCounts[day] = cnt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("trend rows: %w", err)
	}

	points := make([]TrendPoint, days)
	for i := 0; i < days; i++ {
		date := startOfRange.AddDate(0, 0, i).Format("2006-01-02")
		points[i] = TrendPoint{Date: date, ActiveWorkers: dbCounts[date]}
	}
	return points, nil
}
