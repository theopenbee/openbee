package api

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/domain/session"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const localMessageIDPrefix = "local:"

const (
	conversationKindBee    = "bee"
	conversationKindWorker = "worker"
)

const previewMaxRunes = 120

const sseHeartbeatInterval = 25 * time.Second

var messageIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type LocalChatWorkers interface {
	List() ([]model.Worker, error)
	GetByID(id string) (model.Worker, error)
}

type LocalChatClearer interface {
	EvaluateClearSession(ctx context.Context, sessionKey string) (session.ClearSessionPreview, error)
	ClearSession(ctx context.Context, sessionKey string, preview session.ClearSessionPreview) (session.ClearSessionResult, error)
	EvaluateClearWorker(ctx context.Context, sessionKey string, w model.Worker) (session.ClearWorkerPreview, error)
	ClearWorker(ctx context.Context, sessionKey string, w model.Worker, preview session.ClearWorkerPreview) (session.ClearWorkerResult, error)
	StopConversation(ctx context.Context, sessionKey string, w model.Worker) (session.StopWorkerResult, error)
}

type LocalChatDeps struct {
	Receiver  *local.LocalReceiver
	Hub       *local.SSEHub
	Publisher *local.EventPublisher
	Messages  *store.MessageStore
	Outbound  *store.OutboundMessageStore
	Chats     *store.LocalChatStore
	Workers   LocalChatWorkers
	Clear     LocalChatClearer
	MediaRoot string
	Heartbeat time.Duration
}

type LocalChatHandler struct {
	receiver      *local.LocalReceiver
	hub           *local.SSEHub
	publisher     *local.EventPublisher
	outboundStore *store.OutboundMessageStore
	msgStore      *store.MessageStore
	chats         *store.LocalChatStore
	workers       LocalChatWorkers
	clear         LocalChatClearer
	mediaRoot     string
	heartbeat     time.Duration
}

func NewLocalChatHandler(d LocalChatDeps) *LocalChatHandler {
	return &LocalChatHandler{
		receiver:      d.Receiver,
		hub:           d.Hub,
		publisher:     d.Publisher,
		outboundStore: d.Outbound,
		msgStore:      d.Messages,
		chats:         d.Chats,
		workers:       d.Workers,
		clear:         d.Clear,
		mediaRoot:     d.MediaRoot,
		heartbeat:     cmp.Or(d.Heartbeat, sseHeartbeatInterval),
	}
}

type ConversationPreview struct {
	Sender          local.Sender `json:"sender"`
	Kind            string       `json:"kind"`
	Preview         string       `json:"preview"`
	HandoffWorkerID string       `json:"handoff_worker_id,omitempty"`
	TS              int64        `json:"ts"`
}

type ConversationSummary struct {
	ID          string               `json:"id"`
	Kind        string               `json:"kind"`
	WorkerID    string               `json:"worker_id,omitempty"`
	LastMessage *ConversationPreview `json:"last_message,omitempty"`
	Unread      int                  `json:"unread"`
	Activity    local.Activity       `json:"activity"`
}

type conversation struct {
	id         string
	sessionKey string
	worker     *model.Worker
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

	ch, unsub := h.hub.Subscribe(uid)
	defer unsub()

	fmt.Fprint(c.Writer, ": ok\n\n")
	c.Writer.Flush()

	heartbeat := time.NewTicker(h.heartbeat)
	defer heartbeat.Stop()

	ctx := c.Request.Context()
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
		case <-heartbeat.C:
			fmt.Fprint(c.Writer, ": ping\n\n")
			c.Writer.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func (h *LocalChatHandler) resolveConversation(c *gin.Context) (string, conversation, bool) {
	uid, ok := currentUserID(c)
	if !ok {
		return "", conversation{}, false
	}
	cid := c.Param("cid")
	sessionKey, err := local.ConversationSessionKey(uid, cid)
	if err != nil {
		respondConversationNotFound(c)
		return "", conversation{}, false
	}
	conv := conversation{id: cid, sessionKey: sessionKey}
	if workerID, isWorker := local.WorkerIDFromConversation(cid); isWorker {
		w, err := h.workers.GetByID(workerID)
		if errors.Is(err, sql.ErrNoRows) {
			respondConversationNotFound(c)
			return "", conversation{}, false
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return "", conversation{}, false
		}
		conv.worker = &w
	}
	return uid, conv, true
}

func respondConversationNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
}

func bindOptionalJSON(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func chatSender(senderType, senderID string) local.Sender {
	switch senderType {
	case store.SenderTypeUser:
		return local.Sender{Type: store.SenderTypeUser}
	case store.SourceTypeWorker:
		return local.Sender{Type: store.SourceTypeWorker, ID: senderID}
	case store.SourceTypeSystem:
		return local.Sender{Type: store.SourceTypeSystem}
	default:
		return local.Sender{Type: local.SenderBee}
	}
}

func conversationPreview(content string) string {
	_, text := local.DecodeMediaPaths(content)
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= previewMaxRunes {
		return text
	}
	return string([]rune(text)[:previewMaxRunes])
}

func handoffWorkerID(kind, payload string) string {
	if kind != local.KindHandoff {
		return ""
	}
	var info local.HandoffInfo
	if err := json.Unmarshal([]byte(payload), &info); err != nil {
		return ""
	}
	return info.WorkerID
}

func (h *LocalChatHandler) summarizeConversation(ctx context.Context, uid, workerID string, readMarks map[string]int64) (ConversationSummary, error) {
	summary := ConversationSummary{ID: local.BeeConversationID, Kind: conversationKindBee}
	sessionKey := local.SessionKey(uid)
	if workerID != "" {
		summary = ConversationSummary{ID: local.WorkerConversationID(workerID), Kind: conversationKindWorker, WorkerID: workerID}
		sessionKey = local.WorkerSessionKey(uid, workerID)
	}
	last, found, err := h.chats.LastMessage(ctx, sessionKey)
	if err != nil {
		return ConversationSummary{}, err
	}
	if found {
		summary.LastMessage = &ConversationPreview{
			Sender:          chatSender(last.SenderType, last.SenderID),
			Kind:            last.Kind,
			Preview:         conversationPreview(last.Content),
			HandoffWorkerID: handoffWorkerID(last.Kind, last.Payload),
			TS:              last.TS,
		}
	}
	unread, err := h.chats.UnreadCount(ctx, sessionKey, readMarks[summary.ID])
	if err != nil {
		return ConversationSummary{}, err
	}
	summary.Unread = unread
	summary.Activity = h.publisher.Activity(sessionKey)
	return summary, nil
}

type namedConversation struct {
	summary ConversationSummary
	name    string
}

func compareConversations(a, b namedConversation) int {
	switch {
	case a.summary.LastMessage != nil && b.summary.LastMessage != nil:
		if c := cmp.Compare(b.summary.LastMessage.TS, a.summary.LastMessage.TS); c != 0 {
			return c
		}
	case a.summary.LastMessage != nil:
		return -1
	case b.summary.LastMessage != nil:
		return 1
	}
	return cmp.Or(strings.Compare(a.name, b.name), strings.Compare(a.summary.ID, b.summary.ID))
}

func (h *LocalChatHandler) ListConversations(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	workers, err := h.workers.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	readMarks, err := h.chats.ReadMarks(ctx, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	bee, err := h.summarizeConversation(ctx, uid, "", readMarks)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	entries := make([]namedConversation, 0, len(workers))
	for _, w := range workers {
		summary, err := h.summarizeConversation(ctx, uid, w.ID, readMarks)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		entries = append(entries, namedConversation{summary: summary, name: w.Name})
	}
	slices.SortStableFunc(entries, compareConversations)

	conversations := make([]ConversationSummary, 0, len(entries)+1)
	conversations = append(conversations, bee)
	for _, e := range entries {
		conversations = append(conversations, e.summary)
	}
	c.JSON(http.StatusOK, conversations)
}

func (h *LocalChatHandler) SendMessage(c *gin.Context) {
	uid, conv, ok := h.resolveConversation(c)
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

	content := local.EncodeMediaPaths(body.MediaPaths, body.Content)
	ts := time.Now().UnixMilli()

	h.receiver.Enqueue(platform.InboundMessage{
		Platform:          local.PlatformID,
		SenderID:          uid,
		SessionKey:        conv.sessionKey,
		Content:           content,
		RawContent:        content,
		PlatformMessageID: localMessageID(uid, id),
		MessageTime:       ts,
	})

	c.JSON(http.StatusAccepted, gin.H{"id": id, "ts": ts})
}

func (h *LocalChatHandler) GetMessages(c *gin.Context) {
	uid, conv, ok := h.resolveConversation(c)
	if !ok {
		return
	}
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
		inbound, err = h.msgStore.ListBySessionKey(gCtx, conv.sessionKey, before, fetch)
		return err
	})
	g.Go(func() error {
		var err error
		replies, err = h.outboundStore.ListBySessionKey(gCtx, conv.sessionKey, before, fetch)
		return err
	})
	if err := g.Wait(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	mediaDir, _ := local.MediaDir(h.mediaRoot, conv.sessionKey)
	combined := make([]local.ChatMessage, 0, len(inbound)+len(replies))
	for _, m := range inbound {
		combined = append(combined, inboundChatMessage(m, uid))
	}
	for _, r := range replies {
		combined = append(combined, outboundChatMessage(r, mediaDir))
	}
	sort.SliceStable(combined, func(i, j int) bool { return combined[i].TS < combined[j].TS })

	hasMore := len(combined) > limit
	if hasMore {
		combined = combined[len(combined)-limit:]
	}

	c.JSON(http.StatusOK, gin.H{"messages": combined, "has_more": hasMore})
}

func inboundChatMessage(m store.InboundMessage, userID string) local.ChatMessage {
	paths, text := local.DecodeMediaPaths(m.Content)
	msg := local.ChatMessage{
		ID:      inboundChatID(m, userID),
		Sender:  chatSender(m.SenderType, ""),
		Kind:    local.KindMessage,
		Content: text,
		TS:      m.ReceivedAt,
	}
	if len(paths) > 0 {
		msg.MediaPaths = paths
	}
	return msg
}

func outboundChatMessage(r store.OutboundMessage, mediaDir string) local.ChatMessage {
	msg := local.ChatMessage{
		ID:         r.ID,
		Sender:     chatSender(cmp.Or(r.SourceType, store.SourceTypeBee), r.SourceID),
		Kind:       cmp.Or(r.Kind, local.KindMessage),
		Content:    r.Content,
		MediaPaths: replyMedia(mediaDir, r),
		TS:         r.SentAt,
	}
	if msg.Kind == local.KindHandoff {
		var info local.HandoffInfo
		if err := json.Unmarshal([]byte(r.Payload), &info); err == nil {
			msg.Handoff = &info
		}
	}
	return msg
}

func (h *LocalChatHandler) MarkRead(c *gin.Context) {
	uid, conv, ok := h.resolveConversation(c)
	if !ok {
		return
	}
	var body struct {
		TS int64 `json:"ts"`
	}
	if err := bindOptionalJSON(c, &body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ts := body.TS
	if ts <= 0 {
		ts = time.Now().UnixMilli()
	}
	if err := h.chats.MarkRead(c.Request.Context(), uid, conv.id, ts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

type activeTaskResponse struct {
	ID          string `json:"id"`
	Instruction string `json:"instruction"`
	Type        string `json:"type"`
}

func (h *LocalChatHandler) Clear(c *gin.Context) {
	_, conv, ok := h.resolveConversation(c)
	if !ok {
		return
	}
	var body struct {
		Force bool `json:"force"`
	}
	if err := bindOptionalJSON(c, &body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	blocking, err := h.clearConversation(c.Request.Context(), conv, body.Force)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(blocking) > 0 {
		active := make([]activeTaskResponse, 0, len(blocking))
		for _, t := range blocking {
			active = append(active, activeTaskResponse{ID: t.ID, Instruction: t.Instruction, Type: t.Type})
		}
		c.JSON(http.StatusConflict, gin.H{"error": "active tasks", "active_tasks": active})
		return
	}
	h.appendReset(c.Request.Context(), conv)
	c.Status(http.StatusNoContent)
}

func (h *LocalChatHandler) appendReset(ctx context.Context, conv conversation) {
	kind, found, err := h.chats.LastKind(ctx, conv.sessionKey)
	if err != nil {
		logger.Error("read last local chat kind", zap.String("session_key", conv.sessionKey), zap.Error(err))
		return
	}
	if !found || kind == store.OutboundKindReset {
		return
	}
	divider := store.OutboundMessage{
		ID:         uuid.New().String(),
		SessionKey: conv.sessionKey,
		Platform:   local.PlatformID,
		Status:     store.OutboundStatusSent,
		SourceType: store.SourceTypeSystem,
		Kind:       store.OutboundKindReset,
		SentAt:     time.Now().UnixMilli(),
	}
	if err := h.outboundStore.Create(ctx, divider); err != nil {
		logger.Error("store local chat reset", zap.String("session_key", conv.sessionKey), zap.Error(err))
		return
	}
	h.publisher.Message(conv.sessionKey, outboundChatMessage(divider, ""))
}

func (h *LocalChatHandler) clearConversation(ctx context.Context, conv conversation, force bool) ([]model.Task, error) {
	if conv.worker == nil {
		preview, err := h.clear.EvaluateClearSession(ctx, conv.sessionKey)
		if err != nil {
			return nil, err
		}
		if len(preview.ActiveTasks) > 0 && !force {
			return preview.ActiveTasks, nil
		}
		_, err = h.clear.ClearSession(ctx, conv.sessionKey, preview)
		return nil, err
	}
	preview, err := h.clear.EvaluateClearWorker(ctx, conv.sessionKey, *conv.worker)
	if err != nil {
		return nil, err
	}
	if len(preview.ActiveTasks) > 0 && !force {
		return preview.ActiveTasks, nil
	}
	_, err = h.clear.ClearWorker(ctx, conv.sessionKey, *conv.worker, preview)
	return nil, err
}

func (h *LocalChatHandler) Stop(c *gin.Context) {
	_, conv, ok := h.resolveConversation(c)
	if !ok {
		return
	}
	if conv.worker == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only worker conversations can be stopped"})
		return
	}
	if _, err := h.clear.StopConversation(c.Request.Context(), conv.sessionKey, *conv.worker); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func replyMedia(mediaDir string, r store.OutboundMessage) []string {
	if mediaDir == "" || r.MediaPath == "" || r.Status != store.OutboundStatusSent {
		return nil
	}
	name := local.MediaFileName(r.ID, r.MediaPath)
	if info, err := os.Stat(filepath.Join(mediaDir, name)); err != nil || !info.Mode().IsRegular() {
		return nil
	}
	return []string{name}
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
	for _, prefix := range []string{"image/", "video/", "audio/"} {
		if strings.HasPrefix(mediaType, prefix) {
			return true
		}
	}
	return mediaType == "text/plain"
}
