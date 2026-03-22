package weixin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/config"
	"github.com/theopenbee/openbee/internal/media"
	"github.com/theopenbee/openbee/internal/platform"
)

func TestReceiverFiltersMessages(t *testing.T) {
	msgs := []WeixinMessage{
		{MessageID: 1, FromUserID: "u1", MessageType: MessageTypeBot, MessageState: MessageStateFinish},    // bot msg, skip
		{MessageID: 2, FromUserID: "u2", MessageType: MessageTypeUser, MessageState: MessageStateNew},      // not finished, skip
		{MessageID: 3, FromUserID: "u3", MessageType: MessageTypeUser, MessageState: MessageStateGenerating}, // generating, skip
		{
			MessageID: 4, FromUserID: "u4", MessageType: MessageTypeUser, MessageState: MessageStateFinish,
			CreateTimeMs: 1711100000000, ContextToken: "tok",
			ItemList: []MessageItem{{Type: MessageItemTypeText, TextItem: &TextItem{Text: "hello"}}},
		},
	}

	var callCount int
	var mu2 sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ilink/bot/get_updates" {
			mu2.Lock()
			callCount++
			n := callCount
			mu2.Unlock()
			if n == 1 {
				json.NewEncoder(w).Encode(GetUpdatesResp{Ret: 0, Msgs: msgs, GetUpdatesBuf: "c2"})
				return
			}
			// Second+ poll: wait briefly then return empty so we don't block server close
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
			json.NewEncoder(w).Encode(GetUpdatesResp{Ret: 0})
			return
		}
		// Handle other requests (typing etc)
		json.NewEncoder(w).Encode(map[string]int{"ret": 0})
	}))
	defer srv.Close()

	cfg := config.WeixinConfig{Enabled: true, Token: "tok", BaseURL: srv.URL, MaxMediaSize: 50 * 1024 * 1024}
	mediaSvc := media.NewService()
	p := NewPlatform(cfg, mediaSvc)
	recv := p.Receiver()

	var received []platform.InboundMessage
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go recv.Start(ctx, func(msg platform.InboundMessage) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	})

	// Wait for messages to be dispatched
	time.Sleep(1 * time.Second)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 1 {
		t.Fatalf("dispatched %d messages, want 1", len(received))
	}
	msg := received[0]
	if msg.Platform != "weixin" {
		t.Errorf("platform = %q", msg.Platform)
	}
	if msg.SenderID != "u4" {
		t.Errorf("senderID = %q", msg.SenderID)
	}
	if msg.SessionKey != "weixin:u4:u4" {
		t.Errorf("sessionKey = %q", msg.SessionKey)
	}
	if msg.Content != "hello" {
		t.Errorf("content = %q", msg.Content)
	}
	if msg.PlatformMessageID != "4" {
		t.Errorf("platformMessageID = %q", msg.PlatformMessageID)
	}
}

func TestSenderSendsText(t *testing.T) {
	var sentMsg SendMessageReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ilink/bot/send_message":
			json.NewDecoder(r.Body).Decode(&sentMsg)
			json.NewEncoder(w).Encode(map[string]int{"ret": 0})
		default:
			json.NewEncoder(w).Encode(map[string]int{"ret": 0})
		}
	}))
	defer srv.Close()

	cfg := config.WeixinConfig{Enabled: true, Token: "tok", BaseURL: srv.URL, MaxMediaSize: 50 * 1024 * 1024}
	p := NewPlatform(cfg, media.NewService())

	rawMsg := WeixinMessage{FromUserID: "user1", ToUserID: "bot1", ContextToken: "ctx-tok"}
	rawBytes, _ := json.Marshal(rawMsg)

	err := p.Sender().Send(context.Background(), platform.OutboundMessage{
		SessionKey: "weixin:user1:user1",
		Content:    "**hello** world",
		ReplyTo: platform.InboundMessage{
			Raw: string(rawBytes),
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sentMsg.Msg == nil {
		t.Fatal("no message sent")
	}
	if sentMsg.Msg.ToUserID != "user1" {
		t.Errorf("to = %q, want user1", sentMsg.Msg.ToUserID)
	}
	// Check markdown was stripped
	if len(sentMsg.Msg.ItemList) != 1 || sentMsg.Msg.ItemList[0].TextItem == nil {
		t.Fatal("expected 1 text item")
	}
	if sentMsg.Msg.ItemList[0].TextItem.Text != "hello world" {
		t.Errorf("text = %q, want 'hello world'", sentMsg.Msg.ItemList[0].TextItem.Text)
	}
}
