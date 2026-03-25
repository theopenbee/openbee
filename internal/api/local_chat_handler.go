package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"github.com/theopenbee/openbee/internal/logger"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
	"github.com/theopenbee/openbee/internal/store"
)

var log = logger.With(zap.String("component", "api"))

type LocalChatHandler struct {
	receiver     *local.LocalReceiver
	hub          *local.SSEHub
	sessionStore *store.LocalSessionStore
	replyStore   *store.LocalReplyStore
	msgStore     *store.MessageStore
	sessionCtx   *store.SessionStore
}

func NewLocalChatHandler(
	receiver *local.LocalReceiver,
	hub *local.SSEHub,
	sessionStore *store.LocalSessionStore,
	replyStore *store.LocalReplyStore,
	msgStore *store.MessageStore,
	sessionCtx *store.SessionStore,
) *LocalChatHandler {
	return &LocalChatHandler{
		receiver:     receiver,
		hub:          hub,
		sessionStore: sessionStore,
		replyStore:   replyStore,
		msgStore:     msgStore,
		sessionCtx:   sessionCtx,
	}
}

func (h *LocalChatHandler) StreamReplies(c *gin.Context) {
	sessionID := c.Param("id")
	sessionKey := localSessionKey(sessionID)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ch, unsub := h.hub.Subscribe(sessionKey)
	defer unsub()

	ctx := c.Request.Context()
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func (h *LocalChatHandler) createSession(c *gin.Context) {
	var body struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id := uuid.New().String()
	if err := h.sessionStore.Create(c.Request.Context(), id, body.Name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	now := time.Now().UnixMilli()
	c.JSON(http.StatusCreated, store.LocalSession{
		ID:        id,
		Name:      body.Name,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (h *LocalChatHandler) listSessions(c *gin.Context) {
	sessions, err := h.sessionStore.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sessions)
}

func (h *LocalChatHandler) deleteSession(c *gin.Context) {
	id := c.Param("id")
	sessionKey := localSessionKey(id)
	ctx := c.Request.Context()

	// Best-effort cascade: log failures but continue so the session row is always removed.
	if err := h.msgStore.DeleteBySessionKey(ctx, sessionKey); err != nil {
		log.Error("deleteSession: delete messages", zap.String("sessionKey", sessionKey), zap.Error(err))
	}
	if err := h.replyStore.DeleteBySession(ctx, sessionKey); err != nil {
		log.Error("deleteSession: delete replies", zap.String("sessionKey", sessionKey), zap.Error(err))
	}
	if err := h.sessionCtx.ClearSessionContexts(ctx, sessionKey); err != nil {
		log.Error("deleteSession: clear session contexts", zap.String("sessionKey", sessionKey), zap.Error(err))
	}
	if err := h.sessionStore.Delete(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *LocalChatHandler) sendMessage(c *gin.Context) {
	id := c.Param("id")
	sessionKey := localSessionKey(id)

	var body struct {
		Content   string `json:"content" binding:"required"`
		MediaPath string `json:"media_path"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	content := body.Content
	if body.MediaPath != "" {
		uploadDir, err := localUploadDir(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		rel, err := filepath.Rel(uploadDir, body.MediaPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid media_path: must be within upload directory"})
			return
		}
		content = "[文件] " + body.MediaPath + "\n" + content
	}

	h.receiver.Enqueue(platform.InboundMessage{
		Platform:    "local",
		SenderID:    "web",
		SessionKey:  sessionKey,
		Content:     content,
		RawContent:  content,
		MessageTime: time.Now().UnixMilli(),
	})

	h.sessionStore.TouchUpdatedAt(c.Request.Context(), id) //nolint:errcheck — best-effort UI timestamp update

	c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}

type chatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"ts"`
}

func (h *LocalChatHandler) getMessages(c *gin.Context) {
	id := c.Param("id")
	sessionKey := localSessionKey(id)
	ctx := c.Request.Context()

	inbound, err := h.msgStore.ListBySessionKey(ctx, sessionKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	replies, err := h.replyStore.ListBySession(ctx, sessionKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	combined := make([]chatMessage, 0, len(inbound)+len(replies))
	for _, m := range inbound {
		combined = append(combined, chatMessage{Role: "user", Content: m.Content, Timestamp: m.ReceivedAt})
	}
	for _, r := range replies {
		combined = append(combined, chatMessage{Role: "bee", Content: r.Content, Timestamp: r.CreatedAt})
	}
	sort.Slice(combined, func(i, j int) bool { return combined[i].Timestamp < combined[j].Timestamp })

	c.JSON(http.StatusOK, combined)
}

func (h *LocalChatHandler) uploadMedia(c *gin.Context) {
	id := c.Param("id")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing 'file' field"})
		return
	}
	defer file.Close()

	uploadDir, err := localUploadDir(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	destPath := filepath.Join(uploadDir, filepath.Base(header.Filename))
	dest, err := os.Create(destPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer dest.Close()

	if _, err := io.Copy(dest, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"path": destPath})
}

func localSessionKey(id string) string {
	return "local:" + id
}

func localUploadDir(sessionID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".openbee", "local-uploads", sessionID), nil
}
