package linear

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newMockServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *httpClient) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := newHTTPClient("test-key")
	c.endpoint = srv.URL
	return srv, c
}

func TestClient_Viewer(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "test-key" {
			t.Errorf("Authorization header = %q, want test-key", got)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "viewer") {
			t.Errorf("query did not contain 'viewer': %s", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"viewer": map[string]string{"id": "U1", "name": "bot", "email": "bot@x"},
			},
		})
	})

	u, err := c.Viewer(context.Background())
	if err != nil {
		t.Fatalf("Viewer: %v", err)
	}
	if u.ID != "U1" || u.Name != "bot" {
		t.Errorf("got %+v", u)
	}
	_ = time.Now() // keep the time import live for later tests
}

func TestClient_CreateComment(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		if !strings.Contains(s, "commentCreate") {
			t.Errorf("query missing commentCreate: %s", s)
		}
		if !strings.Contains(s, `"issueId":"I1"`) {
			t.Errorf("variables missing issueId: %s", s)
		}
		if !strings.Contains(s, `"parentId":"C0"`) {
			t.Errorf("variables missing parentId: %s", s)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"commentCreate": map[string]any{
					"comment": map[string]any{
						"id":        "C9",
						"body":      "hi",
						"createdAt": "2026-05-03T00:00:00Z",
						"user":      map[string]string{"id": "U1"},
					},
				},
			},
		})
	})

	parent := "C0"
	got, err := c.CreateComment(context.Background(), "I1", "hi", &parent)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if got.ID != "C9" {
		t.Errorf("got %+v", got)
	}
}

func TestClient_IssuesUpdatedSince(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		if !strings.Contains(s, `"label":"openbee"`) {
			t.Errorf("missing label var: %s", s)
		}
		if !strings.Contains(s, `"since":"`) {
			t.Errorf("missing since var: %s", s)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issues": map[string]any{
					"nodes": []map[string]any{
						{
							"id":          "I1",
							"identifier":  "ENG-1",
							"title":       "first",
							"description": "body",
							"createdAt":   "2026-05-02T10:00:00Z",
							"updatedAt":   "2026-05-02T11:00:00Z",
							"team":        map[string]string{"key": "ENG"},
							"creator":     map[string]string{"id": "U2"},
							"labels": map[string]any{"nodes": []map[string]any{
								{"name": "openbee", "createdAt": "2026-05-02T10:30:00Z"},
							}},
							"comments": map[string]any{"nodes": []map[string]any{
								{"id": "C1", "body": "hi", "createdAt": "2026-05-02T10:45:00Z", "user": map[string]string{"id": "U2"}},
							}},
						},
					},
				},
			},
		})
	})

	since := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)
	out, err := c.IssuesUpdatedSince(context.Background(), since, "openbee")
	if err != nil {
		t.Fatalf("IssuesUpdatedSince: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(out))
	}
	if out[0].Identifier != "ENG-1" {
		t.Errorf("identifier = %q", out[0].Identifier)
	}
	if len(out[0].Labels) != 1 || out[0].Labels[0].Name != "openbee" {
		t.Errorf("labels = %+v", out[0].Labels)
	}
	if len(out[0].Comments) != 1 || out[0].Comments[0].ID != "C1" {
		t.Errorf("comments = %+v", out[0].Comments)
	}
	if out[0].Comments[0].IssueID != "I1" {
		t.Errorf("comment IssueID not back-filled: %q", out[0].Comments[0].IssueID)
	}
}
