package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/apperr"
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
