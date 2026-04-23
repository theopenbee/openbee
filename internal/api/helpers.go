package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

func respondDomainError(c *gin.Context, err, errNotFound, errValidation error) {
	switch {
	case errors.Is(err, errNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, errValidation):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

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

func parseInt64Query(c *gin.Context, key string) int64 {
	v := c.Query(key)
	return utils.ParseMillis(&v)
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

func paginatedResponse(items any, total, page, pageSize int) gin.H {
	return gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}
}
