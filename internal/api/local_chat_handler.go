package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
	"golang.org/x/sync/errgroup"
)

// fileMediaMarker is the protocol prefix embedded in message content to carry a media path.
// It is shared between the write path (encodeMediaPaths) and the read path (decodeMediaPaths).
// The leading \x00 (NUL byte) prevents collision with ordinary user text, which cannot contain NUL.
const fileMediaMarker = "\x00[file]"

// legacyFileMediaMarker is the old marker written before the NUL prefix was added.
// decodeMediaPaths still recognises it so existing stored messages are decoded correctly.
const legacyFileMediaMarker = "[file]"

const fileMediaPrefix = fileMediaMarker + " "
const legacyFileMediaPrefix = legacyFileMediaMarker + " "

const chatRoleUser = "user"
const chatRoleBee = "bee"

type LocalChatHandler struct {
	receiver      *local.LocalReceiver
	hub           *local.SSEHub
	outboundStore *store.OutboundMessageStore
	msgStore      *store.MessageStore
	workerStore   *store.WorkerStore
}

func NewLocalChatHandler(
	receiver *local.LocalReceiver,
	hub *local.SSEHub,
	outboundStore *store.OutboundMessageStore,
	msgStore *store.MessageStore,
	workerStore *store.WorkerStore,
) *LocalChatHandler {
	return &LocalChatHandler{
		receiver:      receiver,
		hub:           hub,
		outboundStore: outboundStore,
		msgStore:      msgStore,
		workerStore:   workerStore,
	}
}

// sessionKeyFor returns the per-user local chat session key. When workerID is
// non-empty the key scopes a 1:1 conversation between the user and that digital
// employee; otherwise it scopes the user's conversation with bee.
func sessionKeyFor(userID, workerID string) string {
	if workerID != "" {
		return "local:u:" + userID + ":w:" + workerID
	}
	return "local:u:" + userID
}

func (h *LocalChatHandler) StreamReplies(c *gin.Context) {
	uid := auth.UserID(c)
	workerID, ok := h.resolveWorkerID(c, c.Query("worker_id"))
	if !ok {
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ch, unsub := h.hub.Subscribe(sessionKeyFor(uid, workerID))
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

func (h *LocalChatHandler) SendMessage(c *gin.Context) {
	uid := auth.UserID(c)

	var body struct {
		Content    string   `json:"content" binding:"required"`
		MediaPaths []string `json:"media_paths"`
		WorkerID   string   `json:"worker_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	workerID, ok := h.resolveWorkerID(c, body.WorkerID)
	if !ok {
		return
	}

	for _, p := range body.MediaPaths {
		if strings.ContainsAny(p, "/\\") || p == ".." || p == "." {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename in media_paths"})
			return
		}
	}

	content := encodeMediaPaths(body.MediaPaths, body.Content)

	h.receiver.Enqueue(platform.InboundMessage{
		Platform:       local.PlatformID,
		SenderID:       uid,
		SessionKey:     sessionKeyFor(uid, workerID),
		Content:        content,
		RawContent:     content,
		MessageTime:    time.Now().UnixMilli(),
		TargetWorkerID: workerID,
	})

	c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}

// resolveWorkerID validates an optional worker id from the request. It returns
// ("", true) when no worker is targeted, (id, true) when the worker exists, and
// ("", false) after writing an error response when the worker id is unknown.
func (h *LocalChatHandler) resolveWorkerID(c *gin.Context, workerID string) (string, bool) {
	if workerID == "" {
		return "", true
	}
	if h.workerStore == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "direct worker chat unavailable"})
		return "", false
	}
	if _, err := h.workerStore.GetByID(workerID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "worker not found"})
		return "", false
	}
	return workerID, true
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

// decodeMediaPaths extracts leading "<marker> name\n" lines from content.
// It recognises the current marker (\x00[file]) and the legacy marker ([file])
// so messages stored before the NUL prefix was introduced are still decoded correctly.
// Returns the list of filenames and the remaining text.
func decodeMediaPaths(content string) ([]string, string) {
	var paths []string
	for {
		var rest string
		switch {
		case strings.HasPrefix(content, fileMediaPrefix):
			rest = content[len(fileMediaPrefix):]
		case strings.HasPrefix(content, legacyFileMediaPrefix):
			rest = content[len(legacyFileMediaPrefix):]
		default:
			return paths, content
		}
		filename, after, ok := strings.Cut(rest, "\n")
		if !ok {
			break
		}
		paths = append(paths, filename)
		content = after
	}
	return paths, content
}

type chatMessage struct {
	Role       string   `json:"role"`
	Content    string   `json:"content"`
	MediaPaths []string `json:"media_paths,omitempty"`
	Timestamp  int64    `json:"ts"`
}

func (h *LocalChatHandler) GetMessages(c *gin.Context) {
	ctx := c.Request.Context()

	uid := auth.UserID(c)
	workerID, ok := h.resolveWorkerID(c, c.Query("worker_id"))
	if !ok {
		return
	}
	sessionKey := sessionKeyFor(uid, workerID)

	before := int64(0)
	if v := c.Query("before"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			before = n
		}
	}
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	fetch := limit + 1

	var inbound []store.InboundMessage
	var replies []store.OutboundMessage
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		inbound, err = h.msgStore.ListBySessionKey(gCtx, sessionKey, before, fetch)
		return err
	})
	g.Go(func() error {
		var err error
		replies, err = h.outboundStore.ListBySessionKey(gCtx, sessionKey, before, fetch)
		return err
	})
	if err := g.Wait(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Detect has_more per-store before merging; len(combined) > limit is not reliable
	// because two stores returning limit items each would produce 2*limit combined.
	hasMore := len(inbound) > limit || len(replies) > limit
	if len(inbound) > limit {
		inbound = inbound[:limit]
	}
	if len(replies) > limit {
		replies = replies[:limit]
	}

	combined := make([]chatMessage, 0, len(inbound)+len(replies))
	for _, m := range inbound {
		paths, text := decodeMediaPaths(m.Content)
		msg := chatMessage{Role: chatRoleUser, Content: text, Timestamp: m.ReceivedAt}
		if len(paths) > 0 {
			msg.MediaPaths = paths
		}
		combined = append(combined, msg)
	}
	for _, r := range replies {
		combined = append(combined, chatMessage{Role: chatRoleBee, Content: r.Content, Timestamp: r.SentAt})
	}
	sort.Slice(combined, func(i, j int) bool { return combined[i].Timestamp < combined[j].Timestamp })

	if len(combined) > limit {
		combined = combined[len(combined)-limit:]
	}

	c.JSON(http.StatusOK, gin.H{"messages": combined, "has_more": hasMore})
}

func (h *LocalChatHandler) UploadMedia(c *gin.Context) {
	uid := auth.UserID(c)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing 'file' field"})
		return
	}
	defer file.Close()

	uploadDir, err := localUploadDir(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := uuid.New().String() + "_" + platform.SanitizeFileName(filepath.Base(header.Filename))
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

func (h *LocalChatHandler) ServeMedia(c *gin.Context) {
	uid := auth.UserID(c)

	filename := filepath.Base(c.Param("filename"))
	if filename == "." || filename == ".." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	uploadDir, err := localUploadDir(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.File(filepath.Join(uploadDir, filename))
}

// chatWorker is a digital employee exposed to local chat for 1:1 conversation.
type chatWorker struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// ListWorkers returns the digital employees a user can chat with directly.
func (h *LocalChatHandler) ListWorkers(c *gin.Context) {
	if h.workerStore == nil {
		c.JSON(http.StatusOK, gin.H{"workers": []chatWorker{}})
		return
	}
	workers, err := h.workerStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]chatWorker, 0, len(workers))
	for _, w := range workers {
		out = append(out, chatWorker{
			ID:          w.ID,
			Name:        w.Name,
			Description: w.Description,
			Status:      string(w.Status),
		})
	}
	c.JSON(http.StatusOK, gin.H{"workers": out})
}

func localUploadDir(userID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".openbee", "local-uploads", platform.SanitizeFileName(userID)), nil
}
