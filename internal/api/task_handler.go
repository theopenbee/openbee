package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/model"
	"github.com/theopenbee/openbee/internal/store"
	"golang.org/x/sync/errgroup"
)

func (s *Server) registerTaskRoutes(api *gin.RouterGroup) {
	api.GET("/tasks", s.listTasks)
	api.DELETE("/tasks/:id", s.cancelTask)
	api.POST("/workers/:id/tasks/cancel-all", s.cancelWorkerTasks)
}

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

func (s *Server) listTasks(c *gin.Context) {
	page, pageSize, offset := parsePagination(c)

	taskType := c.DefaultQuery("type", "scheduled,countdown")
	taskStatus := c.DefaultQuery("status", "pending,running")
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
	g, gCtx := errgroup.WithContext(c.Request.Context())
	g.Go(func() error {
		var err error
		total, err = s.TaskStore.CountTasks(gCtx, filter)
		return err
	})
	g.Go(func() error {
		var err error
		tasks, err = s.TaskStore.List(gCtx, filter)
		return err
	})
	if err := g.Wait(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	workerNames := make(map[string]string)
	if workerID != "" {
		if w, err := s.WorkerStore.GetByID(workerID); err == nil {
			workerNames[w.ID] = w.Name
		}
	} else {
		workers, err := s.WorkerStore.List()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		workerNames = make(map[string]string, len(workers))
		for _, w := range workers {
			workerNames[w.ID] = w.Name
		}
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

func (s *Server) cancelTask(c *gin.Context) {
	id := c.Param("id")

	task, err := s.TaskStore.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if task.Status != model.TaskStatusPending {
		c.JSON(http.StatusConflict, gin.H{"error": "task is not in pending state"})
		return
	}

	if err := s.TaskStore.CancelTask(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) cancelWorkerTasks(c *gin.Context) {
	workerID := c.Param("id")

	if err := s.TaskStore.CancelByWorkerID(c.Request.Context(), workerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
