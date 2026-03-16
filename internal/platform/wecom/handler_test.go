package wecom

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/robobee/core/internal/media"
	"github.com/robobee/core/internal/platform"
)

// mockWsConn replaces WsConn in tests — captures SendReply calls, never dials.
type mockWsConn struct {
	mu      sync.Mutex
	replies []sentReply
}

type sentReply struct {
	reqID string
	cmd   string
	body  any
}

func (m *mockWsConn) sendReply(reqID, cmd string, body any) (WsFrame, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.replies = append(m.replies, sentReply{reqID: reqID, cmd: cmd, body: body})
	return WsFrame{}, nil
}

// buildFrame constructs a minimal WsFrame with the given message body.
func buildFrame(t *testing.T, reqID string, body messageBody) WsFrame {
	t.Helper()
	bodyJSON, err := json.Marshal(body)
	require.NoError(t, err)
	return WsFrame{
		Cmd:     WsCmdCallback,
		Headers: WsFrameHeaders{ReqID: reqID},
		Body:    bodyJSON,
	}
}

func newTestReceiver(mock *mockWsConn) *WeComReceiver {
	var ps sync.Map
	r := &WeComReceiver{
		pendingStreams: &ps,
		mediaSvc:      media.NewService(),
	}
	// Inject mock send function
	r.sendReplyFn = mock.sendReply
	return r
}

func TestProcessMessage_Text(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiver(mock)

	frame := buildFrame(t, "req-001", messageBody{
		MsgID:    "msg-001",
		ChatType: "single",
		From:     messageFrom{UserID: "user1"},
		MsgType:  "text",
		Text:     &textContent{Content: "hello world"},
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	require.Len(t, dispatched, 1)
	msg := dispatched[0]
	assert.Equal(t, "wecom", msg.Platform)
	assert.Equal(t, "user1", msg.SenderID)
	assert.Equal(t, "wecom:user1:user1", msg.SessionKey)
	assert.Equal(t, "hello world", msg.Content)
	assert.Equal(t, "hello world", msg.RawContent)
	assert.Equal(t, "msg-001", msg.PlatformMessageID)

	// Thinking message should have been sent
	require.Len(t, mock.replies, 1)
	assert.Equal(t, "req-001", mock.replies[0].reqID)
	assert.Equal(t, WsCmdResponse, mock.replies[0].cmd)
}

func TestProcessMessage_Voice(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiver(mock)

	frame := buildFrame(t, "req-002", messageBody{
		MsgID:    "msg-002",
		ChatType: "single",
		From:     messageFrom{UserID: "user2"},
		MsgType:  "voice",
		Voice:    &voiceContent{Content: "transcribed text"},
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	require.Len(t, dispatched, 1)
	assert.Equal(t, "transcribed text", dispatched[0].Content)
	assert.Equal(t, "transcribed text", dispatched[0].RawContent)
}

func TestProcessMessage_GroupChat(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiver(mock)

	frame := buildFrame(t, "req-003", messageBody{
		MsgID:    "msg-003",
		ChatType: "group",
		ChatID:   "group-chat-1",
		From:     messageFrom{UserID: "user3"},
		MsgType:  "text",
		Text:     &textContent{Content: "group message"},
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	require.Len(t, dispatched, 1)
	assert.Equal(t, "wecom:group-chat-1:user3", dispatched[0].SessionKey)
}

func TestProcessMessage_EmptyText_Skipped(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiver(mock)

	frame := buildFrame(t, "req-004", messageBody{
		MsgID:    "msg-004",
		ChatType: "single",
		From:     messageFrom{UserID: "user4"},
		MsgType:  "text",
		Text:     &textContent{Content: ""},
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	assert.Empty(t, dispatched)
	assert.Empty(t, mock.replies) // no thinking message either
}

func TestProcessMessage_UnsupportedMsgType_Skipped(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiver(mock)

	frame := buildFrame(t, "req-005", messageBody{
		MsgID:    "msg-005",
		ChatType: "single",
		From:     messageFrom{UserID: "user5"},
		MsgType:  "link", // unsupported
	})

	var dispatched []platform.InboundMessage
	r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

	assert.Empty(t, dispatched)
}

func TestProcessMessage_PendingStreams(t *testing.T) {
	mock := &mockWsConn{}
	r := newTestReceiver(mock)

	frame := buildFrame(t, "req-006", messageBody{
		MsgID:    "msg-006",
		ChatType: "single",
		From:     messageFrom{UserID: "user6"},
		MsgType:  "text",
		Text:     &textContent{Content: "hello"},
	})

	r.processMessage(frame, func(m platform.InboundMessage) {})

	// Stream ID should be stored
	val, ok := r.pendingStreams.Load("msg-006")
	assert.True(t, ok)
	assert.NotEmpty(t, val.(string))
}
