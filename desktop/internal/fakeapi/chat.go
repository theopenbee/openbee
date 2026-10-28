package fakeapi

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const (
	defaultPageSize   = 50
	maxPageSize       = 100
	previewMaxRunes   = 120
	multipartMemory   = 32 << 20
	kindMessage       = "message"
	kindReset         = "reset"
	senderUser        = "user"
	senderSystem      = "system"
	senderWorker      = "worker"
	senderBee         = "bee"
	conversationBee   = "bee"
	activityIdle      = "idle"
	legacyMediaMarker = "\x00[file] "
	mediaMarker       = "[file] "
)

var (
	messageIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	unsafeNameChars  = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
)

func (s *Server) chatRoutes() {
	s.route("GET /api/local/conversations", authUser, s.listConversations)
	s.route("GET /api/local/conversations/{cid}/messages", authUser, s.listMessages)
	s.route("POST /api/local/conversations/{cid}/messages", authUser, s.sendMessage)
	s.route("POST /api/local/conversations/{cid}/read", authUser, s.markRead)
	s.route("POST /api/local/conversations/{cid}/clear", authUser, s.clearConversation)
	s.route("POST /api/local/conversations/{cid}/stop", authUser, s.stopConversation)
	s.route("POST /api/local/media", authUser, s.uploadMedia)
	s.raw("GET /api/local/media/{filename}", authUser, s.serveMedia)
	s.raw("GET "+apiChatStream, authUser, s.chatStream)
}

const apiChatStream = "/api" + api.ChatStreamPath

func newSummary(cid string) api.ConversationSummary {
	sum := api.ConversationSummary{ID: cid, Kind: conversationBee, Activity: api.ConversationActivity{State: activityIdle}}
	if id, ok := api.ConversationWorkerID(cid); ok {
		sum.Kind = senderWorker
		sum.WorkerID = id
	}
	return sum
}

func (s *Server) summaryIndex(cid string) int {
	return slices.IndexFunc(s.st.Conversations, func(c api.ConversationSummary) bool { return c.ID == cid })
}

func (s *Server) ensureSummary(cid string) int {
	if i := s.summaryIndex(cid); i >= 0 {
		return i
	}
	s.st.Conversations = append(s.st.Conversations, newSummary(cid))
	return len(s.st.Conversations) - 1
}

func (s *Server) conversationExists(cid string) bool {
	if cid == api.BeeConversation {
		return true
	}
	id, ok := api.ConversationWorkerID(cid)
	return ok && s.workerIndex(id) >= 0
}

func conversationNotFound() reply {
	return errReply(http.StatusNotFound, "conversation not found")
}

func (s *Server) workerName(sum api.ConversationSummary) string {
	if i := s.workerIndex(sum.WorkerID); i >= 0 {
		return s.st.Workers[i].Name
	}
	return ""
}

func (s *Server) listConversations(*request) reply {
	s.ensureSummary(api.BeeConversation)
	for _, w := range s.st.Workers {
		s.ensureSummary(api.WorkerConversation(w.ID))
	}
	out := slices.Clone(s.st.Conversations)
	slices.SortStableFunc(out, func(a, b api.ConversationSummary) int {
		return cmp.Or(
			cmp.Compare(summaryRank(a), summaryRank(b)),
			summaryTimeOrder(a, b),
			cmp.Compare(s.workerName(a), s.workerName(b)),
			cmp.Compare(a.ID, b.ID),
		)
	})
	return ok(out)
}

func summaryRank(c api.ConversationSummary) int {
	switch {
	case c.ID == api.BeeConversation:
		return 0
	case c.LastMessage != nil:
		return 1
	}
	return 2
}

func summaryTimeOrder(a, b api.ConversationSummary) int {
	if a.LastMessage == nil || b.LastMessage == nil {
		return 0
	}
	return cmp.Compare(b.LastMessage.TS, a.LastMessage.TS)
}

func (s *Server) listMessages(q *request) reply {
	cid := q.param("cid")
	if !s.conversationExists(cid) {
		return conversationNotFound()
	}
	limit := defaultPageSize
	if v, err := strconv.Atoi(q.query("limit")); err == nil && v >= 1 && v <= maxPageSize {
		limit = v
	}
	before, _ := strconv.ParseInt(q.query("before"), 10, 64)
	msgs := slices.Clone(s.st.Messages[cid])
	slices.SortStableFunc(msgs, func(a, b api.ChatMessage) int { return cmp.Compare(a.TS, b.TS) })
	if before > 0 {
		msgs = slices.DeleteFunc(msgs, func(m api.ChatMessage) bool { return m.TS >= before })
	}
	hasMore := len(msgs) > limit
	if hasMore {
		msgs = msgs[len(msgs)-limit:]
	}
	if msgs == nil {
		msgs = []api.ChatMessage{}
	}
	return ok(api.MessagesPage{Messages: msgs, HasMore: hasMore})
}

func (s *Server) sendMessage(q *request) reply {
	cid := q.param("cid")
	if !s.conversationExists(cid) {
		return conversationNotFound()
	}
	var req api.SendMessageRequest
	if bad, good := q.decode(&req); !good {
		return bad
	}
	if req.Content == "" {
		return errReply(http.StatusBadRequest, validatorMessage("sendMessageRequest", "Content", "required"))
	}
	id := req.ID
	if id == "" {
		id = s.nextID("msg")
	} else if !messageIDPattern.MatchString(id) {
		return errReply(http.StatusBadRequest, "invalid message id")
	}
	for _, p := range req.MediaPaths {
		if strings.ContainsAny(p, `/\`) || p == "." || p == ".." {
			return errReply(http.StatusBadRequest, "invalid filename in media_paths")
		}
	}
	for _, m := range s.st.Messages[cid] {
		if m.ID == id {
			return accepted(api.SendMessageResponse{ID: id, TS: m.TS})
		}
	}
	msg := api.ChatMessage{
		ID:         id,
		Sender:     api.ChatSender{Type: senderUser},
		Kind:       kindMessage,
		Content:    req.Content,
		MediaPaths: slices.Clone(req.MediaPaths),
		TS:         s.now(),
	}
	s.st.Messages[cid] = append(s.st.Messages[cid], msg)
	s.noteMessage(cid, msg)
	return accepted(api.SendMessageResponse{ID: id, TS: msg.TS})
}

func (s *Server) markRead(q *request) reply {
	cid := q.param("cid")
	if !s.conversationExists(cid) {
		return conversationNotFound()
	}
	var req struct {
		TS int64 `json:"ts"`
	}
	if bad, good := q.decodeOptional(&req); !good {
		return bad
	}
	i := s.ensureSummary(cid)
	if req.TS <= 0 {
		s.st.Conversations[i].Unread = 0
		return noContent()
	}
	unread := 0
	for _, m := range s.st.Messages[cid] {
		if m.TS > req.TS && countsAsUnread(m) {
			unread++
		}
	}
	s.st.Conversations[i].Unread = unread
	return noContent()
}

func countsAsUnread(m api.ChatMessage) bool {
	return m.Kind != kindReset && m.Sender.Type != senderUser
}

func (s *Server) clearConversation(q *request) reply {
	cid := q.param("cid")
	if !s.conversationExists(cid) {
		return conversationNotFound()
	}
	var req struct {
		Force bool `json:"force"`
	}
	if bad, good := q.decodeOptional(&req); !good {
		return bad
	}
	if tasks := s.st.ActiveTasks[cid]; len(tasks) > 0 {
		if !req.Force {
			return reply{status: http.StatusConflict, body: map[string]any{
				"error":        "active tasks",
				"active_tasks": slices.Clone(tasks),
			}}
		}
		delete(s.st.ActiveTasks, cid)
		if i := s.summaryIndex(cid); i >= 0 {
			s.st.Conversations[i].Activity = api.ConversationActivity{State: activityIdle}
		}
	}
	msgs := s.st.Messages[cid]
	if len(msgs) > 0 && msgs[len(msgs)-1].Kind != kindReset {
		s.appendMessage(cid, api.ChatMessage{
			ID:     s.nextID("msg"),
			Sender: api.ChatSender{Type: senderSystem},
			Kind:   kindReset,
			TS:     s.now(),
		})
	}
	return noContent()
}

func (s *Server) stopConversation(q *request) reply {
	cid := q.param("cid")
	if !s.conversationExists(cid) {
		return conversationNotFound()
	}
	if cid == api.BeeConversation {
		return errReply(http.StatusBadRequest, "only worker conversations can be stopped")
	}
	return noContent()
}

func defaultSender(cid string) api.ChatSender {
	if id, ok := api.ConversationWorkerID(cid); ok {
		return api.ChatSender{Type: senderWorker, ID: id}
	}
	return api.ChatSender{Type: senderBee}
}

func (s *Server) Reply(cid string, m api.ChatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.Kind == "" {
		m.Kind = kindMessage
	}
	if m.ID == "" {
		m.ID = s.nextID("msg")
	}
	if m.TS == 0 {
		m.TS = s.now()
	}
	if m.Sender.Type == "" {
		m.Sender = defaultSender(cid)
	}
	s.appendMessage(cid, m)
}

func (s *Server) appendMessage(cid string, m api.ChatMessage) {
	s.st.Messages[cid] = append(s.st.Messages[cid], m)
	s.noteMessage(cid, m)
	msg := m
	s.emitLocked(api.ChatEvent{Type: "message", ConversationID: cid, Message: &msg})
}

func (s *Server) noteMessage(cid string, m api.ChatMessage) {
	if m.Kind == kindReset {
		return
	}
	i := s.ensureSummary(cid)
	last := &api.LastMessage{
		Sender:  api.ChatSender{Type: m.Sender.Type},
		Kind:    m.Kind,
		Preview: previewText(m.Content),
		TS:      m.TS,
	}
	if m.Sender.Type == senderWorker {
		last.Sender.ID = m.Sender.ID
	}
	if m.Handoff != nil {
		last.HandoffWorkerID = m.Handoff.WorkerID
	}
	s.st.Conversations[i].LastMessage = last
	if countsAsUnread(m) {
		s.st.Conversations[i].Unread++
	}
}

func previewText(content string) string {
	rest := content
	for strings.HasPrefix(rest, legacyMediaMarker) || strings.HasPrefix(rest, mediaMarker) {
		end := strings.IndexByte(rest, '\n')
		if end < 0 {
			break
		}
		rest = rest[end+1:]
	}
	text := strings.Join(strings.FieldsFunc(rest, unicode.IsSpace), " ")
	if runes := []rune(text); len(runes) > previewMaxRunes {
		text = string(runes[:previewMaxRunes])
	}
	return text
}

func (s *Server) uploadMedia(q *request) reply {
	if err := q.r.ParseMultipartForm(multipartMemory); err != nil {
		return errReply(http.StatusBadRequest, "missing 'file' field")
	}
	file, header, err := q.r.FormFile("file")
	if err != nil {
		return errReply(http.StatusBadRequest, "missing 'file' field")
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return errReply(http.StatusInternalServerError, err.Error())
	}
	name := strings.Trim(unsafeNameChars.ReplaceAllString(filepath.Base(header.Filename), "_"), ".")
	if name == "" {
		name = "file"
	}
	filename := fmt.Sprintf("%s_%s", s.nextID("upload"), name)
	s.st.Media[filename] = data
	return ok(map[string]string{"path": filename})
}

func inlineMedia(filename string) bool {
	mediaType, _, _ := mime.ParseMediaType(mime.TypeByExtension(filepath.Ext(filename)))
	for _, prefix := range []string{"image/", "video/", "audio/"} {
		if strings.HasPrefix(mediaType, prefix) {
			return true
		}
	}
	return mediaType == "text/plain"
}

func (s *Server) serveMedia(w http.ResponseWriter, r *http.Request) {
	filename := filepath.Base(r.PathValue("filename"))
	if filename == "." || filename == ".." {
		errReply(http.StatusBadRequest, "invalid filename").write(w)
		return
	}
	s.mu.Lock()
	data, found := s.st.Media[filename]
	s.mu.Unlock()
	if !found {
		http.NotFound(w, r)
		return
	}
	header := w.Header()
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Content-Security-Policy", "sandbox")
	if !inlineMedia(filename) {
		header.Set("Content-Type", "application/octet-stream")
		header.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	}
	http.ServeContent(w, r, filename, time.Time{}, bytes.NewReader(data))
}
