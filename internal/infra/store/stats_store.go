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

// StatsOverview holds the numeric dashboard card data: organization counts and
// today/yesterday token usage (the figures the dashboard's basic-info and token
// panels render).
type StatsOverview struct {
	Departments      int   `json:"departments"`
	Workers          int   `json:"workers"`
	ScheduledTasks   int   `json:"scheduled_tasks"`
	TokensTodayTotal int64 `json:"tokens_today_total"`
	TokensYestTotal  int64 `json:"tokens_yesterday_total"`
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

// GetOverview returns the numeric card statistics: organization counts and
// today/yesterday token usage.
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

	eg.Go(func() error {
		return s.db.QueryRowContext(egc, `
			SELECT COUNT(*) FROM bee_tasks
			WHERE type IN (?,?)
			  AND status NOT IN (?,?,?)`,
			model.TaskTypeCountdown, model.TaskTypeScheduled,
			model.TaskStatusCompleted, model.TaskStatusCancelled, model.TaskStatusFailed,
		).Scan(&ov.ScheduledTasks)
	})

	// A session active across midnight is attributed to each day it appears in,
	// matching the cross-day disclosure in the UI (tokensCrossDayNote).
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
	eg.Go(scanTokenRange(todayStart, todayEnd, &ov.TokensTodayTotal))
	eg.Go(scanTokenRange(yestStart, yestEnd, &ov.TokensYestTotal))

	if err := eg.Wait(); err != nil {
		return StatsOverview{}, fmt.Errorf("get overview: %w", err)
	}

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
