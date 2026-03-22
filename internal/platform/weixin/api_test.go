package weixin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIClient_GetUpdates(t *testing.T) {
	want := GetUpdatesResp{
		Ret: 0,
		Msgs: []WeixinMessage{
			{MessageID: 1, FromUserID: "user1", MessageType: MessageTypeUser, MessageState: MessageStateFinish},
		},
		GetUpdatesBuf: "cursor-2",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/get_updates" {
			t.Errorf("path = %q, want /ilink/bot/get_updates", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing or wrong Authorization header")
		}
		if r.Header.Get("X-WECHAT-UIN") == "" {
			t.Error("missing X-WECHAT-UIN header")
		}
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "", "test-token")
	resp, err := client.GetUpdates(context.Background(), "cursor-1")
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if resp.Ret != 0 {
		t.Errorf("ret = %d, want 0", resp.Ret)
	}
	if len(resp.Msgs) != 1 {
		t.Fatalf("msgs count = %d, want 1", len(resp.Msgs))
	}
	if resp.GetUpdatesBuf != "cursor-2" {
		t.Errorf("cursor = %q, want %q", resp.GetUpdatesBuf, "cursor-2")
	}
}

func TestAPIClient_SendMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/send_message" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var req SendMessageReq
		json.NewDecoder(r.Body).Decode(&req)
		if req.Msg == nil || req.Msg.ToUserID != "user1" {
			t.Error("expected msg with ToUserID=user1")
		}
		json.NewEncoder(w).Encode(map[string]int{"ret": 0})
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "", "test-token")
	msg := &WeixinMessage{
		ToUserID: "user1",
		ItemList: []MessageItem{{Type: MessageItemTypeText, TextItem: &TextItem{Text: "hello"}}},
	}
	err := client.SendMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
}

func TestAPIClient_GetUpdatesSessionTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(GetUpdatesResp{Ret: -1, ErrCode: -14, ErrMsg: "session timeout"})
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "", "test-token")
	resp, err := client.GetUpdates(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrCode != -14 {
		t.Errorf("errcode = %d, want -14", resp.ErrCode)
	}
}
