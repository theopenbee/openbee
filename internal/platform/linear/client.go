package linear

import (
	"context"
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
	IssuesUpdatedSince(ctx context.Context, since time.Time, label string) ([]Issue, error)
	CreateComment(ctx context.Context, issueID, body string, parentID *string) (Comment, error)
}
