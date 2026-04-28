package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

// SubtaskFailureNotifier sends failure notifications; it is optional.
type SubtaskFailureNotifier interface {
	NotifyTaskFailure(ctx context.Context, messageID string, info model.FailureInfo) error
}

// SubtaskDispatcher resumes the Group after all subtasks complete.
type SubtaskDispatcher interface {
	NotifySubtaskProgress(ctx context.Context, task model.Task, content string)
	NotifyAllSubtasksTerminal(ctx context.Context, rootTaskID string)
}

type SubtaskHandler struct {
	taskStore  *store.TaskStore
	groupStore *store.GroupStore
	notifier   SubtaskFailureNotifier
	dispatcher SubtaskDispatcher
}

func NewSubtaskHandler(ts *store.TaskStore, gs *store.GroupStore, n SubtaskFailureNotifier, d SubtaskDispatcher) *SubtaskHandler {
	return &SubtaskHandler{taskStore: ts, groupStore: gs, notifier: n, dispatcher: d}
}

func (h *SubtaskHandler) Dispatch(c *gin.Context) {
	var req struct {
		ParentTaskID string `json:"parent_task_id"`
		WorkerID     string `json:"worker_id"`
		Instruction  string `json:"instruction"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	parent, err := h.taskStore.GetByID(c.Request.Context(), req.ParentTaskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "parent task not found"})
		return
	}
	if parent.AgentKind != model.AgentKindGroup {
		c.JSON(http.StatusBadRequest, gin.H{"error": "parent task is not a group task"})
		return
	}
	subID, err := h.taskStore.Create(c.Request.Context(), model.Task{
		MessageID:    parent.MessageID,
		WorkerID:     req.WorkerID,
		Instruction:  req.Instruction,
		Type:         model.TaskTypeImmediate,
		Status:       model.TaskStatusPending,
		ParentTaskID: parent.ID,
		RootTaskID:   parent.RootTaskID,
		AgentKind:    model.AgentKindWorker,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"subtask_id": subID})
}

func (h *SubtaskHandler) ListSubtasks(c *gin.Context) {
	rootID := c.Query("task_id")
	if rootID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id required"})
		return
	}
	list, err := h.taskStore.ListByRoot(c.Request.Context(), rootID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []model.Task{}
	}
	c.JSON(http.StatusOK, list)
}

func (h *SubtaskHandler) Suspend(c *gin.Context) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.taskStore.MarkWaitingSubtasks(c.Request.Context(), req.TaskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Check if all subtasks are already terminal — if so, trigger immediate resume.
	if h.dispatcher != nil {
		list, _ := h.taskStore.ListByRoot(c.Request.Context(), req.TaskID)
		allTerminal := true
		for _, t := range list {
			if t.ID == req.TaskID {
				continue
			}
			if t.Status == model.TaskStatusPending ||
				t.Status == model.TaskStatusRunning ||
				t.Status == model.TaskStatusWaitingSubtasks {
				allTerminal = false
				break
			}
		}
		if allTerminal && len(list) > 1 {
			h.dispatcher.NotifyAllSubtasksTerminal(c.Request.Context(), req.TaskID)
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "waiting_subtasks"})
}

func (h *SubtaskHandler) MarkSuccess(c *gin.Context) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if active, err := h.hasActiveSubtasks(c.Request.Context(), req.TaskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else if active {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot mark success while subtasks are active"})
		return
	}
	if err := h.taskStore.CompleteTask(c.Request.Context(), req.TaskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "completed"})
}

func (h *SubtaskHandler) MarkFailed(c *gin.Context) {
	var req struct {
		TaskID string `json:"task_id"`
		Reason string `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	t, err := h.taskStore.GetByID(c.Request.Context(), req.TaskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if err := h.taskStore.FailTask(c.Request.Context(), req.TaskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.cancelActiveSubtasks(c.Request.Context(), req.TaskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.notifier != nil && t.MessageID != "" {
		_ = h.notifier.NotifyTaskFailure(c.Request.Context(), t.MessageID, model.FailureInfo{
			Reason:     req.Reason,
			WorkerName: t.WorkerID,
		})
	}
	c.JSON(http.StatusOK, gin.H{"status": "failed"})
}

func (h *SubtaskHandler) hasActiveSubtasks(ctx context.Context, rootID string) (bool, error) {
	list, err := h.taskStore.ListByRoot(ctx, rootID)
	if err != nil {
		return false, err
	}
	for _, t := range list {
		if t.ID != rootID && isActiveSubtaskStatus(t.Status) {
			return true, nil
		}
	}
	return false, nil
}

func (h *SubtaskHandler) cancelActiveSubtasks(ctx context.Context, rootID string) error {
	list, err := h.taskStore.ListByRoot(ctx, rootID)
	if err != nil {
		return err
	}
	for _, t := range list {
		if t.ID == rootID || !isActiveSubtaskStatus(t.Status) {
			continue
		}
		if err := h.taskStore.CancelTask(ctx, t.ID); err != nil {
			return err
		}
	}
	return nil
}

func isActiveSubtaskStatus(status string) bool {
	return status == model.TaskStatusPending ||
		status == model.TaskStatusRunning ||
		status == model.TaskStatusWaitingSubtasks
}
