package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform/local"
	"golang.org/x/sync/errgroup"
)

const (
	maxTaskTargets   = 100
	minCountdownLead = 5 * time.Second
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
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type TaskCanceller interface {
	CancelTask(ctx context.Context, taskID string) error
}

type TaskHandler struct {
	tasks     *store.TaskStore
	workers   *store.WorkerStore
	messages  *store.MessageStore
	canceller TaskCanceller
}

func NewTaskHandler(ts *store.TaskStore, ws *store.WorkerStore, ms *store.MessageStore, canceller TaskCanceller) *TaskHandler {
	return &TaskHandler{tasks: ts, workers: ws, messages: ms, canceller: canceller}
}

func newTaskResponse(t model.Task, workerName string) taskResponse {
	return taskResponse{
		ID:          t.ID,
		WorkerID:    t.WorkerID,
		WorkerName:  workerName,
		Instruction: t.Instruction,
		Type:        t.Type,
		Status:      t.Status,
		ScheduledAt: t.ScheduledAt,
		CronExpr:    t.CronExpr,
		NextRunAt:   t.NextRunAt,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
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
		items[i] = newTaskResponse(t, workerNames[t.WorkerID])
	}

	c.JSON(http.StatusOK, paginatedResponse(items, total, page, pageSize))
}

type taskRequest struct {
	WorkerID    string `json:"worker_id"`
	Instruction string `json:"instruction"`
	Type        string `json:"type"`
	ScheduledAt *int64 `json:"scheduled_at"`
	CronExpr    string `json:"cron_expr"`
}

type createTaskRequest struct {
	taskRequest
	SessionKey string `json:"session_key"`
}

type taskTargetResponse struct {
	SessionKey   string `json:"session_key"`
	Platform     string `json:"platform"`
	LastActiveAt int64  `json:"last_active_at"`
}

var (
	errTaskNotFound    = apperr.New("task_not_found", "task not found")
	errTaskNotEditable = apperr.New("task_not_editable", "only pending scheduled or countdown tasks can be edited")
	errTaskTarget      = apperr.New("task_target_not_found", "reply session not found")
)

func (h *TaskHandler) Targets(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		return
	}
	sessions, err := h.messages.ListRecentSessions(c.Request.Context(), local.PlatformID, local.SessionKey(uid), maxTaskTargets)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	items := make([]taskTargetResponse, len(sessions))
	for i, s := range sessions {
		items[i] = taskTargetResponse{SessionKey: s.SessionKey, Platform: s.Platform, LastActiveAt: s.LastActiveAt}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *TaskHandler) Create(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		return
	}
	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	ctx := c.Request.Context()

	task, workerName, err := h.buildTask(req.taskRequest)
	if err != nil {
		respondTaskInputError(c, err)
		return
	}
	task.MessageID, err = h.resolveTarget(ctx, uid, req.SessionKey)
	if err != nil {
		respondTaskInputError(c, err)
		return
	}
	task.Status = model.TaskStatusPending

	id, err := h.tasks.Create(ctx, task)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	created, err := h.tasks.GetByID(ctx, id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusCreated, newTaskResponse(created, workerName))
}

func (h *TaskHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req taskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	ctx := c.Request.Context()

	existing, err := h.tasks.GetByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(c, http.StatusNotFound, errTaskNotFound)
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	if existing.Status != model.TaskStatusPending || existing.Type == model.TaskTypeImmediate {
		respondError(c, http.StatusConflict, errTaskNotEditable)
		return
	}

	task, workerName, err := h.buildTask(req)
	if err != nil {
		respondTaskInputError(c, err)
		return
	}
	task.ID = id
	updated, err := h.tasks.UpdatePending(ctx, task)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	if !updated {
		respondError(c, http.StatusConflict, errTaskNotEditable)
		return
	}
	saved, err := h.tasks.GetByID(ctx, id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, newTaskResponse(saved, workerName))
}

func (h *TaskHandler) buildTask(req taskRequest) (model.Task, string, error) {
	task := model.Task{
		WorkerID:    strings.TrimSpace(req.WorkerID),
		Instruction: strings.TrimSpace(req.Instruction),
		Type:        req.Type,
	}
	if task.Instruction == "" {
		return model.Task{}, "", apperr.New("task_instruction_required", "instruction is required")
	}

	now := time.Now()
	switch req.Type {
	case model.TaskTypeCountdown:
		if req.ScheduledAt == nil || *req.ScheduledAt < now.Add(minCountdownLead).UnixMilli() {
			return model.Task{}, "", apperr.New("task_scheduled_at_invalid", "scheduled_at must be at least 5 seconds in the future")
		}
		task.ScheduledAt = req.ScheduledAt
	case model.TaskTypeScheduled:
		expr := strings.TrimSpace(req.CronExpr)
		sched, err := cron.ParseStandard(expr)
		if err != nil {
			return model.Task{}, "", apperr.New("task_cron_invalid", "invalid cron_expr: "+err.Error())
		}
		next := sched.Next(now).UnixMilli()
		task.CronExpr = expr
		task.NextRunAt = &next
	default:
		return model.Task{}, "", apperr.New("task_type_invalid", "type must be scheduled or countdown")
	}

	if task.WorkerID == "" {
		return model.Task{}, "", apperr.New("worker_not_found", "worker not found")
	}
	worker, err := h.workers.GetByID(task.WorkerID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Task{}, "", apperr.New("worker_not_found", "worker not found")
	}
	if err != nil {
		return model.Task{}, "", err
	}
	return task, worker.Name, nil
}

func (h *TaskHandler) resolveTarget(ctx context.Context, userID, sessionKey string) (string, error) {
	if sessionKey == "" {
		return "", errTaskTarget
	}
	msg, err := h.messages.LatestBySessionKey(ctx, sessionKey)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errTaskTarget
	}
	if err != nil {
		return "", err
	}
	if msg.Platform == local.PlatformID && sessionKey != local.SessionKey(userID) {
		return "", errTaskTarget
	}
	return msg.ID, nil
}

func respondTaskInputError(c *gin.Context, err error) {
	if apperr.Code(err) != "" {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondError(c, http.StatusInternalServerError, err)
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
