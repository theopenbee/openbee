package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/theopenbee/openbee/internal/infra/model"
	"golang.org/x/sync/errgroup"
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
	Departments             int       `json:"departments"`
	Workers                 int       `json:"workers"`
	ActiveWorkersToday      int       `json:"active_workers_today"`
	ActiveWorkersYesterday  int       `json:"active_workers_yesterday"`
	ActiveWorkersChange     *float64  `json:"active_workers_change"`
	MessagesReceivedToday   int       `json:"messages_received_today"`
	MessagesSentToday       int       `json:"messages_sent_today"`
	MessagesTotalToday      int       `json:"messages_total_today"`
	ExecutionsToday         ExecStats `json:"executions_today"`
	ExecDurationTodayMS     int64     `json:"exec_duration_today_ms"`
	ExecDurationYesterdayMS int64     `json:"exec_duration_yesterday_ms"`
	ExecDurationTotalMS     int64     `json:"exec_duration_total_ms"`
	ScheduledTasks          int       `json:"scheduled_tasks"`
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
	var (
		ov      StatsOverview
		mu      sync.Mutex
		eg, egc = errgroup.WithContext(ctx)
	)

	todayStart, todayEnd := dayBounds(0)
	yestStart, yestEnd := dayBounds(-1)

	eg.Go(func() error {
		return s.db.QueryRowContext(egc, `SELECT COUNT(*) FROM bee_departments`).Scan(&ov.Departments)
	})

	eg.Go(func() error {
		return s.db.QueryRowContext(egc, `SELECT COUNT(*) FROM bee_workers`).Scan(&ov.Workers)
	})

	activeWorkerQuery := `
		SELECT COUNT(DISTINCT worker_id)
		FROM bee_executions
		WHERE worker_id IS NOT NULL
		  AND started_at >= ? AND started_at < ?`

	eg.Go(func() error {
		return s.db.QueryRowContext(egc, activeWorkerQuery, todayStart, todayEnd).Scan(&ov.ActiveWorkersToday)
	})

	eg.Go(func() error {
		return s.db.QueryRowContext(egc, activeWorkerQuery, yestStart, yestEnd).Scan(&ov.ActiveWorkersYesterday)
	})

	eg.Go(func() error {
		return s.db.QueryRowContext(egc,
			`SELECT COUNT(*) FROM bee_platform_messages WHERE received_at >= ? AND received_at < ?`,
			todayStart, todayEnd,
		).Scan(&ov.MessagesReceivedToday)
	})

	eg.Go(func() error {
		return s.db.QueryRowContext(egc,
			`SELECT COUNT(*) FROM bee_outbound_messages WHERE sent_at >= ? AND sent_at < ?`,
			todayStart, todayEnd,
		).Scan(&ov.MessagesSentToday)
	})

	durationQuery := `
		SELECT COALESCE(SUM(completed_at - started_at), 0)
		FROM bee_executions
		WHERE status = 'completed'
		  AND completed_at IS NOT NULL
		  AND started_at >= ? AND started_at < ?`

	eg.Go(func() error {
		return s.db.QueryRowContext(egc, durationQuery, todayStart, todayEnd).Scan(&ov.ExecDurationTodayMS)
	})

	eg.Go(func() error {
		return s.db.QueryRowContext(egc, durationQuery, yestStart, yestEnd).Scan(&ov.ExecDurationYesterdayMS)
	})

	eg.Go(func() error {
		return s.db.QueryRowContext(egc, `
			SELECT COALESCE(SUM(completed_at - started_at), 0)
			FROM bee_executions
			WHERE status = 'completed'
			  AND completed_at IS NOT NULL`).Scan(&ov.ExecDurationTotalMS)
	})

	eg.Go(func() error {
		rows, err := s.db.QueryContext(egc, `
			SELECT status, COUNT(*) FROM bee_executions
			WHERE started_at >= ? AND started_at < ?
			GROUP BY status`, todayStart, todayEnd)
		if err != nil {
			return fmt.Errorf("executions today: %w", err)
		}
		defer rows.Close()
		var stats ExecStats
		for rows.Next() {
			var status string
			var cnt int
			if err := rows.Scan(&status, &cnt); err != nil {
				return err
			}
			stats.Total += cnt
			switch status {
			case model.TaskStatusCompleted:
				stats.Success += cnt
			case model.TaskStatusFailed:
				stats.Failed += cnt
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("executions today rows: %w", err)
		}
		mu.Lock()
		ov.ExecutionsToday = stats
		mu.Unlock()
		return nil
	})

	eg.Go(func() error {
		return s.db.QueryRowContext(egc, `
			SELECT COUNT(*) FROM bee_tasks
			WHERE type IN (?,?)
			  AND status NOT IN (?,?,?)`,
			model.TaskTypeCountdown, model.TaskTypeScheduled,
			model.TaskStatusCompleted, model.TaskStatusCancelled, model.TaskStatusFailed,
		).Scan(&ov.ScheduledTasks)
	})

	if err := eg.Wait(); err != nil {
		return StatsOverview{}, fmt.Errorf("get overview: %w", err)
	}

	ov.MessagesTotalToday = ov.MessagesReceivedToday + ov.MessagesSentToday

	if ov.ActiveWorkersYesterday > 0 {
		change := float64(ov.ActiveWorkersToday-ov.ActiveWorkersYesterday) / float64(ov.ActiveWorkersYesterday)
		ov.ActiveWorkersChange = &change
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
	for i := range days {
		date := startOfRange.AddDate(0, 0, i).Format("2006-01-02")
		points[i] = TrendPoint{Date: date, ActiveWorkers: dbCounts[date]}
	}
	return points, nil
}

// ExecDurationTrendPoint is one day's total execution duration.
type ExecDurationTrendPoint struct {
	Date            string `json:"date"`
	TotalDurationMS int64  `json:"total_duration_ms"`
}

// GetExecutionDurationTrend returns the sum of completed execution durations
// for each of the last `days` days (local time), filling missing days with zero.
func (s *StatsStore) GetExecutionDurationTrend(ctx context.Context, days int) ([]ExecDurationTrendPoint, error) {
	now := time.Now()
	y, m, d := now.Date()
	loc := now.Location()

	startOfToday := time.Date(y, m, d, 0, 0, 0, 0, loc)
	startOfRange := startOfToday.AddDate(0, 0, -(days - 1))
	startMS := startOfRange.UnixMilli()
	endMS := startOfToday.AddDate(0, 0, 1).UnixMilli()

	rows, err := s.db.QueryContext(ctx, `
		SELECT DATE(started_at/1000, 'unixepoch', 'localtime') AS day,
		       COALESCE(SUM(completed_at - started_at), 0) AS total_ms
		FROM bee_executions
		WHERE status = 'completed'
		  AND completed_at IS NOT NULL
		  AND started_at >= ? AND started_at < ?
		GROUP BY day
		ORDER BY day ASC`, startMS, endMS)
	if err != nil {
		return nil, fmt.Errorf("execution duration trend query: %w", err)
	}
	defer rows.Close()

	dbTotals := make(map[string]int64, days)
	for rows.Next() {
		var day string
		var total int64
		if err := rows.Scan(&day, &total); err != nil {
			return nil, err
		}
		dbTotals[day] = total
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("execution duration trend rows: %w", err)
	}

	points := make([]ExecDurationTrendPoint, days)
	for i := range days {
		date := startOfRange.AddDate(0, 0, i).Format("2006-01-02")
		points[i] = ExecDurationTrendPoint{Date: date, TotalDurationMS: dbTotals[date]}
	}
	return points, nil
}
