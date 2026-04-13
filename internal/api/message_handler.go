package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/infra/store"
)

type MessageHandler struct {
	messages *store.MessageStore
}

func NewMessageHandler(ms *store.MessageStore) *MessageHandler {
	return &MessageHandler{messages: ms}
}

// List handles GET /api/messages with optional filters:
//   - session_key, platform, status (exact match)
//   - received_at_from, received_at_to (Unix ms, inclusive range)
//   - page, page_size (pagination)
func (h *MessageHandler) List(c *gin.Context) {
	page, pageSize, offset := parsePagination(c)

	f := store.MessageFilter{
		SessionKey:     c.Query("session_key"),
		Platform:       c.Query("platform"),
		Status:         c.Query("status"),
		ReceivedAtFrom: parseInt64Query(c, "received_at_from"),
		ReceivedAtTo:   parseInt64Query(c, "received_at_to"),
	}

	msgs, total, err := h.messages.ListFiltered(c.Request.Context(), f, pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, paginatedResponse(msgs, total, page, pageSize))
}
