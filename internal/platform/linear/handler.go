package linear

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/domain/linearcfg"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/utils"
)

// PlatformID is the platform identifier used in SessionKey and ingest routing.
const PlatformID = "linear"

// selfMarker is prepended to every outbound comment body. The receiver
// recognises bot-authored comments by checking HasPrefix(body, "[openbee-bot]").
const selfMarker = "[openbee-bot]\n\n"

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

// NewPlatform constructs a Linear platform from configuration. Persistent
// state lives in ~/.openbee/.linear/. The projectStore holds the project
// allow-list and is consulted on every poll tick so runtime updates from
// SystemSettings take effect on the next cycle.
func NewPlatform(cfg config.LinearConfig, projectStore *linearcfg.Store) (platform.Platform, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("linear: resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".openbee", ".linear")
	client := NewClient(cfg.APIKey)
	return &LinearPlatform{
		receiver: &LinearReceiver{
			client:       client,
			cursor:       NewCursor(dir),
			seenComments: NewSeenComments(dir),
			labelName:    cfg.LabelName,
			pollInterval: cfg.PollInterval,
			projectStore: projectStore,
		},
		sender: &LinearSender{client: client},
	}, nil
}

func (p *LinearPlatform) ID() string                                 { return PlatformID }
func (p *LinearPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *LinearPlatform) Sender() platform.PlatformSenderAdapter     { return p.sender }

// LinearReceiver polls Linear for issue/comment updates.
type LinearReceiver struct {
	client       Client
	cursor       cursorAPI
	seenComments seenAPI
	labelName    string
	pollInterval time.Duration
	// projectStore is consulted on every tick. An empty list (the default
	// when no project is configured) means the receiver skips the API call
	// entirely — by policy, no projects = process nothing.
	projectStore *linearcfg.Store
}

// Start runs the polling loop until ctx is cancelled.
func (r *LinearReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	if err := r.seenComments.Load(ctx); err != nil {
		return fmt.Errorf("linear receiver: seen comments load: %w", err)
	}
	viewer, err := r.client.Viewer(ctx)
	if err != nil {
		return fmt.Errorf("linear receiver: viewer: %w", err)
	}
	log.Info("linear receiver started", zap.String("viewer_id", viewer.ID), zap.String("label", r.labelName))

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

// projects returns the current Linear project allow-list. A nil store yields
// nil (which the caller treats as "skip"), so tests can construct a Receiver
// without wiring a store.
func (r *LinearReceiver) projects() []string {
	if r.projectStore == nil {
		return nil
	}
	return r.projectStore.Get()
}

func (r *LinearReceiver) tickOnce(ctx context.Context, dispatch func(platform.InboundMessage)) {
	projects := r.projects()
	if len(projects) == 0 {
		return
	}
	since, err := r.cursor.Load(ctx)
	if err != nil {
		log.Error("cursor load", zap.Error(err))
		return
	}
	log.Info("tick: start",
		zap.Time("since", since),
		zap.Strings("projects", projects),
		zap.String("label", r.labelName),
	)
	issues, err := r.client.IssuesInStates(ctx, []string{"Todo", "In Progress"}, r.labelName, projects)
	if err != nil {
		log.Error("issues fetch", zap.Error(err))
		return
	}
	// truncated is no longer reported by the client. Tasks 5/6 will fully
	// rewrite this receiver; for now keep a local zero so the rest of the
	// existing tick logic compiles unchanged.
	truncated := false
	identifiers := make([]string, 0, len(issues))
	for _, i := range issues {
		identifiers = append(identifiers, i.Identifier)
	}
	log.Info("tick: api result",
		zap.Int("issue_count", len(issues)),
		zap.Bool("truncated", truncated),
		zap.Strings("identifiers", identifiers),
	)
	highWater := since
	var newIDs []string
	for _, issue := range issues {
		log.Info("tick: issue",
			zap.String("identifier", issue.Identifier),
			zap.String("issue_id", issue.ID),
			zap.Time("issue_updated_at", issue.UpdatedAt),
			zap.Time("issue_created_at", issue.CreatedAt),
			zap.Int("comment_count", len(issue.Comments)),
		)
		if isNewlyOwned(issue, since, r.labelName) {
			log.Info("tick: dispatch issue body",
				zap.String("identifier", issue.Identifier),
				zap.String("issue_id", issue.ID),
			)
			dispatch(buildIssueInbound(issue))
		}
		for _, c := range issue.Comments {
			body := c.Body
			if len(body) > 80 {
				body = body[:80] + "…"
			}
			if r.seenComments.Contains(c.ID) {
				log.Info("tick: skip comment (already seen)",
					zap.String("identifier", issue.Identifier),
					zap.String("comment_id", c.ID),
					zap.Time("comment_created_at", c.CreatedAt),
					zap.String("body_preview", body),
				)
				continue
			}
			if strings.HasPrefix(c.Body, "[openbee-bot]") {
				log.Info("tick: skip comment (self bot prefix)",
					zap.String("identifier", issue.Identifier),
					zap.String("comment_id", c.ID),
					zap.Time("comment_created_at", c.CreatedAt),
				)
				continue
			}
			log.Info("tick: dispatch comment",
				zap.String("identifier", issue.Identifier),
				zap.String("comment_id", c.ID),
				zap.Time("comment_created_at", c.CreatedAt),
				zap.String("user_id", c.User.ID),
				zap.String("body_preview", body),
			)
			dispatch(buildCommentInbound(issue, c))
			newIDs = append(newIDs, c.ID)
		}
		if issue.UpdatedAt.After(highWater) {
			log.Info("tick: highWater advanced by issue.UpdatedAt (no later comment)",
				zap.String("identifier", issue.Identifier),
				zap.Time("issue_updated_at", issue.UpdatedAt),
				zap.Time("prev_high_water", highWater),
			)
			highWater = issue.UpdatedAt
		}
	}
	if len(newIDs) > 0 {
		if err := r.seenComments.Add(ctx, newIDs); err != nil {
			log.Error("seen comments save", zap.Error(err))
		}
	}
	// Don't advance the cursor when the page is truncated: we don't know what
	// lies past the page boundary, and advancing would skip it permanently.
	if truncated {
		log.Warn("tick: cursor not advanced (page truncated)",
			zap.Time("since", since),
			zap.Time("computed_high_water", highWater),
		)
		return
	}
	if highWater.After(since) {
		log.Info("tick: cursor save",
			zap.Time("from", since),
			zap.Time("to", highWater),
		)
		if err := r.cursor.Save(ctx, highWater); err != nil {
			log.Error("cursor save", zap.Error(err))
		}
	} else {
		log.Info("tick: cursor unchanged", zap.Time("since", since))
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

// replyTarget is serialized into InboundMessage.Raw so the sender can resolve
// the comment target without re-querying Linear.
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
	// Top-level comments have no parent; reply under the comment itself so the
	// thread stays attached to the original conversation.
	parent := c.ParentID
	if parent == nil {
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
	return utils.RetryWithBackoff(ctx, func() error {
		_, err := s.client.CreateComment(ctx, target.IssueID, selfMarker+msg.Content, target.ParentCommentID)
		return err
	}, utils.DefaultRetryCount, utils.DefaultRetryDelay)
}

var _ platform.Platform = (*LinearPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*LinearReceiver)(nil)
var _ platform.PlatformSenderAdapter = (*LinearSender)(nil)
