package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	BeeConversation = "bee"
	DefaultBeeID    = "default"
	ChatStreamPath  = "/local/stream"

	workerConversationPrefix = "worker:"
	emptyContentPlaceholder  = " "
)

func WorkerConversation(workerID string) string {
	return workerConversationPrefix + workerID
}

func ConversationWorkerID(cid string) (string, bool) {
	id, ok := strings.CutPrefix(cid, workerConversationPrefix)
	if !ok || id == "" || strings.Contains(id, ":") {
		return "", false
	}
	return id, true
}

func ActiveTasks(err error) []ActiveTask {
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict {
		return nil
	}
	raw, ok := apiErr.Details["active_tasks"]
	if !ok {
		return nil
	}
	data, marshalErr := json.Marshal(raw)
	if marshalErr != nil {
		return nil
	}
	var tasks []ActiveTask
	if json.Unmarshal(data, &tasks) != nil {
		return nil
	}
	return tasks
}

func conversationPath(cid string) string {
	return "/local/conversations/" + escapeSegment(cid)
}

func (c *Client) Conversations(ctx context.Context) ([]ConversationSummary, error) {
	var out []ConversationSummary
	if err := c.Do(ctx, http.MethodGet, "/local/conversations", nil, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []ConversationSummary{}
	}
	return out, nil
}

func (c *Client) Messages(ctx context.Context, cid string, before int64, limit int) (MessagesPage, error) {
	query := url.Values{}
	if before > 0 {
		query.Set("before", strconv.FormatInt(before, 10))
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	path := conversationPath(cid) + "/messages"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	var out MessagesPage
	if err := c.Do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return MessagesPage{}, err
	}
	if out.Messages == nil {
		out.Messages = []ChatMessage{}
	}
	return out, nil
}

func (c *Client) SendMessage(ctx context.Context, cid string, req SendMessageRequest) (SendMessageResponse, error) {
	if req.Content == "" {
		req.Content = emptyContentPlaceholder
	}
	var out SendMessageResponse
	err := c.Do(ctx, http.MethodPost, conversationPath(cid)+"/messages", req, &out)
	return out, err
}

func (c *Client) MarkRead(ctx context.Context, cid string, ts int64) error {
	body := struct {
		TS int64 `json:"ts,omitempty"`
	}{TS: ts}
	return c.Do(ctx, http.MethodPost, conversationPath(cid)+"/read", body, nil)
}

func (c *Client) ClearConversation(ctx context.Context, cid string, force bool) error {
	var body any
	if force {
		body = map[string]bool{"force": true}
	}
	return c.Do(ctx, http.MethodPost, conversationPath(cid)+"/clear", body, nil)
}

func (c *Client) StopConversation(ctx context.Context, cid string) error {
	return c.Do(ctx, http.MethodPost, conversationPath(cid)+"/stop", nil, nil)
}
