package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"golang.org/x/sync/errgroup"
)

type taskResponse struct {
	ID          string `json:"id"`
	WorkerID    string `json:"worker_id"`
	WorkerName  string `json:"worker_name"`
	Instruction string `json:"instruction"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	ScheduledAt *int64 `json:"scheduled_at"`
	CronExpr    string `json:"cron_expr"`
	NextRunAt   *int64 `json:"next_run_at"`
	ExecutionID string `json:"execution_id"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type TaskCanceller interface {
	CancelTask(ctx context.Context, taskID string) error
}

type TaskHandler struct {
	tasks     *store.TaskStore
	workers   *store.WorkerStore
	canceller TaskCanceller
}

func NewTaskHandler(ts *store.TaskStore, ws *store.WorkerStore, canceller TaskCanceller) *TaskHandler {
	return &TaskHandler{tasks: ts, workers: ws, canceller: canceller}
}

func (h *TaskHandler) List(c *gin.Context) {
	page, pageSize, offset := parsePagination(c)

	taskType := c.DefaultQuery("type", model.TaskTypeScheduled+","+model.TaskTypeCountdown)
	taskStatus := c.DefaultQuery("status", model.TaskStatusActive)
	workerID := c.Query("worker_id")

	filter := store.TaskFilter{
		Type:     taskType,
		Status:   taskStatus,
		WorkerID: workerID,
		Limit:    pageSize,
		Offset:   offset,
	}

	var total int
	var tasks []model.Task
	var workerList []model.Worker
	g, gCtx := errgroup.WithContext(c.Request.Context())
	g.Go(func() error {
		var err error
		total, err = h.tasks.CountTasks(gCtx, filter)
		return err
	})
	g.Go(func() error {
		var err error
		tasks, err = h.tasks.List(gCtx, filter)
		return err
	})
	if workerID != "" {
		g.Go(func() error {
			w, err := h.workers.GetByID(workerID)
			if err == nil {
				workerList = []model.Worker{w}
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			return nil
		})
	} else {
		g.Go(func() error {
			var err error
			workerList, err = h.workers.List()
			return err
		})
	}
	if err := g.Wait(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	workerNames := make(map[string]string, len(workerList))
	for _, w := range workerList {
		workerNames[w.ID] = w.Name
	}

	items := make([]taskResponse, len(tasks))
	for i, t := range tasks {
		items[i] = taskResponse{
			ID:          t.ID,
			WorkerID:    t.WorkerID,
			WorkerName:  workerNames[t.WorkerID],
			Instruction: t.Instruction,
			Type:        t.Type,
			Status:      t.Status,
			ScheduledAt: t.ScheduledAt,
			CronExpr:    t.CronExpr,
			NextRunAt:   t.NextRunAt,
			ExecutionID: t.ExecutionID,
			CreatedAt:   t.CreatedAt,
			UpdatedAt:   t.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, paginatedResponse(items, total, page, pageSize))
}

func (h *TaskHandler) Cancel(c *gin.Context) {
	id := c.Param("id")

	task, err := h.tasks.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if task.Status != model.TaskStatusPending && task.Status != model.TaskStatusRunning {
		c.JSON(http.StatusConflict, gin.H{"error": "task cannot be cancelled"})
		return
	}

	if err := h.canceller.CancelTask(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *TaskHandler) CancelByWorker(c *gin.Context) {
	workerID := c.Param("id")

	if err := h.tasks.CancelByWorkerID(c.Request.Context(), workerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
