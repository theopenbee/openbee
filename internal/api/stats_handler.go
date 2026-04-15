package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type StatsHandler struct {
	stats *store.StatsStore
}

func NewStatsHandler(ss *store.StatsStore) *StatsHandler {
	return &StatsHandler{stats: ss}
}

func (h *StatsHandler) GetOverview(c *gin.Context) {
	ov, err := h.stats.GetOverview(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ov)
}

func (h *StatsHandler) GetTrend(c *gin.Context) {
	days, ok := parseDaysParam(c)
	if !ok {
		return
	}

	points, err := h.stats.GetTrend(c.Request.Context(), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "data": points})
}

func (h *StatsHandler) GetExecutionDurationTrend(c *gin.Context) {
	days, ok := parseDaysParam(c)
	if !ok {
		return
	}

	points, err := h.stats.GetExecutionDurationTrend(c.Request.Context(), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "data": points})
}
