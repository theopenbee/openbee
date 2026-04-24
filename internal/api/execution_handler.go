package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type modelTokenStats struct {
	Model               string `json:"model"`
	TotalTokens         int64  `json:"total_tokens"`
	InputTokens         int64  `json:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens"`
}

type sessionTokenStats struct {
	TotalTokens int64             `json:"total_tokens"`
	ByModel     []modelTokenStats `json:"by_model"`
}

type ExecutionHandler struct {
	executions *store.ExecutionStore
	tokenStats *store.TokenStatsStore
}

func NewExecutionHandler(es *store.ExecutionStore, ts *store.TokenStatsStore) *ExecutionHandler {
	return &ExecutionHandler{executions: es, tokenStats: ts}
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

	// When no filters are applied, paginate at the session level so that each
	// page contains a consistent number of sessions (the frontend groups by session).
	if f == (store.ExecutionFilter{}) {
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
		c.JSON(http.StatusOK, gin.H{
			"items":       execs,
			"total":       total,
			"page":        page,
			"page_size":   pageSize,
			"token_stats": h.buildTokenStatsMap(execs),
		})
		return
	}

	execs, total, err := h.executions.ListFiltered(c.Request.Context(), f, pageSize, offset)
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

	var since int64
	if raw := c.Query("since"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid since parameter"})
			return
		}
		since = n
	}

	slice, err := h.executions.ReadLogSince(id, since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if slice.Status == model.ExecStatusCompleted || slice.Status == model.ExecStatusFailed {
		c.Header("Cache-Control", "public, max-age=3600")
	}
	c.JSON(http.StatusOK, gin.H{
		"content":   slice.Content,
		"size":      slice.Size,
		"truncated": slice.Truncated,
	})
}

func (h *ExecutionHandler) buildTokenStatsMap(execs []model.WorkerExecution) map[string]*sessionTokenStats {
	seen := make(map[string]struct{})
	var sessionIDs []string
	for _, e := range execs {
		if _, ok := seen[e.SessionID]; !ok {
			seen[e.SessionID] = struct{}{}
			sessionIDs = append(sessionIDs, e.SessionID)
		}
	}
	if len(sessionIDs) == 0 {
		return nil
	}
	rows, err := h.tokenStats.GetBySessionIDs(sessionIDs)
	if err != nil {
		return nil
	}
	result := make(map[string]*sessionTokenStats)
	for _, row := range rows {
		entry := result[row.SessionID]
		if entry == nil {
			entry = &sessionTokenStats{}
			result[row.SessionID] = entry
		}
		entry.TotalTokens += row.TotalTokens
		entry.ByModel = append(entry.ByModel, modelTokenStats{
			Model:               row.Model,
			TotalTokens:         row.TotalTokens,
			InputTokens:         row.InputTokens,
			OutputTokens:        row.OutputTokens,
			CacheCreationTokens: row.CacheCreationTokens,
			CacheReadTokens:     row.CacheReadTokens,
		})
	}
	return result
}
