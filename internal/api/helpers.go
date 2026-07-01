package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

// respondError writes a JSON error body carrying the raw message plus, when the
// error is coded (see internal/apperr), a stable "code" the frontend maps to a
// localized string and optional interpolation "params".
func respondError(c *gin.Context, status int, err error) {
	respondErrorCode(c, status, err, "")
}

// respondErrorCode is like respondError but applies defaultCode when err itself
// carries no code. Used for domain sentinels (e.g. ErrNotFound) that have no
// per-instance code of their own.
func respondErrorCode(c *gin.Context, status int, err error, defaultCode string) {
	body := gin.H{"error": err.Error()}
	code := apperr.Code(err)
	if code == "" {
		code = defaultCode
	}
	if code != "" {
		body["code"] = code
	}
	if params := apperr.Params(err); len(params) > 0 {
		body["params"] = params
	}
	c.JSON(status, body)
}

func respondDomainError(c *gin.Context, err, errNotFound, errValidation error, notFoundCode, validationCode string) {
	switch {
	case errors.Is(err, errNotFound):
		respondErrorCode(c, http.StatusNotFound, err, notFoundCode)
	case errors.Is(err, errValidation):
		respondErrorCode(c, http.StatusBadRequest, err, validationCode)
	default:
		respondError(c, http.StatusInternalServerError, err)
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
