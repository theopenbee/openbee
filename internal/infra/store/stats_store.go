package store

import (
	"context"
	"database/sql"
	"fmt"
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

// StatsOverview holds all numeric dashboard card data.
type StatsOverview struct {
	Departments             int      `json:"departments"`
	Workers                 int      `json:"workers"`
	ActiveWorkersToday      int      `json:"active_workers_today"`
	ActiveWorkersYesterday  int      `json:"active_workers_yesterday"`
	ActiveWorkersChange     *float64 `json:"active_workers_change"`
	MessagesTotalToday      int      `json:"messages_total_today"`
	MessagesTotalYesterday  int      `json:"messages_total_yesterday"`
	MessagesChange          *float64 `json:"messages_change"`
	MessagesTotalGlobal     int      `json:"messages_total_global"`
	ExecutionsToday         int      `json:"executions_today"`
	ExecutionsYesterday     int      `json:"executions_yesterday"`
	ExecutionsChange        *float64 `json:"executions_change"`
	ExecDurationTodayMS     int64    `json:"exec_duration_today_ms"`
	ExecDurationYesterdayMS int64    `json:"exec_duration_yesterday_ms"`
	ExecDurationTotalMS     int64    `json:"exec_duration_total_ms"`
	ScheduledTasks          int      `json:"scheduled_tasks"`
	TokensTotal      int64 `json:"tokens_total"`
	TokensTodayTotal int64 `json:"tokens_today_total"`
	TokensYestTotal  int64 `json:"tokens_yesterday_total"`
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

	// All local vars below are written exclusively inside their own goroutines
	// and read only after eg.Wait(), which provides the necessary happens-before guarantee.
	var (
		msgRecToday, msgSentToday int
		msgRecYest, msgSentYest   int
		globalReceived, globalSent int
	)

	eg.Go(func() error {
		return s.db.QueryRowContext(egc,
			`SELECT COUNT(*) FROM bee_platform_messages WHERE received_at >= ? AND received_at < ?`,
			todayStart, todayEnd,
		).Scan(&msgRecToday)
	})

	eg.Go(func() error {
		return s.db.QueryRowContext(egc,
			`SELECT COUNT(*) FROM bee_outbound_messages WHERE sent_at >= ? AND sent_at < ?`,
			todayStart, todayEnd,
		).Scan(&msgSentToday)
	})

	eg.Go(func() error {
		return s.db.QueryRowContext(egc,
			`SELECT COUNT(*) FROM bee_platform_messages WHERE received_at >= ? AND received_at < ?`,
			yestStart, yestEnd,
		).Scan(&msgRecYest)
	})

	eg.Go(func() error {
		return s.db.QueryRowContext(egc,
			`SELECT COUNT(*) FROM bee_outbound_messages WHERE sent_at >= ? AND sent_at < ?`,
			yestStart, yestEnd,
		).Scan(&msgSentYest)
	})

	eg.Go(func() error {
		return s.db.QueryRowContext(egc, `SELECT COUNT(*) FROM bee_platform_messages`).Scan(&globalReceived)
	})

	eg.Go(func() error {
		return s.db.QueryRowContext(egc, `SELECT COUNT(*) FROM bee_outbound_messages`).Scan(&globalSent)
	})

	eg.Go(func() error {
		return s.db.QueryRowContext(egc, `
			SELECT
			  COALESCE(SUM(CASE WHEN started_at >= ? AND started_at < ? THEN completed_at - started_at END), 0),
			  COALESCE(SUM(CASE WHEN started_at >= ? AND started_at < ? THEN completed_at - started_at END), 0),
			  COALESCE(SUM(completed_at - started_at), 0)
			FROM bee_executions
			WHERE status = ? AND completed_at IS NOT NULL`,
			todayStart, todayEnd, yestStart, yestEnd, string(model.ExecStatusCompleted),
		).Scan(&ov.ExecDurationTodayMS, &ov.ExecDurationYesterdayMS, &ov.ExecDurationTotalMS)
	})

	eg.Go(func() error {
		return s.db.QueryRowContext(egc,
			`SELECT COUNT(*) FROM bee_executions WHERE started_at >= ? AND started_at < ?`,
			todayStart, todayEnd,
		).Scan(&ov.ExecutionsToday)
	})

	var execYestTotal int
	eg.Go(func() error {
		return s.db.QueryRowContext(egc,
			`SELECT COUNT(*) FROM bee_executions WHERE started_at >= ? AND started_at < ?`,
			yestStart, yestEnd,
		).Scan(&execYestTotal)
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

	var tokensTotal, tokensTodayTotal, tokensYestTotal int64

	eg.Go(func() error {
		return s.db.QueryRowContext(egc,
			`SELECT COALESCE(SUM(total_tokens),0) FROM bee_token_stats`,
		).Scan(&tokensTotal)
	})

	const tokenRangeQuery = `
		SELECT COALESCE(SUM(ts.total_tokens),0)
		FROM bee_token_stats ts
		WHERE ts.session_id IN (
		  SELECT DISTINCT session_id FROM bee_executions
		  WHERE completed_at >= ? AND completed_at < ?
		    AND session_id IS NOT NULL
		)`
	scanTokenRange := func(startMS, endMS int64, total *int64) func() error {
		return func() error {
			return s.db.QueryRowContext(egc, tokenRangeQuery, startMS, endMS).Scan(total)
		}
	}
	eg.Go(scanTokenRange(todayStart, todayEnd, &tokensTodayTotal))
	eg.Go(scanTokenRange(yestStart, yestEnd, &tokensYestTotal))

	if err := eg.Wait(); err != nil {
		return StatsOverview{}, fmt.Errorf("get overview: %w", err)
	}

	ov.MessagesTotalToday = msgRecToday + msgSentToday
	ov.MessagesTotalYesterday = msgRecYest + msgSentYest
	ov.MessagesTotalGlobal = globalReceived + globalSent

	if ov.ActiveWorkersYesterday > 0 {
		change := float64(ov.ActiveWorkersToday-ov.ActiveWorkersYesterday) / float64(ov.ActiveWorkersYesterday)
		ov.ActiveWorkersChange = &change
	}

	if ov.MessagesTotalYesterday > 0 {
		change := float64(ov.MessagesTotalToday-ov.MessagesTotalYesterday) / float64(ov.MessagesTotalYesterday)
		ov.MessagesChange = &change
	}

	ov.ExecutionsYesterday = execYestTotal
	if execYestTotal > 0 {
		change := float64(ov.ExecutionsToday-execYestTotal) / float64(execYestTotal)
		ov.ExecutionsChange = &change
	}

	ov.TokensTotal = tokensTotal
	ov.TokensTodayTotal = tokensTodayTotal
	ov.TokensYestTotal = tokensYestTotal

	return ov, nil
}

// buildDailySlice creates a slice of P with one entry per day in the window,
// calling fn(date) for each day where date is formatted as "2006-01-02".
func buildDailySlice[P any](startOfRange time.Time, days int, fn func(date string) P) []P {
	points := make([]P, days)
	for i := range days {
		date := startOfRange.AddDate(0, 0, i).Format("2006-01-02")
		points[i] = fn(date)
	}
	return points
}

// trendRange returns the millisecond epoch bounds and start-of-range time for a
// days-wide window ending at end-of-today (local time).
func trendRange(days int) (startOfRange time.Time, startMS, endMS int64) {
	now := time.Now()
	y, m, d := now.Date()
	loc := now.Location()
	startOfToday := time.Date(y, m, d, 0, 0, 0, 0, loc)
	startOfRange = startOfToday.AddDate(0, 0, -(days - 1))
	startMS = startOfRange.UnixMilli()
	endMS = startOfToday.AddDate(0, 0, 1).UnixMilli()
	return
}

// GetTrend returns active-worker counts for each of the last `days` days (local time),
// filling missing days with zero.
func (s *StatsStore) GetTrend(ctx context.Context, days int) ([]TrendPoint, error) {
	startOfRange, startMS, endMS := trendRange(days)

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

	points := buildDailySlice(startOfRange, days, func(date string) TrendPoint {
		return TrendPoint{Date: date, ActiveWorkers: dbCounts[date]}
	})
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
	startOfRange, startMS, endMS := trendRange(days)

	rows, err := s.db.QueryContext(ctx, `
		SELECT DATE(started_at/1000, 'unixepoch', 'localtime') AS day,
		       COALESCE(SUM(completed_at - started_at), 0) AS total_ms
		FROM bee_executions
		WHERE status = ?
		  AND completed_at IS NOT NULL
		  AND started_at >= ? AND started_at < ?
		GROUP BY day
		ORDER BY day ASC`, string(model.ExecStatusCompleted), startMS, endMS)
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

	points := buildDailySlice(startOfRange, days, func(date string) ExecDurationTrendPoint {
		return ExecDurationTrendPoint{Date: date, TotalDurationMS: dbTotals[date]}
	})
	return points, nil
}

// TokenTrendPoint is one day's total token usage.
type TokenTrendPoint struct {
	Date        string `json:"date"`
	TotalTokens int64  `json:"total_tokens"`
}

// GetTokenTrend returns token usage per day for the last `days` days.
// A session active across midnight is attributed to each day it appears in,
// matching the cross-day disclosure in the UI (tokensCrossDayNote).
func (s *StatsStore) GetTokenTrend(ctx context.Context, days int) ([]TokenTrendPoint, error) {
	startOfRange, startMS, endMS := trendRange(days)

	rows, err := s.db.QueryContext(ctx, `
		SELECT day, COALESCE(SUM(ts.total_tokens), 0) AS tokens
		FROM (
		  SELECT DISTINCT session_id,
		         DATE(completed_at/1000, 'unixepoch', 'localtime') AS day
		  FROM bee_executions
		  WHERE completed_at >= ? AND completed_at < ?
		    AND session_id IS NOT NULL
		) sessions
		JOIN bee_token_stats ts ON ts.session_id = sessions.session_id
		GROUP BY day
		ORDER BY day ASC`, startMS, endMS)
	if err != nil {
		return nil, fmt.Errorf("token trend query: %w", err)
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
		return nil, fmt.Errorf("token trend rows: %w", err)
	}

	points := buildDailySlice(startOfRange, days, func(date string) TokenTrendPoint {
		return TokenTrendPoint{Date: date, TotalTokens: dbTotals[date]}
	})
	return points, nil
}
