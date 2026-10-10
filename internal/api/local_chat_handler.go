package api

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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

const localMessageIDPrefix = "local:"
const chatRoleUser = "user"
const chatRoleBee = "bee"

var messageIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type LocalChatHandler struct {
	receiver      *local.LocalReceiver
	hub           *local.SSEHub
	outboundStore *store.OutboundMessageStore
	msgStore      *store.MessageStore
	mediaRoot     string
}

func NewLocalChatHandler(
	receiver *local.LocalReceiver,
	hub *local.SSEHub,
	outboundStore *store.OutboundMessageStore,
	msgStore *store.MessageStore,
	mediaRoot string,
) *LocalChatHandler {
	return &LocalChatHandler{
		receiver:      receiver,
		hub:           hub,
		outboundStore: outboundStore,
		msgStore:      msgStore,
		mediaRoot:     mediaRoot,
	}
}

func currentUserID(c *gin.Context) (string, bool) {
	uid := auth.UserID(c)
	if uid == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return "", false
	}
	return uid, true
}

func (h *LocalChatHandler) currentMediaDir(c *gin.Context) (string, bool) {
	uid, ok := currentUserID(c)
	if !ok {
		return "", false
	}
	dir, err := local.MediaDir(h.mediaRoot, local.SessionKey(uid))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return "", false
	}
	return dir, true
}

func localMessageID(userID, clientID string) string {
	return localMessageIDPrefix + userID + ":" + clientID
}

func (h *LocalChatHandler) StreamReplies(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ch, unsub := h.hub.Subscribe(local.SessionKey(uid))
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
	uid, ok := currentUserID(c)
	if !ok {
		return
	}

	var body struct {
		ID         string   `json:"id"`
		Content    string   `json:"content" binding:"required"`
		MediaPaths []string `json:"media_paths"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id := body.ID
	if id == "" {
		id = uuid.New().String()
	} else if !messageIDPattern.MatchString(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
		return
	}

	for _, p := range body.MediaPaths {
		if strings.ContainsAny(p, "/\\") || p == ".." || p == "." {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename in media_paths"})
			return
		}
	}

	content := encodeMediaPaths(body.MediaPaths, body.Content)
	ts := time.Now().UnixMilli()

	h.receiver.Enqueue(platform.InboundMessage{
		Platform:          local.PlatformID,
		SenderID:          uid,
		SessionKey:        local.SessionKey(uid),
		Content:           content,
		RawContent:        content,
		PlatformMessageID: localMessageID(uid, id),
		MessageTime:       ts,
	})

	c.JSON(http.StatusAccepted, gin.H{"status": "queued", "id": id, "ts": ts})
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
	ID         string   `json:"id"`
	Role       string   `json:"role"`
	Content    string   `json:"content"`
	MediaPaths []string `json:"media_paths,omitempty"`
	Timestamp  int64    `json:"ts"`
}

func (h *LocalChatHandler) GetMessages(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		return
	}
	sessionKey := local.SessionKey(uid)
	ctx := c.Request.Context()

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

	combined := make([]chatMessage, 0, len(inbound)+len(replies))
	for _, m := range inbound {
		paths, text := decodeMediaPaths(m.Content)
		msg := chatMessage{ID: inboundChatID(m, uid), Role: chatRoleUser, Content: text, Timestamp: m.ReceivedAt}
		if len(paths) > 0 {
			msg.MediaPaths = paths
		}
		combined = append(combined, msg)
	}
	for _, r := range replies {
		msg := chatMessage{ID: r.ID, Role: chatRoleBee, Content: r.Content, Timestamp: r.SentAt}
		if h.isStagedMedia(r.MediaPath) {
			msg.MediaPaths = []string{filepath.Base(r.MediaPath)}
		}
		combined = append(combined, msg)
	}
	sort.SliceStable(combined, func(i, j int) bool { return combined[i].Timestamp < combined[j].Timestamp })

	hasMore := len(combined) > limit
	if hasMore {
		combined = combined[len(combined)-limit:]
	}

	c.JSON(http.StatusOK, gin.H{"messages": combined, "has_more": hasMore})
}

func (h *LocalChatHandler) isStagedMedia(path string) bool {
	return path != "" && filepath.Dir(filepath.Dir(path)) == filepath.Clean(h.mediaRoot)
}

func inboundChatID(m store.InboundMessage, userID string) string {
	if id, ok := strings.CutPrefix(m.PlatformMsgID, localMessageID(userID, "")); ok {
		return id
	}
	if id, ok := strings.CutPrefix(m.PlatformMsgID, localMessageIDPrefix); ok {
		return id
	}
	return m.ID
}

func (h *LocalChatHandler) UploadMedia(c *gin.Context) {
	uploadDir, ok := h.currentMediaDir(c)
	if !ok {
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing 'file' field"})
		return
	}
	defer file.Close()

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
	mediaDir, ok := h.currentMediaDir(c)
	if !ok {
		return
	}

	filename := filepath.Base(c.Param("filename"))
	if filename == "." || filename == ".." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	path := filepath.Join(mediaDir, filename)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox")
	if isInlineMedia(filename) {
		c.File(path)
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.FileAttachment(path, filename)
}

func isInlineMedia(filename string) bool {
	mediaType, _, _ := mime.ParseMediaType(mime.TypeByExtension(filepath.Ext(filename)))
	if mediaType == "image/svg+xml" {
		return false
	}
	for _, prefix := range []string{"image/", "video/", "audio/"} {
		if strings.HasPrefix(mediaType, prefix) {
			return true
		}
	}
	return mediaType == "text/plain"
}
