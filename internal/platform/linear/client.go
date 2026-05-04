package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
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

// IssueLabel carries the per-issue label assignment timestamp so we can detect
// when an issue first received the gating label.
type IssueLabel struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// Comment is the subset of Linear's Comment type we care about.
type Comment struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	User      User      `json:"user"`
	ParentID  *string   `json:"parentId"`
	IssueID   string    `json:"-"` // populated when read from Issue.Comments
}

// Issue is the subset of Linear's Issue type we care about.
type Issue struct {
	ID          string       `json:"id"`
	Identifier  string       `json:"identifier"` // e.g. "ENG-42"
	Title       string       `json:"title"`
	Description string       `json:"description"`
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
	Team        Team         `json:"team"`
	Creator     User         `json:"creator"`
	Labels      []IssueLabel `json:"-"` // unwrapped from labels.nodes
	Comments    []Comment    `json:"-"` // unwrapped from comments.nodes
}

// Client is the Linear GraphQL client surface used by the receiver and sender.
// Tests substitute a fake.
type Client interface {
	Viewer(ctx context.Context) (User, error)
	// IssuesUpdatedSince returns issues whose updatedAt is greater than `since`,
	// carry the gating `label`, and belong to one of the given `projects`. The
	// caller must pass a non-empty `projects` list — empty is rejected by
	// policy at the platform layer.
	IssuesUpdatedSince(ctx context.Context, since time.Time, label string, projects []string) (issues []Issue, truncated bool, err error)
	CreateComment(ctx context.Context, issueID, body string, parentID *string) (Comment, error)
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
    comment { id body createdAt user { id } parentId }
  }
}`

func (c *httpClient) CreateComment(ctx context.Context, issueID, body string, parentID *string) (Comment, error) {
	vars := map[string]any{"issueId": issueID, "body": body, "parentId": parentID}
	var data struct {
		CommentCreate struct {
			Comment Comment `json:"comment"`
		} `json:"commentCreate"`
	}
	if err := c.do(ctx, "commentCreate", createCommentMutation, vars, &data); err != nil {
		return Comment{}, err
	}
	return data.CommentCreate.Comment, nil
}

const issuesPageSize = 50

const issuesQuery = `
query Issues($since: DateTimeOrDuration!, $label: String!, $projects: [String!]!, $first: Int!) {
  issues(
    filter: {
      updatedAt: { gt: $since },
      labels: { name: { eq: $label } },
      project: { name: { in: $projects } }
    }
    orderBy: updatedAt
    first: $first
  ) {
    pageInfo { hasNextPage }
    nodes {
      id identifier title description createdAt updatedAt
      team { key }
      creator { id name email }
      labels(filter: { name: { eq: $label } }) {
        nodes { name createdAt }
      }
      comments(filter: { createdAt: { gt: $since } }, orderBy: createdAt) {
        nodes { id body createdAt user { id name email } parentId }
      }
    }
  }
}`

// IssuesUpdatedSince returns matching issues. If hasNextPage is true the
// returned bool is true, signaling the caller that the result is truncated and
// the cursor must not be advanced.
func (c *httpClient) IssuesUpdatedSince(ctx context.Context, since time.Time, label string, projects []string) (issues []Issue, truncated bool, err error) {
	vars := map[string]any{
		"since":    since.UTC().Format(time.RFC3339),
		"label":    label,
		"projects": projects,
		"first":    issuesPageSize,
	}
	var data struct {
		Issues struct {
			PageInfo struct {
				HasNextPage bool `json:"hasNextPage"`
			} `json:"pageInfo"`
			Nodes []struct {
				ID          string    `json:"id"`
				Identifier  string    `json:"identifier"`
				Title       string    `json:"title"`
				Description string    `json:"description"`
				CreatedAt   time.Time `json:"createdAt"`
				UpdatedAt   time.Time `json:"updatedAt"`
				Team        Team      `json:"team"`
				Creator     User      `json:"creator"`
				Labels      struct {
					Nodes []IssueLabel `json:"nodes"`
				} `json:"labels"`
				Comments struct {
					Nodes []Comment `json:"nodes"`
				} `json:"comments"`
			} `json:"nodes"`
		} `json:"issues"`
	}
	if err := c.do(ctx, "issues", issuesQuery, vars, &data); err != nil {
		return nil, false, err
	}
	truncated = data.Issues.PageInfo.HasNextPage
	if truncated {
		log.Warn("linear: issues page truncated; cursor will not advance until next tick clears it",
			zap.Int("page_size", issuesPageSize))
	}
	log.Info("linear: graphql issues response",
		zap.String("since", vars["since"].(string)),
		zap.Int("returned", len(data.Issues.Nodes)),
		zap.Bool("has_next_page", truncated),
	)
	out := make([]Issue, 0, len(data.Issues.Nodes))
	for _, n := range data.Issues.Nodes {
		commentIDs := make([]string, 0, len(n.Comments.Nodes))
		for _, cn := range n.Comments.Nodes {
			commentIDs = append(commentIDs, cn.ID)
		}
		log.Info("linear: graphql issue node",
			zap.String("identifier", n.Identifier),
			zap.Time("updated_at", n.UpdatedAt),
			zap.Int("comments_returned", len(n.Comments.Nodes)),
			zap.Strings("comment_ids", commentIDs),
		)
		issue := Issue{
			ID:          n.ID,
			Identifier:  n.Identifier,
			Title:       n.Title,
			Description: n.Description,
			CreatedAt:   n.CreatedAt,
			UpdatedAt:   n.UpdatedAt,
			Team:        n.Team,
			Creator:     n.Creator,
			Labels:      n.Labels.Nodes,
			Comments:    n.Comments.Nodes,
		}
		for i := range issue.Comments {
			issue.Comments[i].IssueID = issue.ID
		}
		out = append(out, issue)
	}
	return out, truncated, nil
}
