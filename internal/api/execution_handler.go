package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type ExecutionHandler struct {
	executions *store.ExecutionStore
}

func NewExecutionHandler(es *store.ExecutionStore) *ExecutionHandler {
	return &ExecutionHandler{executions: es}
}

func (h *ExecutionHandler) ListByWorker(c *gin.Context) {
	workerID := c.Param("id")
	page, pageSize, offset := parsePagination(c)

	total, err := h.executions.CountSessionsByWorkerID(workerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	execs, err := h.executions.ListPaginatedByWorkerID(workerID, pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, paginatedResponse(execs, total, page, pageSize))
}

func (h *ExecutionHandler) List(c *gin.Context) {
	page, pageSize, offset := parsePagination(c)

	f := store.ExecutionFilter{
		WorkerID:      c.Query("worker_id"),
		SessionID:     c.Query("session_id"),
		Status:        c.Query("status"),
		StartedFrom:   parseInt64Query(c, "started_at_from"),
		StartedTo:     parseInt64Query(c, "started_at_to"),
		CompletedFrom: parseInt64Query(c, "completed_at_from"),
		CompletedTo:   parseInt64Query(c, "completed_at_to"),
	}

	execs, total, err := h.executions.ListFiltered(f, pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, paginatedResponse(execs, total, page, pageSize))
}

func (h *ExecutionHandler) Get(c *gin.Context) {
	exec, err := h.executions.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "execution not found"})
		return
	}
	c.JSON(http.StatusOK, exec)
}

func (h *ExecutionHandler) ListBySession(c *gin.Context) {
	sessionID := c.Query("session_id")
	execs, err := h.executions.ListBySessionID(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, execs)
}

func (h *ExecutionHandler) GetLogs(c *gin.Context) {
	id := c.Param("id")
	exec, err := h.executions.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	content, err := h.executions.ReadLog(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if exec.Status == model.ExecStatusCompleted || exec.Status == model.ExecStatusFailed {
		c.Header("Cache-Control", "public, max-age=3600")
	}
	c.String(http.StatusOK, content)
}
