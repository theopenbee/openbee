package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/utils"
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

func parseInt64Query(c *gin.Context, key string) int64 {
	v := c.Query(key)
	return utils.ParseMillis(&v)
}

func paginatedResponse(items any, total, page, pageSize int) gin.H {
	return gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}
}
