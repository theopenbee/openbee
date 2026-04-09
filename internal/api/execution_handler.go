package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

func parsePagination(c *gin.Context) (page, pageSize, offset int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ = strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset = (page - 1) * pageSize
	return
}

func paginatedResponse(items any, total, page, pageSize int) gin.H {
	return gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}
}

// ExecutionHandler handles HTTP requests for execution resources.
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

	total, err := h.executions.CountSessions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	execs, err := h.executions.ListPaginated(pageSize, offset)
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
	sessionID := c.Param("sessionId")
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
