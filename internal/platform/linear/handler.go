package linear

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/media"
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

// reactionEmoji is the shortcode used to acknowledge inbound dispatches; it
// is removed by the sender after the reply comment is posted.
const reactionEmoji = ":eyes:"

// reactionCleanupTTL bounds how long an unresolved pendingReactions entry
// lingers before being swept (memory-leak guard).
const reactionCleanupTTL = 10 * time.Minute

var log = logger.With(zap.String("component", "linear"))

// LinearPlatform implements platform.Platform.
type LinearPlatform struct {
	receiver *LinearReceiver
	sender   *LinearSender
}

// NewPlatform constructs a Linear platform from configuration. Persistent
// state (seen_issues.ndjson, seen_comments.ndjson) lives in ~/.openbee/.linear/.
func NewPlatform(cfg config.LinearConfig, mediaSvc *media.Service) (platform.Platform, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("linear: resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".openbee", ".linear")
	client := NewClient(cfg.APIKey)
	maxSize := cfg.MaxMediaSize
	if maxSize == 0 {
		maxSize = 50 * 1024 * 1024
	}
	pending := &sync.Map{}
	return &LinearPlatform{
		receiver: &LinearReceiver{
			client:       client,
			seenIssues:   NewSeenSet(dir, "seen_issues.ndjson"),
			seenComments: NewSeenSet(dir, "seen_comments.ndjson"),
			labelName:    cfg.LabelName,
			pollInterval: cfg.PollInterval,
			projectsList: cleanStringList(cfg.Projects),
			statesList:   cleanStringList(cfg.States),
			resolver: &resolver{
				client:  client,
				media:   mediaSvc,
				maxSize: maxSize,
			},
			pendingReactions: pending,
		},
		sender: &LinearSender{
			client: client,
			uploader: &uploader{
				client:  client,
				maxSize: maxSize,
				http:    &http.Client{Timeout: uploadPutTimeout + 30*time.Second},
			},
			pendingReactions: pending,
		},
	}, nil
}

func (p *LinearPlatform) ID() string                                 { return PlatformID }
func (p *LinearPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *LinearPlatform) Sender() platform.PlatformSenderAdapter     { return p.sender }

// LinearReceiver polls Linear for issue/comment updates by workflow-state.
type LinearReceiver struct {
	client           Client
	seenIssues       seenAPI
	seenComments     seenAPI
	labelName        string
	pollInterval     time.Duration
	projectsList     []string
	statesList       []string
	resolver         *resolver
	pendingReactions *sync.Map
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
		log.Warn("linear receiver viewer check failed; continuing polling",
			zap.String("label", r.labelName),
			zap.Error(err),
		)
	} else {
		log.Info("linear receiver started",
			zap.String("viewer_id", viewer.ID),
			zap.String("label", r.labelName),
		)
	}

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

// addReaction asynchronously creates a reaction on target and stores the
// resulting ID in pendingReactions under key. A buffered channel coordinates
// with sender's LoadAndDelete so the sender can wait for the ID even when
// the API call has not yet returned.
func (r *LinearReceiver) addReaction(ctx context.Context, key string, target ReactionTarget) {
	if r.pendingReactions == nil {
		return
	}
	ch := make(chan string, 1)
	r.pendingReactions.Store(key, ch)
	go func() {
		defer time.AfterFunc(reactionCleanupTTL, func() {
			r.pendingReactions.Delete(key)
		})
		var reactionID string
		err := utils.RetryWithBackoff(ctx, func() error {
			id, e := r.client.CreateReaction(ctx, target, reactionEmoji)
			if e != nil {
				return e
			}
			reactionID = id
			return nil
		}, 1, utils.DefaultRetryDelay)
		if err != nil {
			log.Warn("linear: add reaction failed", zap.String("key", key), zap.Error(err))
			close(ch)
			return
		}
		if reactionID == "" {
			close(ch)
			return
		}
		ch <- reactionID
	}()
}

// removeReaction is invoked by the sender after a reply has been posted. It
// looks up any pending reaction stored under key, waits up to 5s for the
// reactionID, and fires DeleteReaction in a background goroutine. Failures
// are logged and never propagated to the caller.
func (s *LinearSender) removeReaction(key string) {
	if s.pendingReactions == nil {
		return
	}
	val, ok := s.pendingReactions.LoadAndDelete(key)
	if !ok {
		return
	}
	ch, ok := val.(chan string)
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case reactionID, received := <-ch:
			if !received || reactionID == "" {
				return
			}
			if err := utils.RetryWithBackoff(ctx, func() error {
				return s.client.DeleteReaction(ctx, reactionID)
			}, utils.DefaultRetryCount, utils.DefaultRetryDelay); err != nil {
				log.Warn("linear: remove reaction failed", zap.String("key", key), zap.Error(err))
			}
		case <-timer.C:
			log.Warn("linear: timed out waiting for reaction ID", zap.String("key", key))
		}
	}()
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
			r.addReaction(ctx, "issue:"+issue.ID, ReactionTarget{IssueID: issue.ID})
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
			r.addReaction(ctx, "comment:"+c.ID, ReactionTarget{CommentID: c.ID})
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
	client           Client
	uploader         *uploader
	pendingReactions *sync.Map
}

func (s *LinearSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	var target replyTarget
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &target); err != nil {
		return fmt.Errorf("linear: parse reply target: %w", err)
	}
	if target.IssueID == "" {
		return errors.New("linear: reply target missing issue_id")
	}

	body := selfMarker + msg.Content
	if msg.MediaPath != "" {
		md, err := s.uploader.Upload(ctx, msg.MediaPath)
		if err != nil {
			return fmt.Errorf("linear: upload attachment: %w", err)
		}
		if msg.Content != "" {
			body = body + "\n\n" + md
		} else {
			body = selfMarker + md
		}
	}

	if err := utils.RetryWithBackoff(ctx, func() error {
		_, err := s.client.CreateComment(ctx, target.IssueID, body, target.ParentCommentID)
		return err
	}, utils.DefaultRetryCount, utils.DefaultRetryDelay); err != nil {
		return err
	}
	s.removeReaction(msg.ReplyTo.PlatformMessageID)
	return nil
}

var _ platform.Platform = (*LinearPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*LinearReceiver)(nil)
var _ platform.PlatformSenderAdapter = (*LinearSender)(nil)
