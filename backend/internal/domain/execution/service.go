package execution

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"slices"
	"sync"

	"go.uber.org/zap"

	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform/local"
)

const (
	maxRuns         = 20
	maxCacheEntries = 16

	StepsErrorUnsupported = "transcript_unsupported"
	StepsErrorUnavailable = "engine_unavailable"
	StepsErrorFailed      = "transcript_failed"
)

var ErrNotFound = errors.New("execution not found")

var log = logger.With(zap.String("component", "execution"))

type executionStore interface {
	GetByID(id string) (model.WorkerExecution, error)
	ListByTaskIDs(ctx context.Context, taskIDs []string, limitPerTask int) (map[string][]model.WorkerExecution, error)
}

type taskStore interface {
	GetByID(ctx context.Context, id string) (model.Task, error)
}

type Summary struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	StartedAt   int64  `json:"started_at"`
	CompletedAt *int64 `json:"completed_at"`
}

type Detail struct {
	ID          string              `json:"id"`
	TaskID      string              `json:"task_id"`
	WorkerID    string              `json:"worker_id"`
	Engine      string              `json:"engine"`
	Status      string              `json:"status"`
	Instruction string              `json:"instruction"`
	StartedAt   int64               `json:"started_at"`
	CompletedAt *int64              `json:"completed_at"`
	Error       string              `json:"error"`
	Steps       []ai.TranscriptStep `json:"steps"`
	StepsError  string              `json:"steps_error"`
	Runs        []Summary           `json:"runs,omitempty"`
}

type Service struct {
	executions executionStore
	tasks      taskStore
	engines    ai.EngineSource
	cache      *stepsCache
}

func New(executions executionStore, tasks taskStore, engines ai.EngineSource) *Service {
	return &Service{executions: executions, tasks: tasks, engines: engines, cache: newStepsCache(maxCacheEntries)}
}

func (s *Service) Detail(ctx context.Context, userID, id string) (Detail, error) {
	exec, err := s.executions.GetByID(id)
	if errors.Is(err, sql.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	if exec.WorkerID == nil {
		return s.beeDetail(ctx, userID, exec)
	}
	if exec.TaskID == "" {
		return Detail{}, ErrNotFound
	}
	task, err := s.tasks.GetByID(ctx, exec.TaskID)
	if errors.Is(err, sql.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	if owner, _, ok := local.ParseSessionKey(task.SessionKey); ok && owner != userID {
		return Detail{}, ErrNotFound
	}

	detail := s.detail(ctx, exec, task.Instruction)
	detail.TaskID = exec.TaskID
	detail.WorkerID = *exec.WorkerID

	runs, err := s.executions.ListByTaskIDs(ctx, []string{exec.TaskID}, maxRuns)
	if err != nil {
		return Detail{}, err
	}
	if list := runs[exec.TaskID]; len(list) > 1 {
		detail.Runs = make([]Summary, 0, len(list))
		for _, run := range list {
			detail.Runs = append(detail.Runs, summarize(run))
		}
	}
	return detail, nil
}

func (s *Service) beeDetail(ctx context.Context, userID string, exec model.WorkerExecution) (Detail, error) {
	if owner, _, ok := local.ParseSessionKey(exec.SessionKey); !ok || owner != userID {
		return Detail{}, ErrNotFound
	}
	return s.detail(ctx, exec, exec.Instruction), nil
}

func (s *Service) detail(ctx context.Context, exec model.WorkerExecution, instruction string) Detail {
	detail := Detail{
		ID:          exec.ID,
		Engine:      exec.Engine,
		Status:      reportedStatus(exec.Status),
		Instruction: instruction,
		CompletedAt: exec.CompletedAt,
	}
	if exec.StartedAt != nil {
		detail.StartedAt = *exec.StartedAt
	}
	if exec.Status == model.ExecStatusFailed {
		detail.Error = exec.Result
	}
	detail.Steps, detail.StepsError = s.steps(ctx, exec)
	return detail
}

func reportedStatus(status model.ExecutionStatus) string {
	if status == model.ExecStatusPending {
		return string(model.ExecStatusRunning)
	}
	return string(status)
}

func summarize(exec model.WorkerExecution) Summary {
	summary := Summary{ID: exec.ID, Status: reportedStatus(exec.Status), CompletedAt: exec.CompletedAt}
	if exec.StartedAt != nil {
		summary.StartedAt = *exec.StartedAt
	}
	return summary
}

func (s *Service) steps(ctx context.Context, exec model.WorkerExecution) ([]ai.TranscriptStep, string) {
	empty := []ai.TranscriptStep{}
	if exec.LogPath == "" {
		return empty, ""
	}
	info, err := os.Stat(exec.LogPath)
	if errors.Is(err, fs.ErrNotExist) {
		return empty, ""
	}
	if err != nil {
		log.Error("stat execution log", zap.String("executionID", exec.ID), zap.Error(err))
		return empty, StepsErrorFailed
	}
	adapter, ok := s.engines.Engine(exec.Engine)
	if !ok {
		return empty, StepsErrorUnavailable
	}
	reader, ok := adapter.(ai.TranscriptReader)
	if !ok {
		return empty, StepsErrorUnsupported
	}

	key := cacheKey{path: exec.LogPath, size: info.Size(), modTime: info.ModTime().UnixNano()}
	if steps, ok := s.cache.get(key); ok {
		return steps, ""
	}
	steps, err := reader.Transcript(ctx, exec.LogPath)
	switch {
	case errors.Is(err, ai.ErrTranscriptUnsupported):
		return empty, StepsErrorUnsupported
	case err != nil:
		if ctx.Err() == nil {
			log.Error("read execution transcript", zap.String("executionID", exec.ID), zap.String("engine", exec.Engine), zap.Error(err))
		}
		return empty, StepsErrorFailed
	}
	if steps == nil {
		steps = empty
	}
	s.cache.put(key, steps)
	return slices.Clone(steps), ""
}

type cacheKey struct {
	path    string
	size    int64
	modTime int64
}

type stepsCache struct {
	mu      sync.Mutex
	limit   int
	order   []cacheKey
	entries map[cacheKey][]ai.TranscriptStep
}

func newStepsCache(limit int) *stepsCache {
	return &stepsCache{limit: limit, entries: make(map[cacheKey][]ai.TranscriptStep, limit)}
}

func (c *stepsCache) get(key cacheKey) ([]ai.TranscriptStep, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	steps, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	return slices.Clone(steps), true
}

func (c *stepsCache) put(key cacheKey, steps []ai.TranscriptStep) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range c.order {
		if k.path == key.path {
			delete(c.entries, k)
		}
	}
	c.order = slices.DeleteFunc(c.order, func(k cacheKey) bool { return k.path == key.path })
	for len(c.order) >= c.limit {
		delete(c.entries, c.order[0])
		c.order = c.order[1:]
	}
	c.entries[key] = slices.Clone(steps)
	c.order = append(c.order, key)
}
