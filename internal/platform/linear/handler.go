package linear

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

// PlatformID is the platform identifier used in SessionKey and ingest routing.
const PlatformID = "linear"

var log = logger.With(zap.String("component", "linear"))

// cursorAPI is satisfied by *Cursor and by test fakes.
type cursorAPI interface {
	Load(ctx context.Context) (time.Time, error)
	Save(ctx context.Context, t time.Time) error
}

// LinearPlatform implements platform.Platform.
type LinearPlatform struct {
	receiver *LinearReceiver
	sender   *LinearSender
}

// NewPlatform constructs a Linear platform from configuration.
func NewPlatform(cfg config.LinearConfig, sysCfg *store.SystemConfigStore) platform.Platform {
	client := NewClient(cfg.APIKey)
	return &LinearPlatform{
		receiver: &LinearReceiver{
			client:       client,
			cursor:       NewCursor(sysCfg),
			labelName:    cfg.LabelName,
			pollInterval: cfg.PollInterval,
		},
		sender: &LinearSender{client: client},
	}
}

func (p *LinearPlatform) ID() string                                 { return PlatformID }
func (p *LinearPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *LinearPlatform) Sender() platform.PlatformSenderAdapter     { return p.sender }

// LinearReceiver polls Linear for issue/comment updates.
type LinearReceiver struct {
	client       Client
	cursor       cursorAPI
	labelName    string
	pollInterval time.Duration
	botUserID    string
}

// Start runs the polling loop until ctx is cancelled.
func (r *LinearReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	viewer, err := r.client.Viewer(ctx)
	if err != nil {
		return fmt.Errorf("linear receiver: viewer: %w", err)
	}
	r.botUserID = viewer.ID
	log.Info("linear receiver started", zap.String("bot_user_id", r.botUserID), zap.String("label", r.labelName))

	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.tickOnce(ctx, dispatch)
		}
	}
}

// tickOnce performs one polling cycle. Errors are logged; the cursor only
// advances on success.
func (r *LinearReceiver) tickOnce(ctx context.Context, dispatch func(platform.InboundMessage)) {
	since, err := r.cursor.Load(ctx)
	if err != nil {
		log.Error("cursor load", zap.Error(err))
		return
	}
	issues, err := r.client.IssuesUpdatedSince(ctx, since, r.labelName)
	if err != nil {
		log.Error("issues fetch", zap.Error(err))
		return
	}
	highWater := since
	for _, issue := range issues {
		if isNewlyOwned(issue, since, r.labelName) {
			dispatch(buildIssueInbound(issue))
		}
		for _, c := range issue.Comments {
			if !c.CreatedAt.After(since) {
				continue
			}
			if c.User.ID == r.botUserID {
				continue
			}
			dispatch(buildCommentInbound(issue, c))
			if c.CreatedAt.After(highWater) {
				highWater = c.CreatedAt
			}
		}
		if issue.UpdatedAt.After(highWater) {
			highWater = issue.UpdatedAt
		}
	}
	if highWater.After(since) {
		if err := r.cursor.Save(ctx, highWater); err != nil {
			log.Error("cursor save", zap.Error(err))
		}
	}
}

func isNewlyOwned(issue Issue, since time.Time, labelName string) bool {
	for _, l := range issue.Labels {
		if l.Name == labelName && l.CreatedAt.After(since) {
			return true
		}
	}
	return issue.CreatedAt.After(since)
}

func buildSessionKey(teamKey, identifier string) string {
	return fmt.Sprintf("%s:%s:%s", PlatformID, teamKey, identifier)
}

// replyTarget is what we serialize into InboundMessage.Raw so the sender can
// resolve the comment target without re-querying Linear.
type replyTarget struct {
	IssueID         string  `json:"issue_id"`
	ParentCommentID *string `json:"parent_comment_id,omitempty"`
}

func buildIssueInbound(issue Issue) platform.InboundMessage {
	raw, _ := json.Marshal(replyTarget{IssueID: issue.ID})
	content := issue.Title
	if issue.Description != "" {
		content = issue.Title + "\n\n" + issue.Description
	}
	return platform.InboundMessage{
		Platform:          PlatformID,
		SenderID:          issue.Creator.ID,
		SessionKey:        buildSessionKey(issue.Team.Key, issue.Identifier),
		Content:           content,
		RawContent:        content,
		Raw:               string(raw),
		PlatformMessageID: "issue:" + issue.ID,
		MessageTime:       issue.CreatedAt.UnixMilli(),
	}
}

func buildCommentInbound(issue Issue, c Comment) platform.InboundMessage {
	parent := c.ParentID
	if parent == nil {
		// Top-level comment: replies should thread under the comment itself.
		id := c.ID
		parent = &id
	}
	raw, _ := json.Marshal(replyTarget{IssueID: issue.ID, ParentCommentID: parent})
	return platform.InboundMessage{
		Platform:          PlatformID,
		SenderID:          c.User.ID,
		SessionKey:        buildSessionKey(issue.Team.Key, issue.Identifier),
		Content:           c.Body,
		RawContent:        c.Body,
		Raw:               string(raw),
		PlatformMessageID: "comment:" + c.ID,
		MessageTime:       c.CreatedAt.UnixMilli(),
	}
}

// LinearSender posts replies as Linear comments.
type LinearSender struct {
	client Client
}

// Send posts msg.Content as a comment on the issue identified by msg.ReplyTo.Raw.
func (s *LinearSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	if msg.MediaPath != "" {
		return errors.New("linear: media attachments not supported in v0")
	}
	var target replyTarget
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &target); err != nil {
		return fmt.Errorf("linear: parse reply target: %w", err)
	}
	if target.IssueID == "" {
		return errors.New("linear: reply target missing issue_id")
	}
	_, err := s.client.CreateComment(ctx, target.IssueID, msg.Content, target.ParentCommentID)
	return err
}

// Interface compliance guards.
var _ platform.Platform                = (*LinearPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*LinearReceiver)(nil)
var _ platform.PlatformSenderAdapter   = (*LinearSender)(nil)
