package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// User is the subset of Linear's User type we care about.
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Team is the subset of Linear's Team type we care about.
type Team struct {
	Key string `json:"key"` // e.g. "ENG"
}

// Project is the subset of Linear's Project type we care about.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Comment is the subset of Linear's Comment type we care about.
type Comment struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	User      User      `json:"user"`
	ParentID  *string   `json:"parentId"`
}

// FileUploadTicket is the result of Linear's fileUpload mutation. AssetURL is
// embedded into a comment markdown after the bytes are PUT to UploadURL with
// the supplied Headers.
type FileUploadTicket struct {
	AssetURL  string
	UploadURL string
	Headers   map[string]string
}

// ReactionTarget identifies what to react on. Exactly one of CommentID or
// IssueID must be non-empty; CommentID takes precedence when both are set.
type ReactionTarget struct {
	CommentID string
	IssueID   string
}

// Issue is the subset of Linear's Issue type we care about.
type Issue struct {
	ID          string    `json:"id"`
	Identifier  string    `json:"identifier"` // e.g. "ENG-42"
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Team        Team      `json:"team"`
	Creator     User      `json:"creator"`
	Project     *Project  `json:"project,omitempty"` // nil when issue has no project
	Comments    []Comment `json:"-"`                 // unwrapped from comments.nodes
}

// Client is the Linear GraphQL client surface used by the receiver and sender.
// Tests substitute a fake.
type Client interface {
	Viewer(ctx context.Context) (User, error)
	// IssuesInStates returns every issue whose state.name is in `states`,
	// carrying label `label`, and belonging to one of the given `projects`.
	// Empty `states` or `projects` is rejected by policy at the platform layer.
	// The returned slice contains all pages materialized in chronological
	// (createdAt-ascending) order.
	IssuesInStates(ctx context.Context, states []string, label string, projects []string) ([]Issue, error)
	CreateComment(ctx context.Context, issueID, body string, parentID *string) (Comment, error)
	// DownloadAsset fetches a uploads.linear.app asset using the workspace API
	// key in the Authorization header. Returns the body and the server-reported
	// Content-Type. A non-2xx response or body larger than maxBytes is returned
	// as an error. maxBytes <= 0 disables the size limit.
	DownloadAsset(ctx context.Context, url string, maxBytes int) (data []byte, contentType string, err error)
	// FileUpload runs Linear's fileUpload mutation and returns the presigned
	// upload target plus the asset URL to embed in a comment markdown.
	FileUpload(ctx context.Context, name, mime string, size int) (FileUploadTicket, error)
	// CreateReaction adds a reaction to the given target with the given emoji
	// shortcode (e.g. ":eyes:") and returns the new reaction's ID.
	CreateReaction(ctx context.Context, target ReactionTarget, emoji string) (string, error)
}

const defaultEndpoint = "https://api.linear.app/graphql"

type httpClient struct {
	apiKey   string
	endpoint string
	http     *http.Client
}

// NewClient returns a Client backed by Linear's GraphQL endpoint.
func NewClient(apiKey string) Client {
	return newHTTPClient(apiKey)
}

func newHTTPClient(apiKey string) *httpClient {
	return &httpClient{
		apiKey:   apiKey,
		endpoint: defaultEndpoint,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

func (c *httpClient) do(ctx context.Context, op string, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return fmt.Errorf("linear: marshal %s: %w", op, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("linear: build request %s: %w", op, err)
	}
	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("linear: do %s: %w", op, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("linear: %s http %d: %s", op, resp.StatusCode, string(body))
	}

	var envelope gqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("linear: decode %s: %w", op, err)
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("linear: %s graphql error: %s", op, envelope.Errors[0].Message)
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("linear: decode %s data: %w", op, err)
		}
	}
	return nil
}

const viewerQuery = `query { viewer { id name email } }`

func (c *httpClient) Viewer(ctx context.Context) (User, error) {
	var data struct {
		Viewer User `json:"viewer"`
	}
	if err := c.do(ctx, "viewer", viewerQuery, nil, &data); err != nil {
		return User{}, err
	}
	return data.Viewer, nil
}

const createCommentMutation = `
mutation CreateComment($issueId: String!, $body: String!, $parentId: String) {
  commentCreate(input: { issueId: $issueId, body: $body, parentId: $parentId }) {
    success
    comment { id body createdAt user { id } parentId }
  }
}`

func (c *httpClient) CreateComment(ctx context.Context, issueID, body string, parentID *string) (Comment, error) {
	vars := map[string]any{"issueId": issueID, "body": body, "parentId": parentID}
	var data struct {
		CommentCreate struct {
			Success bool    `json:"success"`
			Comment Comment `json:"comment"`
		} `json:"commentCreate"`
	}
	if err := c.do(ctx, "commentCreate", createCommentMutation, vars, &data); err != nil {
		return Comment{}, err
	}
	if !data.CommentCreate.Success {
		return Comment{}, fmt.Errorf("linear: commentCreate not successful")
	}
	comment := data.CommentCreate.Comment
	if comment.ID == "" {
		return Comment{}, fmt.Errorf("linear: commentCreate returned empty comment id")
	}
	return comment, nil
}

const fileUploadMutation = `
mutation FileUpload($filename: String!, $contentType: String!, $size: Int!) {
  fileUpload(filename: $filename, contentType: $contentType, size: $size) {
    success
    uploadFile {
      assetUrl
      uploadUrl
      headers { key value }
    }
  }
}`

func (c *httpClient) FileUpload(ctx context.Context, name, mime string, size int) (FileUploadTicket, error) {
	vars := map[string]any{
		"filename":    name,
		"contentType": mime,
		"size":        size,
	}
	var data struct {
		FileUpload struct {
			Success    bool `json:"success"`
			UploadFile struct {
				AssetURL  string `json:"assetUrl"`
				UploadURL string `json:"uploadUrl"`
				Headers   []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"uploadFile"`
		} `json:"fileUpload"`
	}
	if err := c.do(ctx, "fileUpload", fileUploadMutation, vars, &data); err != nil {
		return FileUploadTicket{}, err
	}
	if !data.FileUpload.Success {
		return FileUploadTicket{}, fmt.Errorf("linear: fileUpload not successful")
	}
	headers := make(map[string]string, len(data.FileUpload.UploadFile.Headers))
	for _, h := range data.FileUpload.UploadFile.Headers {
		headers[h.Key] = h.Value
	}
	return FileUploadTicket{
		AssetURL:  data.FileUpload.UploadFile.AssetURL,
		UploadURL: data.FileUpload.UploadFile.UploadURL,
		Headers:   headers,
	}, nil
}

const reactionCreateMutation = `
mutation ReactionCreate($input: ReactionCreateInput!) {
  reactionCreate(input: $input) { reaction { id } }
}`

func (c *httpClient) CreateReaction(ctx context.Context, target ReactionTarget, emoji string) (string, error) {
	input := map[string]any{"emoji": emoji}
	switch {
	case target.CommentID != "":
		input["commentId"] = target.CommentID
	case target.IssueID != "":
		input["issueId"] = target.IssueID
	default:
		return "", fmt.Errorf("linear: CreateReaction requires CommentID or IssueID")
	}
	vars := map[string]any{"input": input}
	var data struct {
		ReactionCreate struct {
			Reaction struct {
				ID string `json:"id"`
			} `json:"reaction"`
		} `json:"reactionCreate"`
	}
	if err := c.do(ctx, "reactionCreate", reactionCreateMutation, vars, &data); err != nil {
		return "", err
	}
	return data.ReactionCreate.Reaction.ID, nil
}

const downloadTimeout = 30 * time.Second

func (c *httpClient) DownloadAsset(ctx context.Context, url string, maxBytes int) ([]byte, string, error) {
	dlCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("linear: build download request: %w", err)
	}
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("linear: download asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, "", fmt.Errorf("linear: download asset http %d: %s", resp.StatusCode, string(body))
	}
	if maxBytes > 0 && resp.ContentLength > int64(maxBytes) {
		return nil, "", fmt.Errorf("linear: asset exceeds max size: %d bytes (max %d)", resp.ContentLength, maxBytes)
	}

	var reader io.Reader = resp.Body
	if maxBytes > 0 {
		reader = io.LimitReader(resp.Body, int64(maxBytes)+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, "", fmt.Errorf("linear: read asset body: %w", err)
	}
	if maxBytes > 0 && len(data) > maxBytes {
		return nil, "", fmt.Errorf("linear: asset exceeds max size: >%d bytes", maxBytes)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

const issuesPageSize = 50
const commentsPageSize = 50

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type commentsConnection struct {
	PageInfo pageInfo  `json:"pageInfo"`
	Nodes    []Comment `json:"nodes"`
}

const issuesQuery = `
query Issues($states: [String!]!, $label: String!, $projects: [String!]!, $first: Int!, $after: String, $commentsFirst: Int!) {
  issues(
    filter: {
      state:   { name: { in: $states } },
      labels:  { name: { eq: $label } },
      project: { name: { in: $projects } }
    }
    orderBy: createdAt
    first: $first
    after: $after
  ) {
    pageInfo { hasNextPage endCursor }
    nodes {
      id identifier title description createdAt updatedAt
      team { key }
      creator { id name email }
      project { id name }
      comments(orderBy: createdAt, first: $commentsFirst) {
        pageInfo { hasNextPage endCursor }
        nodes { id body createdAt user { id name email } parentId }
      }
    }
  }
}`

// IssuesInStates returns every issue whose state.name is in `states`, carrying
// label `label`, and belonging to one of the given `projects`. All pages are
// materialized within a single call.
func (c *httpClient) IssuesInStates(ctx context.Context, states []string, label string, projects []string) ([]Issue, error) {
	var all []Issue
	var after *string
	page := 0
	for {
		page++
		vars := map[string]any{
			"states":        states,
			"label":         label,
			"projects":      projects,
			"first":         issuesPageSize,
			"after":         after,
			"commentsFirst": commentsPageSize,
		}
		var data struct {
			Issues struct {
				PageInfo pageInfo `json:"pageInfo"`
				Nodes    []struct {
					ID          string             `json:"id"`
					Identifier  string             `json:"identifier"`
					Title       string             `json:"title"`
					Description string             `json:"description"`
					CreatedAt   time.Time          `json:"createdAt"`
					UpdatedAt   time.Time          `json:"updatedAt"`
					Team        Team               `json:"team"`
					Creator     User               `json:"creator"`
					Project     *Project           `json:"project"`
					Comments    commentsConnection `json:"comments"`
				} `json:"nodes"`
			} `json:"issues"`
		}
		if err := c.do(ctx, "issues", issuesQuery, vars, &data); err != nil {
			return nil, fmt.Errorf("linear: issues page %d: %w", page, err)
		}
		for _, n := range data.Issues.Nodes {
			comments := append([]Comment(nil), n.Comments.Nodes...)
			if n.Comments.PageInfo.HasNextPage {
				more, err := c.issueCommentsAfter(ctx, n.ID, n.Comments.PageInfo.EndCursor)
				if err != nil {
					return nil, fmt.Errorf("linear: issue %s comments: %w", n.ID, err)
				}
				comments = append(comments, more...)
			}
			issue := Issue{
				ID:          n.ID,
				Identifier:  n.Identifier,
				Title:       n.Title,
				Description: n.Description,
				CreatedAt:   n.CreatedAt,
				UpdatedAt:   n.UpdatedAt,
				Team:        n.Team,
				Creator:     n.Creator,
				Project:     n.Project,
				Comments:    comments,
			}
			all = append(all, issue)
		}
		if !data.Issues.PageInfo.HasNextPage {
			return all, nil
		}
		end := data.Issues.PageInfo.EndCursor
		after = &end
	}
}

const issueCommentsQuery = `
query IssueComments($issueId: String!, $first: Int!, $after: String) {
  issue(id: $issueId) {
    comments(orderBy: createdAt, first: $first, after: $after) {
      pageInfo { hasNextPage endCursor }
      nodes { id body createdAt user { id name email } parentId }
    }
  }
}`

func (c *httpClient) issueCommentsAfter(ctx context.Context, issueID, cursor string) ([]Comment, error) {
	if cursor == "" {
		return nil, fmt.Errorf("missing comment endCursor")
	}

	var all []Comment
	after := &cursor
	page := 0
	for {
		page++
		var data struct {
			Issue struct {
				Comments commentsConnection `json:"comments"`
			} `json:"issue"`
		}
		vars := map[string]any{
			"issueId": issueID,
			"first":   commentsPageSize,
			"after":   after,
		}
		if err := c.do(ctx, "issueComments", issueCommentsQuery, vars, &data); err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		all = append(all, data.Issue.Comments.Nodes...)
		if !data.Issue.Comments.PageInfo.HasNextPage {
			return all, nil
		}
		end := data.Issue.Comments.PageInfo.EndCursor
		if end == "" {
			return nil, fmt.Errorf("page %d missing endCursor", page)
		}
		after = &end
	}
}
