package linear

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_DownloadAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer srv.Close()

	c := newHTTPClient("test-key")
	data, ct, err := c.DownloadAsset(context.Background(), srv.URL+"/some/path", 1024)
	require.NoError(t, err)
	assert.Equal(t, "PNGDATA", string(data))
	assert.Equal(t, "image/png", ct)
}

func TestClient_DownloadAsset_NonOKReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := newHTTPClient("test-key")
	_, _, err := c.DownloadAsset(context.Background(), srv.URL+"/x", 1024)
	require.Error(t, err)
}

func TestClient_DownloadAsset_RespectsMaxBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("12345678901"))
	}))
	defer srv.Close()

	c := newHTTPClient("test-key")
	_, _, err := c.DownloadAsset(context.Background(), srv.URL+"/large", 10)
	require.Error(t, err)
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
		assert.Equal(t, "test-key", r.Header.Get("Authorization"))
		assert.Equal(t, http.MethodPost, r.Method)
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), "viewer")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"viewer": map[string]string{"id": "U1", "name": "bot", "email": "bot@x"},
			},
		})
	})

	u, err := c.Viewer(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "U1", u.ID)
	assert.Equal(t, "bot", u.Name)
}

func TestClient_CreateComment(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		assert.Contains(t, s, "commentCreate")
		assert.Contains(t, s, "success")
		assert.Contains(t, s, `"issueId":"I1"`)
		assert.Contains(t, s, `"parentId":"C0"`)
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
	require.NoError(t, err)
	assert.Equal(t, "C9", got.ID)
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
	require.Error(t, err)
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
	require.Error(t, err)
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
		assert.NoError(t, json.Unmarshal(body, &req))

		mu.Lock()
		callCount++
		current := callCount
		mu.Unlock()

		switch current {
		case 1:
			assert.Contains(t, req.Query, "query Issues")
			assert.Equal(t, float64(commentsPageSize), req.Variables["commentsFirst"])
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
			assert.Contains(t, req.Query, "query IssueComments")
			assert.Equal(t, "I1", req.Variables["issueId"])
			assert.Equal(t, "comments-page-2", req.Variables["after"])
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
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Comments, 2)
	assert.Equal(t, "C1", out[0].Comments[0].ID)
	assert.Equal(t, "C2", out[0].Comments[1].ID)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, callCount)
}

func TestClient_FileUpload(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		assert.Contains(t, s, "fileUpload")
		assert.Contains(t, s, `"filename":"foo.png"`)
		assert.Contains(t, s, `"contentType":"image/png"`)
		assert.Contains(t, s, `"size":42`)
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
	require.NoError(t, err)
	assert.Equal(t, "https://uploads.linear.app/abc.png", got.AssetURL)
	assert.Equal(t, "https://s3.example/abc?sig=xyz", got.UploadURL)
	assert.Equal(t, "private", got.Headers["x-amz-acl"])
	assert.Equal(t, "image/png", got.Headers["Content-Type"])
}

func TestClient_FileUpload_GraphQLError(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]string{{"message": "denied"}},
		})
	})
	_, err := c.FileUpload(context.Background(), "foo.png", "image/png", 1)
	require.Error(t, err)
}

// TestClient_CreateReaction covers reacting to a comment vs. an issue: the
// mutation input must carry exactly the matching target ID field.
func TestClient_CreateReaction(t *testing.T) {
	cases := []struct {
		name           string
		target         ReactionTarget
		wantInput      string
		wantAbsent     string
		wantReactionID string
	}{
		{
			name:           "OnComment",
			target:         ReactionTarget{CommentID: "C9"},
			wantInput:      `"commentId":"C9"`,
			wantAbsent:     `"issueId":`,
			wantReactionID: "R1",
		},
		{
			name:           "OnIssue",
			target:         ReactionTarget{IssueID: "I1"},
			wantInput:      `"issueId":"I1"`,
			wantAbsent:     `"commentId":`,
			wantReactionID: "R2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				s := string(body)
				assert.Contains(t, s, "reactionCreate")
				assert.Contains(t, s, tc.wantInput)
				assert.Contains(t, s, `"emoji":":eyes:"`)
				assert.NotContains(t, s, tc.wantAbsent)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{
						"reactionCreate": map[string]any{
							"reaction": map[string]any{"id": tc.wantReactionID},
						},
					},
				})
			})

			id, err := c.CreateReaction(context.Background(), tc.target, ":eyes:")
			require.NoError(t, err)
			assert.Equal(t, tc.wantReactionID, id)
		})
	}
}

func TestClient_CreateReaction_RejectsEmptyTarget(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("HTTP should not be called when target is empty")
	})
	_, err := c.CreateReaction(context.Background(), ReactionTarget{}, ":eyes:")
	require.Error(t, err)
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
		assert.NoError(t, json.Unmarshal(body, &req))

		mu.Lock()
		callCount++
		current := callCount
		mu.Unlock()

		switch current {
		case 1:
			// First call: after should be nil/absent.
			assert.Nil(t, req.Variables["after"], "first call: expected after=nil")
			s := string(body)
			assert.Contains(t, s, `"states":["Todo"]`)
			assert.Contains(t, s, `"label":"openbee"`)
			assert.Contains(t, s, `"projects":["alpha"]`)
			_, hasSince := req.Variables["since"]
			assert.False(t, hasSince, "request variables unexpectedly contains 'since': %v", req.Variables)
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
								"comments": map[string]any{"nodes": []map[string]any{
									{"id": "C1", "body": "hi", "createdAt": "2026-05-02T10:45:00Z", "user": map[string]string{"id": "U2"}},
								}},
							},
						},
					},
				},
			})
		case 2:
			// Second call: after must be "page2".
			assert.Equal(t, "page2", req.Variables["after"])
			assert.Contains(t, string(body), `"after":"page2"`)
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
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, "I1", out[0].ID)
	assert.Equal(t, "ENG-1", out[0].Identifier)
	assert.Equal(t, "I2", out[1].ID)
	require.Len(t, out[0].Comments, 1) // guard: index access out[0].Comments[0] below
	assert.Equal(t, "C1", out[0].Comments[0].ID)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, callCount)
}
