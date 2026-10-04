package wecom

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/platform"
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
		mediaSvc:       media.NewService(),
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

// TestProcessMessage merges the single-field-check scenarios of processMessage
// into one table; cases needing a download stub set withDownload.
func TestProcessMessage(t *testing.T) {
	cases := []struct {
		name         string
		reqID        string
		body         messageBody
		withDownload bool
		check        func(t *testing.T, dispatched []platform.InboundMessage, mock *mockWsConn)
	}{
		{
			name:  "Voice",
			reqID: "req-002",
			body: messageBody{
				MsgID:    "msg-002",
				ChatType: "single",
				From:     messageFrom{UserID: "user2"},
				MsgType:  "voice",
				Voice:    &voiceContent{Content: "transcribed text"},
			},
			check: func(t *testing.T, dispatched []platform.InboundMessage, _ *mockWsConn) {
				require.Len(t, dispatched, 1)
				assert.Equal(t, "transcribed text", dispatched[0].Content)
				assert.Equal(t, "transcribed text", dispatched[0].RawContent)
			},
		},
		{
			name:  "GroupChat",
			reqID: "req-003",
			body: messageBody{
				MsgID:    "msg-003",
				ChatType: "group",
				ChatID:   "group-chat-1",
				From:     messageFrom{UserID: "user3"},
				MsgType:  "text",
				Text:     &textContent{Content: "group message"},
			},
			check: func(t *testing.T, dispatched []platform.InboundMessage, _ *mockWsConn) {
				require.Len(t, dispatched, 1)
				assert.Equal(t, "wecom:group-chat-1:user3", dispatched[0].SessionKey)
			},
		},
		{
			name:  "EmptyText_Skipped",
			reqID: "req-004",
			body: messageBody{
				MsgID:    "msg-004",
				ChatType: "single",
				From:     messageFrom{UserID: "user4"},
				MsgType:  "text",
				Text:     &textContent{Content: ""},
			},
			check: func(t *testing.T, dispatched []platform.InboundMessage, mock *mockWsConn) {
				assert.Empty(t, dispatched)
				assert.Empty(t, mock.replies) // no thinking message either
			},
		},
		{
			name:  "UnsupportedMsgType_Skipped",
			reqID: "req-005",
			body: messageBody{
				MsgID:    "msg-005",
				ChatType: "single",
				From:     messageFrom{UserID: "user5"},
				MsgType:  "link", // unsupported
			},
			check: func(t *testing.T, dispatched []platform.InboundMessage, _ *mockWsConn) {
				assert.Empty(t, dispatched)
			},
		},
		{
			name:         "Image",
			reqID:        "req-010",
			withDownload: true,
			body: messageBody{
				MsgID:    "msg-010",
				ChatType: "single",
				From:     messageFrom{UserID: "u1"},
				MsgType:  "image",
				Image:    &encryptedMedia{URL: "https://example.com/img.jpg", AesKey: "key1"},
			},
			check: func(t *testing.T, dispatched []platform.InboundMessage, _ *mockWsConn) {
				require.Len(t, dispatched, 1)
				assert.Contains(t, dispatched[0].Content, "image")
				assert.Equal(t, "", dispatched[0].RawContent)
			},
		},
		{
			name:         "File",
			reqID:        "req-011",
			withDownload: true,
			body: messageBody{
				MsgID:    "msg-011",
				ChatType: "single",
				From:     messageFrom{UserID: "u1"},
				MsgType:  "file",
				File:     &encryptedMedia{URL: "https://example.com/doc.pdf", AesKey: "key2"},
			},
			check: func(t *testing.T, dispatched []platform.InboundMessage, _ *mockWsConn) {
				require.Len(t, dispatched, 1)
				assert.Contains(t, dispatched[0].Content, "document")
			},
		},
		{
			name:         "Mixed",
			reqID:        "req-012",
			withDownload: true,
			body: messageBody{
				MsgID:    "msg-012",
				ChatType: "single",
				From:     messageFrom{UserID: "u1"},
				MsgType:  "mixed",
				Mixed: &mixedContent{MsgItem: []mixedItem{
					{MsgType: "text", Text: &textContent{Content: "look at this:"}},
					{MsgType: "image", Image: &encryptedMedia{URL: "https://example.com/x.png", AesKey: "key3"}},
				}},
			},
			check: func(t *testing.T, dispatched []platform.InboundMessage, _ *mockWsConn) {
				require.Len(t, dispatched, 1)
				assert.Contains(t, dispatched[0].Content, "look at this:")
				assert.Contains(t, dispatched[0].Content, "image")
				assert.Equal(t, "look at this:", dispatched[0].RawContent)
			},
		},
		{
			name:         "TextWithQuote",
			reqID:        "req-013",
			withDownload: true,
			body: messageBody{
				MsgID:    "msg-013",
				ChatType: "single",
				From:     messageFrom{UserID: "u1"},
				MsgType:  "text",
				Text:     &textContent{Content: "my reply"},
				Quote: &quoteContent{
					MsgType: "text",
					Text:    &textContent{Content: "quoted original"},
				},
			},
			check: func(t *testing.T, dispatched []platform.InboundMessage, _ *mockWsConn) {
				require.Len(t, dispatched, 1)
				assert.Contains(t, dispatched[0].Content, "my reply")
				assert.Contains(t, dispatched[0].Content, "quoted original")
			},
		},
		{
			name:         "QuoteFile",
			reqID:        "req-014",
			withDownload: true,
			body: messageBody{
				MsgID:    "msg-014",
				ChatType: "single",
				From:     messageFrom{UserID: "u1"},
				MsgType:  "text",
				Text:     &textContent{Content: "see attached"},
				Quote: &quoteContent{
					MsgType: "file",
					File:    &encryptedMedia{URL: "https://example.com/q.pdf", AesKey: "key4"},
				},
			},
			check: func(t *testing.T, dispatched []platform.InboundMessage, _ *mockWsConn) {
				require.Len(t, dispatched, 1)
				assert.Contains(t, dispatched[0].Content, "see attached")
				assert.Contains(t, dispatched[0].Content, "document")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockWsConn{}
			var r *WeComReceiver
			if tc.withDownload {
				r = newTestReceiverWithDownload(mock)
			} else {
				r = newTestReceiver(mock)
			}

			frame := buildFrame(t, tc.reqID, tc.body)

			var dispatched []platform.InboundMessage
			r.processMessage(frame, func(m platform.InboundMessage) { dispatched = append(dispatched, m) })

			tc.check(t, dispatched, mock)
		})
	}
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

// newTestReceiverWithDownload returns a receiver whose downloadDecryptSave
// always returns a predictable placeholder (no real HTTP).
func newTestReceiverWithDownload(mock *mockWsConn) *WeComReceiver {
	r := newTestReceiver(mock)
	r.downloadFn = func(_ context.Context, url, _, mediaType, filename string) string {
		if url == "" {
			return r.mediaSvc.BuildPlaceholder(mediaType, "", filename)
		}
		return r.mediaSvc.BuildPlaceholder(mediaType, "/tmp/fake-"+mediaType, filename)
	}
	return r
}

// mockSendReply records all SendReply calls and returns a configurable response.
type mockSendReply struct {
	mu       sync.Mutex
	calls    []sentReply
	response map[string]WsFrame // cmd → response frame to return
}

func (m *mockSendReply) fn(reqID, cmd string, body any) (WsFrame, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, sentReply{reqID: reqID, cmd: cmd, body: body})
	if resp, ok := m.response[cmd]; ok {
		return resp, nil
	}
	return WsFrame{}, nil
}

func buildUploadInitResponse(uploadID string) WsFrame {
	body, _ := json.Marshal(map[string]string{"upload_id": uploadID})
	return WsFrame{Body: body}
}

func buildUploadFinishResponse(mediaID string) WsFrame {
	body, _ := json.Marshal(map[string]string{"media_id": mediaID})
	return WsFrame{Body: body}
}

func TestSend_TextReply(t *testing.T) {
	mock := &mockSendReply{}
	var ps sync.Map
	s := &WeComSender{pendingStreams: &ps, sendReplyFn: mock.fn}

	// Pre-store stream ID
	ps.Store("msg-100", "stream-abc")

	raw := buildRawFrame(t, "req-100", messageBody{
		MsgID: "msg-100", ChatType: "single", From: messageFrom{UserID: "u1"},
	})
	err := s.Send(context.Background(), platform.OutboundMessage{
		Content: "hello back",
		ReplyTo: platform.InboundMessage{Raw: raw},
	})
	require.NoError(t, err)

	require.Len(t, mock.calls, 1)
	assert.Equal(t, WsCmdResponse, mock.calls[0].cmd)
	assert.Equal(t, "req-100", mock.calls[0].reqID)

	body := mock.calls[0].body.(streamBody)
	assert.True(t, body.Stream.Finish)
	assert.Equal(t, "hello back", body.Stream.Content)
	assert.Equal(t, "stream-abc", body.Stream.ID)
}

func TestSend_MediaUpload(t *testing.T) {
	// Create a small temp file
	tmpFile := t.TempDir() + "/test.png"
	require.NoError(t, os.WriteFile(tmpFile, make([]byte, 100), 0600))

	mock := &mockSendReply{
		response: map[string]WsFrame{
			WsCmdUploadMediaInit:   buildUploadInitResponse("upload-xyz"),
			WsCmdUploadMediaFinish: buildUploadFinishResponse("media-xyz"),
		},
	}
	var ps sync.Map
	ps.Store("msg-200", "stream-200")
	s := &WeComSender{pendingStreams: &ps, sendReplyFn: mock.fn, mediaSvc: media.NewService()}

	raw := buildRawFrame(t, "req-200", messageBody{
		MsgID: "msg-200", ChatType: "single", From: messageFrom{UserID: "u1"},
	})
	err := s.Send(context.Background(), platform.OutboundMessage{
		MediaPath: tmpFile,
		ReplyTo:   platform.InboundMessage{Raw: raw},
	})
	require.NoError(t, err)

	cmds := make([]string, len(mock.calls))
	for i, c := range mock.calls {
		cmds[i] = c.cmd
	}
	// Expected: init, chunk(s), finish, send_msg, respond_msg (thinking finish)
	assert.Contains(t, cmds, WsCmdUploadMediaInit)
	assert.Contains(t, cmds, WsCmdUploadMediaChunk)
	assert.Contains(t, cmds, WsCmdUploadMediaFinish)
	assert.Contains(t, cmds, WsCmdSendMsg)
	assert.Contains(t, cmds, WsCmdResponse)
}

func TestSend_FileTooLarge(t *testing.T) {
	var ps sync.Map
	ps.Store("msg-300", "stream-300")
	mock := &mockSendReply{}
	s := &WeComSender{pendingStreams: &ps, sendReplyFn: mock.fn}

	// Create exactly 20MB+1 byte file
	tmpFile := t.TempDir() + "/big.bin"
	data := make([]byte, 20*1024*1024+1)
	require.NoError(t, os.WriteFile(tmpFile, data, 0600))

	raw := buildRawFrame(t, "req-300", messageBody{
		MsgID: "msg-300", ChatType: "single", From: messageFrom{UserID: "u1"},
	})
	err := s.Send(context.Background(), platform.OutboundMessage{
		MediaPath: tmpFile,
		ReplyTo:   platform.InboundMessage{Raw: raw},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

// buildRawFrame creates the Raw string for OutboundMessage.ReplyTo.
func buildRawFrame(t *testing.T, reqID string, body messageBody) string {
	t.Helper()
	bodyJSON, _ := json.Marshal(body)
	frame := WsFrame{
		Cmd:     WsCmdCallback,
		Headers: WsFrameHeaders{ReqID: reqID},
		Body:    bodyJSON,
	}
	raw, _ := json.Marshal(frame)
	return string(raw)
}

func TestExtractContext_SingleChat(t *testing.T) {
	body := `{"msgid":"msg1","aibotid":"bot1","chatid":"","chattype":"single","from":{"userid":"user1"},"msgtype":"text","create_time":1700000000}`
	frame := `{"cmd":"aibot_callback","headers":{"req_id":"req1"},"body":` + body + `}`
	got := ExtractContext(frame)
	require.NotEmpty(t, got)
	// from must be a nested object, not flattened
	assert.Contains(t, got, `"from"`)
	assert.Contains(t, got, `"userid":"user1"`)
	// single chat: chatid must be empty string, NOT overridden with userid
	assert.Contains(t, got, `"chatid":""`)
	// msgtype must be present
	assert.Contains(t, got, `"msgtype":"text"`)
	// userid must NOT appear as a top-level key (old flattened field)
	assert.NotContains(t, got, `"userid":"user1","`)
	assert.False(t, strings.HasPrefix(got, `{"wecom":{"userid"`), "userid should not be a top-level context field")
}

func TestExtractContext_GroupChat(t *testing.T) {
	body := `{"msgid":"msg2","aibotid":"bot1","chatid":"group1","chattype":"group","from":{"userid":"user1"},"msgtype":"text","create_time":1700000000}`
	frame := `{"cmd":"aibot_callback","headers":{"req_id":"req1"},"body":` + body + `}`
	got := ExtractContext(frame)
	require.NotEmpty(t, got)
	// group chat: chatid must be the group ID
	assert.Contains(t, got, `"chatid":"group1"`)
	assert.Contains(t, got, `"chattype":"group"`)
}

func TestExtractContext_InvalidRaw(t *testing.T) {
	got := ExtractContext("not-json")
	assert.Empty(t, got)
}

func TestExtractContext_InvalidBody(t *testing.T) {
	// Valid WsFrame but body is not a messageBody
	frame := `{"cmd":"aibot_callback","headers":{"req_id":"req1"},"body":"not-an-object"}`
	got := ExtractContext(frame)
	assert.Empty(t, got)
}
