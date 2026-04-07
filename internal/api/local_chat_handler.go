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
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
	"github.com/theopenbee/openbee/internal/infra/store"
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

const defaultSessionKey = "local:default"

type LocalChatHandler struct {
	receiver      *local.LocalReceiver
	hub           *local.SSEHub
	outboundStore *store.OutboundMessageStore
	msgStore      *store.MessageStore
}

func NewLocalChatHandler(
	receiver *local.LocalReceiver,
	hub *local.SSEHub,
	outboundStore *store.OutboundMessageStore,
	msgStore *store.MessageStore,
) *LocalChatHandler {
	return &LocalChatHandler{
		receiver:      receiver,
		hub:           hub,
		outboundStore: outboundStore,
		msgStore:      msgStore,
	}
}

func (h *LocalChatHandler) StreamReplies(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ch, unsub := h.hub.Subscribe(defaultSessionKey)
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

func (h *LocalChatHandler) sendMessage(c *gin.Context) {
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
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename in media_paths"})
			return
		}
	}

	content := encodeMediaPaths(body.MediaPaths, body.Content)

	h.receiver.Enqueue(platform.InboundMessage{
		Platform:    local.PlatformID,
		SenderID:    "web",
		SessionKey:  defaultSessionKey,
		Content:     content,
		RawContent:  content,
		MessageTime: time.Now().UnixMilli(),
	})

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

func (h *LocalChatHandler) getMessages(c *gin.Context) {
	ctx := c.Request.Context()

	inbound, err := h.msgStore.ListBySessionKey(ctx, defaultSessionKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	replies, err := h.outboundStore.ListBySessionKey(ctx, defaultSessionKey, 0)
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
		combined = append(combined, chatMessage{Role: "bee", Content: r.Content, Timestamp: r.SentAt})
	}
	sort.Slice(combined, func(i, j int) bool { return combined[i].Timestamp < combined[j].Timestamp })

	c.JSON(http.StatusOK, combined)
}

func (h *LocalChatHandler) uploadMedia(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing 'file' field"})
		return
	}
	defer file.Close()

	uploadDir, err := localUploadDir()
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

func (h *LocalChatHandler) serveMedia(c *gin.Context) {
	filename := filepath.Base(c.Param("filename"))
	if filename == "." || filename == ".." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	uploadDir, err := localUploadDir()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.File(filepath.Join(uploadDir, filename))
}

func localUploadDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".openbee", "local-uploads", "default"), nil
}
