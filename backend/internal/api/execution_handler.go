package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/domain/execution"
	"github.com/theopenbee/openbee/internal/infra/auth"
)

type ExecutionHandler struct {
	service *execution.Service
}

func NewExecutionHandler(service *execution.Service) *ExecutionHandler {
	return &ExecutionHandler{service: service}
}

func (h *ExecutionHandler) Get(c *gin.Context) {
	detail, err := h.service.Detail(c.Request.Context(), auth.UserID(c), c.Param("id"))
	switch {
	case err == nil:
		c.JSON(http.StatusOK, detail)
	case errors.Is(err, execution.ErrNotFound):
		respondError(c, http.StatusNotFound, apperr.New("execution_not_found", err.Error()))
	default:
		respondError(c, http.StatusInternalServerError, err)
	}
}
