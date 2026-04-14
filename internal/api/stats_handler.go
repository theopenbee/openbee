package api

import (
	"net/http"
	"strconv"

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

// parseDaysParam writes a 400 response and returns (0, false) on invalid input,
// so callers must not write an additional response on failure.
func parseDaysParam(c *gin.Context) (int, bool) {
	daysStr := c.DefaultQuery("days", "7")
	days, err := strconv.Atoi(daysStr)
	if err != nil || (days != 7 && days != 15 && days != 30) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "days must be 7, 15, or 30"})
		return 0, false
	}
	return days, true
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
