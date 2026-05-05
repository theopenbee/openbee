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

// Comment is the subset of Linear's Comment type we care about.
type Comment struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	User      User      `json:"user"`
	ParentID  *string   `json:"parentId"`
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
	Comments    []Comment `json:"-"` // unwrapped from comments.nodes
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
query Issues($states: [String!]!, $label: String!, $projects: [String!]!, $first: Int!, $after: String) {
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
      comments(orderBy: createdAt) {
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
			"states":   states,
			"label":    label,
			"projects": projects,
			"first":    issuesPageSize,
			"after":    after,
		}
		var data struct {
			Issues struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
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
					Comments    struct {
						Nodes []Comment `json:"nodes"`
					} `json:"comments"`
				} `json:"nodes"`
			} `json:"issues"`
		}
		if err := c.do(ctx, "issues", issuesQuery, vars, &data); err != nil {
			return nil, fmt.Errorf("linear: issues page %d: %w", page, err)
		}
		for _, n := range data.Issues.Nodes {
			issue := Issue{
				ID:          n.ID,
				Identifier:  n.Identifier,
				Title:       n.Title,
				Description: n.Description,
				CreatedAt:   n.CreatedAt,
				UpdatedAt:   n.UpdatedAt,
				Team:        n.Team,
				Creator:     n.Creator,
				Comments:    n.Comments.Nodes,
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
