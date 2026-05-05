package linear

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestClient_DownloadAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "test-key" {
			t.Errorf("Authorization = %q, want test-key", got)
		}
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer srv.Close()

	c := newHTTPClient("test-key")
	data, ct, err := c.DownloadAsset(context.Background(), srv.URL+"/some/path", 1024)
	if err != nil {
		t.Fatalf("DownloadAsset: %v", err)
	}
	if string(data) != "PNGDATA" {
		t.Errorf("data = %q, want PNGDATA", string(data))
	}
	if ct != "image/png" {
		t.Errorf("contentType = %q, want image/png", ct)
	}
}

func TestClient_DownloadAsset_NonOKReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := newHTTPClient("test-key")
	_, _, err := c.DownloadAsset(context.Background(), srv.URL+"/x", 1024)
	if err == nil {
		t.Fatal("expected error on non-2xx, got nil")
	}
}

func TestClient_DownloadAsset_RespectsMaxBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("12345678901"))
	}))
	defer srv.Close()

	c := newHTTPClient("test-key")
	_, _, err := c.DownloadAsset(context.Background(), srv.URL+"/large", 10)
	if err == nil {
		t.Fatal("expected error when asset exceeds max bytes")
	}
}

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
}

func TestClient_CreateComment(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		if !strings.Contains(s, "commentCreate") {
			t.Errorf("query missing commentCreate: %s", s)
		}
		if !strings.Contains(s, "success") {
			t.Errorf("query missing success: %s", s)
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
					"success": true,
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

func TestClient_CreateComment_UnsuccessfulPayloadReturnsError(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"commentCreate": map[string]any{
					"success": false,
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

	_, err := c.CreateComment(context.Background(), "I1", "hi", nil)
	if err == nil {
		t.Fatal("expected error when commentCreate success is false")
	}
}

func TestClient_CreateComment_EmptyCommentIDReturnsError(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"commentCreate": map[string]any{
					"success": true,
					"comment": map[string]any{
						"body":      "hi",
						"createdAt": "2026-05-03T00:00:00Z",
						"user":      map[string]string{"id": "U1"},
					},
				},
			},
		})
	})

	_, err := c.CreateComment(context.Background(), "I1", "hi", nil)
	if err == nil {
		t.Fatal("expected error when commentCreate returns an empty comment id")
	}
}

func TestClient_IssuesInStates_SinglePage(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		if !strings.Contains(s, `"states":["Todo","In Progress"]`) {
			t.Errorf("missing states var: %s", s)
		}
		if !strings.Contains(s, `"label":"openbee"`) {
			t.Errorf("missing label var: %s", s)
		}
		if !strings.Contains(s, `"projects":["alpha","beta"]`) {
			t.Errorf("missing projects var: %s", s)
		}
		// Decode the request and assert the variables map does not contain a
		// "since" key (i.e. no time-based filter is being passed).
		var req struct {
			Variables map[string]any `json:"variables"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if _, ok := req.Variables["since"]; ok {
			t.Errorf("request variables unexpectedly contains 'since': %v", req.Variables)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"issues": map[string]any{
					"pageInfo": map[string]any{
						"hasNextPage": false,
						"endCursor":   "",
					},
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
							"comments": map[string]any{"nodes": []map[string]any{
								{"id": "C1", "body": "hi", "createdAt": "2026-05-02T10:45:00Z", "user": map[string]string{"id": "U2"}},
							}},
						},
					},
				},
			},
		})
	})

	out, err := c.IssuesInStates(context.Background(), []string{"Todo", "In Progress"}, "openbee", []string{"alpha", "beta"})
	if err != nil {
		t.Fatalf("IssuesInStates: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(out))
	}
	if out[0].Identifier != "ENG-1" {
		t.Errorf("identifier = %q", out[0].Identifier)
	}
	if len(out[0].Comments) != 1 || out[0].Comments[0].ID != "C1" {
		t.Errorf("comments = %+v", out[0].Comments)
	}
}

func TestClient_IssuesInStates_PaginatesNestedComments(t *testing.T) {
	var (
		mu        sync.Mutex
		callCount int
	)

	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		mu.Lock()
		callCount++
		current := callCount
		mu.Unlock()

		switch current {
		case 1:
			if !strings.Contains(req.Query, "query Issues") {
				t.Fatalf("first call should query issues, got: %s", req.Query)
			}
			if got := req.Variables["commentsFirst"]; got != float64(commentsPageSize) {
				t.Errorf("commentsFirst = %v, want %d", got, commentsPageSize)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issues": map[string]any{
						"pageInfo": map[string]any{
							"hasNextPage": false,
							"endCursor":   "",
						},
						"nodes": []map[string]any{
							{
								"id":          "I1",
								"identifier":  "ENG-1",
								"title":       "first",
								"description": "",
								"createdAt":   "2026-05-02T10:00:00Z",
								"updatedAt":   "2026-05-02T11:00:00Z",
								"team":        map[string]string{"key": "ENG"},
								"creator":     map[string]string{"id": "U2"},
								"comments": map[string]any{
									"pageInfo": map[string]any{
										"hasNextPage": true,
										"endCursor":   "comments-page-2",
									},
									"nodes": []map[string]any{
										{"id": "C1", "body": "first comment", "createdAt": "2026-05-02T10:45:00Z", "user": map[string]string{"id": "U2"}},
									},
								},
							},
						},
					},
				},
			})
		case 2:
			if !strings.Contains(req.Query, "query IssueComments") {
				t.Fatalf("second call should query issue comments, got: %s", req.Query)
			}
			if got := req.Variables["issueId"]; got != "I1" {
				t.Errorf("issueId = %v, want I1", got)
			}
			if got := req.Variables["after"]; got != "comments-page-2" {
				t.Errorf("after = %v, want comments-page-2", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issue": map[string]any{
						"comments": map[string]any{
							"pageInfo": map[string]any{
								"hasNextPage": false,
								"endCursor":   "",
							},
							"nodes": []map[string]any{
								{"id": "C2", "body": "second comment", "createdAt": "2026-05-02T10:46:00Z", "user": map[string]string{"id": "U3"}},
							},
						},
					},
				},
			})
		default:
			t.Errorf("unexpected call count %d", current)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	out, err := c.IssuesInStates(context.Background(), []string{"Todo"}, "openbee", []string{"alpha"})
	if err != nil {
		t.Fatalf("IssuesInStates: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(out))
	}
	if got, want := len(out[0].Comments), 2; got != want {
		t.Fatalf("comments len = %d, want %d", got, want)
	}
	if out[0].Comments[0].ID != "C1" || out[0].Comments[1].ID != "C2" {
		t.Errorf("comments = %+v", out[0].Comments)
	}

	mu.Lock()
	defer mu.Unlock()
	if callCount != 2 {
		t.Errorf("expected 2 server calls, got %d", callCount)
	}
}

func TestClient_FileUpload(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		if !strings.Contains(s, "fileUpload") {
			t.Errorf("query missing fileUpload: %s", s)
		}
		if !strings.Contains(s, `"filename":"foo.png"`) {
			t.Errorf("variables missing filename: %s", s)
		}
		if !strings.Contains(s, `"contentType":"image/png"`) {
			t.Errorf("variables missing contentType: %s", s)
		}
		if !strings.Contains(s, `"size":42`) {
			t.Errorf("variables missing size: %s", s)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"fileUpload": map[string]any{
					"success": true,
					"uploadFile": map[string]any{
						"assetUrl":  "https://uploads.linear.app/abc.png",
						"uploadUrl": "https://s3.example/abc?sig=xyz",
						"headers": []map[string]string{
							{"key": "x-amz-acl", "value": "private"},
							{"key": "Content-Type", "value": "image/png"},
						},
					},
				},
			},
		})
	})

	got, err := c.FileUpload(context.Background(), "foo.png", "image/png", 42)
	if err != nil {
		t.Fatalf("FileUpload: %v", err)
	}
	if got.AssetURL != "https://uploads.linear.app/abc.png" {
		t.Errorf("AssetURL = %q", got.AssetURL)
	}
	if got.UploadURL != "https://s3.example/abc?sig=xyz" {
		t.Errorf("UploadURL = %q", got.UploadURL)
	}
	if got.Headers["x-amz-acl"] != "private" || got.Headers["Content-Type"] != "image/png" {
		t.Errorf("Headers = %v", got.Headers)
	}
}

func TestClient_FileUpload_GraphQLError(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]string{{"message": "denied"}},
		})
	})
	_, err := c.FileUpload(context.Background(), "foo.png", "image/png", 1)
	if err == nil {
		t.Fatal("expected error on graphql error response")
	}
}

func TestIssuesInStates_FullPagination(t *testing.T) {
	var (
		mu        sync.Mutex
		callCount int
	)

	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Variables map[string]any `json:"variables"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		mu.Lock()
		callCount++
		current := callCount
		mu.Unlock()

		switch current {
		case 1:
			// First call: after should be nil/absent.
			if v, ok := req.Variables["after"]; ok && v != nil {
				t.Errorf("first call: expected after=nil, got %v", v)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issues": map[string]any{
						"pageInfo": map[string]any{
							"hasNextPage": true,
							"endCursor":   "page2",
						},
						"nodes": []map[string]any{
							{
								"id":          "I1",
								"identifier":  "ENG-1",
								"title":       "first",
								"description": "",
								"createdAt":   "2026-05-02T10:00:00Z",
								"updatedAt":   "2026-05-02T11:00:00Z",
								"team":        map[string]string{"key": "ENG"},
								"creator":     map[string]string{"id": "U2"},
								"labels":      map[string]any{"nodes": []map[string]any{}},
								"comments":    map[string]any{"nodes": []map[string]any{}},
							},
						},
					},
				},
			})
		case 2:
			// Second call: after must be "page2".
			if v, _ := req.Variables["after"].(string); v != "page2" {
				t.Errorf("second call: expected after=\"page2\", got %v", req.Variables["after"])
			}
			if !strings.Contains(string(body), `"after":"page2"`) {
				t.Errorf("second call body missing after=page2: %s", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"issues": map[string]any{
						"pageInfo": map[string]any{
							"hasNextPage": false,
							"endCursor":   "",
						},
						"nodes": []map[string]any{
							{
								"id":          "I2",
								"identifier":  "ENG-2",
								"title":       "second",
								"description": "",
								"createdAt":   "2026-05-02T12:00:00Z",
								"updatedAt":   "2026-05-02T13:00:00Z",
								"team":        map[string]string{"key": "ENG"},
								"creator":     map[string]string{"id": "U2"},
								"labels":      map[string]any{"nodes": []map[string]any{}},
								"comments":    map[string]any{"nodes": []map[string]any{}},
							},
						},
					},
				},
			})
		default:
			t.Errorf("unexpected call count %d", current)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	out, err := c.IssuesInStates(context.Background(), []string{"Todo"}, "openbee", []string{"alpha"})
	if err != nil {
		t.Fatalf("IssuesInStates: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(out))
	}
	if out[0].ID != "I1" || out[1].ID != "I2" {
		t.Errorf("expected order [I1, I2], got [%s, %s]", out[0].ID, out[1].ID)
	}

	mu.Lock()
	defer mu.Unlock()
	if callCount != 2 {
		t.Errorf("expected 2 server calls, got %d", callCount)
	}
}
