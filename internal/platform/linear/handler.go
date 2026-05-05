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

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/utils"
)

// PlatformID is the platform identifier used in SessionKey and ingest routing.
const PlatformID = "linear"

// botCommentPrefix marks bot-authored comments. Inbound dedup matches on this
// prefix alone so a stray formatting variant still filters; outbound bodies use
// selfMarker (prefix + blank line) for readability.
const botCommentPrefix = "[openbee-bot]"
const selfMarker = botCommentPrefix + "\n\n"

var log = logger.With(zap.String("component", "linear"))

// LinearPlatform implements platform.Platform.
type LinearPlatform struct {
	receiver *LinearReceiver
	sender   *LinearSender
}

// NewPlatform constructs a Linear platform from configuration. Persistent
// state (seen_issues.ndjson, seen_comments.ndjson) lives in ~/.openbee/.linear/.
func NewPlatform(cfg config.LinearConfig) (platform.Platform, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("linear: resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".openbee", ".linear")
	client := NewClient(cfg.APIKey)
	return &LinearPlatform{
		receiver: &LinearReceiver{
			client:       client,
			seenIssues:   NewSeenSet(dir, "seen_issues.ndjson"),
			seenComments: NewSeenSet(dir, "seen_comments.ndjson"),
			labelName:    cfg.LabelName,
			pollInterval: cfg.PollInterval,
			projectsList: cleanStringList(cfg.Projects),
			statesList:   cleanStringList(cfg.States),
		},
		sender: &LinearSender{client: client},
	}, nil
}

func (p *LinearPlatform) ID() string                                 { return PlatformID }
func (p *LinearPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *LinearPlatform) Sender() platform.PlatformSenderAdapter     { return p.sender }

// LinearReceiver polls Linear for issue/comment updates by workflow-state.
type LinearReceiver struct {
	client       Client
	seenIssues   seenAPI
	seenComments seenAPI
	labelName    string
	pollInterval time.Duration
	projectsList []string
	statesList   []string
	resolver     *resolver
}

// Start runs the polling loop until ctx is cancelled.
func (r *LinearReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	if err := r.seenIssues.Load(ctx); err != nil {
		return fmt.Errorf("linear receiver: seen issues load: %w", err)
	}
	if err := r.seenComments.Load(ctx); err != nil {
		return fmt.Errorf("linear receiver: seen comments load: %w", err)
	}
	viewer, err := r.client.Viewer(ctx)
	if err != nil {
		return fmt.Errorf("linear receiver: viewer: %w", err)
	}
	log.Info("linear receiver started",
		zap.String("viewer_id", viewer.ID),
		zap.String("label", r.labelName),
	)

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

func (r *LinearReceiver) projects() []string {
	return append([]string(nil), r.projectsList...)
}

func (r *LinearReceiver) states() []string {
	return append([]string(nil), r.statesList...)
}

func cleanStringList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func (r *LinearReceiver) tickOnce(ctx context.Context, dispatch func(platform.InboundMessage)) {
	projects := r.projects()
	states := r.states()
	if len(projects) == 0 || len(states) == 0 {
		return
	}
	issues, err := r.client.IssuesInStates(ctx, states, r.labelName, projects)
	if err != nil {
		log.Error("issues fetch", zap.Error(err))
		return
	}

	var newIssueIDs []string
	var newCommentIDs []string

	for _, issue := range issues {
		if !r.seenIssues.Contains(issue.ID) {
			nonBot := nonBotComments(issue.Comments)
			resolvedIssue := issue
			resolvedIssue.Description = r.resolver.Resolve(ctx, issue.Description)
			resolvedComments := make([]Comment, len(nonBot))
			for i, c := range nonBot {
				rc := c
				rc.Body = r.resolver.Resolve(ctx, c.Body)
				resolvedComments[i] = rc
			}
			log.Debug("tick: dispatch initial merged",
				zap.String("identifier", issue.Identifier),
				zap.String("issue_id", issue.ID),
				zap.Int("non_bot_comment_count", len(nonBot)),
			)
			dispatch(buildInitialInbound(resolvedIssue, resolvedComments))
			newIssueIDs = append(newIssueIDs, issue.ID)
			for _, c := range nonBot {
				newCommentIDs = append(newCommentIDs, c.ID)
			}
			continue
		}
		for _, c := range issue.Comments {
			if r.seenComments.Contains(c.ID) {
				continue
			}
			if strings.HasPrefix(c.Body, botCommentPrefix) {
				continue
			}
			rc := c
			rc.Body = r.resolver.Resolve(ctx, c.Body)
			log.Debug("tick: dispatch comment",
				zap.String("identifier", issue.Identifier),
				zap.String("comment_id", c.ID),
				zap.String("user_id", c.User.ID),
			)
			dispatch(buildCommentInbound(issue, rc))
			newCommentIDs = append(newCommentIDs, c.ID)
		}
	}

	if len(newIssueIDs) > 0 {
		if err := r.seenIssues.Add(ctx, newIssueIDs); err != nil {
			log.Error("seen issues save",
				zap.Int("count", len(newIssueIDs)),
				zap.Strings("ids", newIssueIDs),
				zap.Error(err))
		}
	}
	if len(newCommentIDs) > 0 {
		if err := r.seenComments.Add(ctx, newCommentIDs); err != nil {
			log.Error("seen comments save",
				zap.Int("count", len(newCommentIDs)),
				zap.Strings("ids", newCommentIDs),
				zap.Error(err))
		}
	}
}

func nonBotComments(in []Comment) []Comment {
	out := make([]Comment, 0, len(in))
	for _, c := range in {
		if strings.HasPrefix(c.Body, botCommentPrefix) {
			continue
		}
		out = append(out, c)
	}
	return out
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

// buildInitialInbound merges title, description, and the supplied non-bot
// comments into a single InboundMessage. The reply target is the issue itself
// (no parent comment), so the bee's reply lands at the top level of the issue.
func buildInitialInbound(issue Issue, comments []Comment) platform.InboundMessage {
	raw, _ := json.Marshal(replyTarget{IssueID: issue.ID})
	content := mergeIssueContent(issue, comments)
	createdAt := issue.CreatedAt
	if createdAt.IsZero() {
		log.Warn("linear: issue createdAt is zero, falling back to wall clock for MessageTime",
			zap.String("issue_id", issue.ID),
			zap.String("identifier", issue.Identifier),
		)
		createdAt = time.Now()
	}
	return platform.InboundMessage{
		Platform:          PlatformID,
		SenderID:          issue.Creator.ID,
		SessionKey:        buildSessionKey(issue.Team.Key, issue.Identifier),
		Content:           content,
		RawContent:        content,
		Raw:               string(raw),
		PlatformMessageID: "issue:" + issue.ID,
		MessageTime:       createdAt.UnixMilli(),
	}
}

// mergeIssueContent renders project header (if any), title, optional description,
// and the supplied non-bot comments into one body. Description is omitted when
// empty; the "Comments (N):" header is omitted when there are no non-bot comments.
func mergeIssueContent(issue Issue, comments []Comment) string {
	var b strings.Builder
	if issue.Project != nil {
		fmt.Fprintf(&b, "[Project: %s]\n\n", issue.Project.Name)
	}
	b.WriteString(issue.Title)
	if issue.Description != "" {
		b.WriteString("\n\n")
		b.WriteString(issue.Description)
	}
	if len(comments) > 0 {
		fmt.Fprintf(&b, "\n\n---\nComments (%d):\n", len(comments))
		for _, c := range comments {
			name := c.User.Name
			if name == "" {
				name = c.User.Email
			}
			if name == "" {
				name = c.User.ID
			}
			fmt.Fprintf(&b, "\n[%s]: %s", name, c.Body)
		}
	}
	return b.String()
}

func buildCommentInbound(issue Issue, c Comment) platform.InboundMessage {
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
