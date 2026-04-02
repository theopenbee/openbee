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

// fileMediaMarker is the protocol prefix embedded in message content to carry a media path.
// It is shared between the write path (encodeMediaPaths) and the read path (decodeMediaPaths).
const fileMediaMarker = "[file]"

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
	if !validateSessionID(c, id) {
		return
	}
	sessionKey := localSessionKey(id)

	var body struct {
		Content    string   `json:"content" binding:"required"`
		MediaPaths []string `json:"media_paths"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for _, p := range body.MediaPaths {
		if strings.ContainsAny(p, "/\\") || p == ".." || p == "." {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid media_path"})
			return
		}
	}

	content := encodeMediaPaths(body.MediaPaths, body.Content)

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

// encodeMediaPaths prepends zero or more "[file] name\n" lines to text.
func encodeMediaPaths(paths []string, text string) string {
	if len(paths) == 0 {
		return text
	}
	var sb strings.Builder
	for _, p := range paths {
		sb.WriteString(fileMediaMarker)
		sb.WriteByte(' ')
		sb.WriteString(p)
		sb.WriteByte('\n')
	}
	sb.WriteString(text)
	return sb.String()
}

// decodeMediaPaths extracts leading "[file] name\n" lines from content.
// Returns the list of filenames and the remaining text.
func decodeMediaPaths(content string) ([]string, string) {
	prefix := fileMediaMarker + " "
	var paths []string
	for strings.HasPrefix(content, prefix) {
		rest := content[len(prefix):]
		idx := strings.IndexByte(rest, '\n')
		if idx < 0 {
			break
		}
		paths = append(paths, rest[:idx])
		content = rest[idx+1:]
	}
	return paths, content
}

type chatMessage struct {
	Role       string   `json:"role"`
	Content    string   `json:"content"`
	MediaPaths []string `json:"media_paths,omitempty"`
	Timestamp  int64    `json:"ts"`
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
		paths, text := decodeMediaPaths(m.Content)
		msg := chatMessage{Role: "user", Content: text, Timestamp: m.ReceivedAt}
		if len(paths) > 0 {
			msg.MediaPaths = paths
		}
		combined = append(combined, msg)
	}
	for _, r := range replies {
		combined = append(combined, chatMessage{Role: "bee", Content: r.Content, Timestamp: r.CreatedAt})
	}
	sort.Slice(combined, func(i, j int) bool { return combined[i].Timestamp < combined[j].Timestamp })

	c.JSON(http.StatusOK, combined)
}

func (h *LocalChatHandler) uploadMedia(c *gin.Context) {
	id := c.Param("id")
	if !validateSessionID(c, id) {
		return
	}

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

	filename := filepath.Base(header.Filename)
	destPath := filepath.Join(uploadDir, filename)
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

	c.JSON(http.StatusOK, gin.H{"path": filename})
}

func (h *LocalChatHandler) serveMedia(c *gin.Context) {
	id := c.Param("id")
	if !validateSessionID(c, id) {
		return
	}
	filename := filepath.Base(c.Param("filename"))
	if filename == "." || filename == ".." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	uploadDir, err := localUploadDir(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.File(filepath.Join(uploadDir, filename))
}

func validateSessionID(c *gin.Context, id string) bool {
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return false
	}
	return true
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
