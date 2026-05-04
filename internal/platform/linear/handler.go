package linear

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

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

// selfComments tracks comment IDs the bot has posted so the receiver can skip
// them on the next poll. The set is persisted to <dir>/self_comments.log
// (one ID per line, append-only) so a restart does not cause the bot to
// re-process its own replies.
type selfComments struct {
	mu  sync.Mutex
	set map[string]struct{}
	f   *os.File
}

func newSelfComments(dir string) (*selfComments, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("self_comments: mkdir: %w", err)
	}
	path := filepath.Join(dir, "self_comments.log")
	set := make(map[string]struct{})
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if line != "" {
				set[line] = struct{}{}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("self_comments: read %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("self_comments: open %s: %w", path, err)
	}
	return &selfComments{set: set, f: f}, nil
}

func (s *selfComments) Add(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.set[id]; ok {
		return
	}
	if _, err := s.f.Write([]byte(id + "\n")); err != nil {
		// Log and skip — the in-memory set isn't updated either, so we'll
		// retry persisting if the same ID comes back through a later Send.
		log.Error("self_comments: write", zap.Error(err), zap.String("id", id))
		return
	}
	s.set[id] = struct{}{}
}

func (s *selfComments) Has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.set[id]
	return ok
}

// LinearPlatform implements platform.Platform.
type LinearPlatform struct {
	receiver *LinearReceiver
	sender   *LinearSender
}

// NewPlatform constructs a Linear platform from configuration. Persistent
// state lives in ~/.openbee/.linear/.
func NewPlatform(cfg config.LinearConfig) (platform.Platform, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("linear: resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".openbee", ".linear")
	client := NewClient(cfg.APIKey)
	self, err := newSelfComments(dir)
	if err != nil {
		return nil, err
	}
	return &LinearPlatform{
		receiver: &LinearReceiver{
			client:       client,
			cursor:       NewCursor(dir),
			labelName:    cfg.LabelName,
			pollInterval: cfg.PollInterval,
			self:         self,
		},
		sender: &LinearSender{client: client, self: self},
	}, nil
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
	self         *selfComments
}

// Start runs the polling loop until ctx is cancelled.
func (r *LinearReceiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
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

func (r *LinearReceiver) tickOnce(ctx context.Context, dispatch func(platform.InboundMessage)) {
	since, err := r.cursor.Load(ctx)
	if err != nil {
		log.Error("cursor load", zap.Error(err))
		return
	}
	issues, truncated, err := r.client.IssuesUpdatedSince(ctx, since, r.labelName)
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
			if strings.HasPrefix(c.Body, "[openbee-bot]") {
				continue
			}
			if r.self.Has(c.ID) {
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
	// Don't advance the cursor when the page is truncated: we don't know what
	// lies past the page boundary, and advancing would skip it permanently.
	if truncated {
		return
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
	self   *selfComments
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
		c, err := s.client.CreateComment(ctx, target.IssueID, selfMarker+msg.Content, target.ParentCommentID)
		if err != nil {
			return err
		}
		if s.self != nil {
			s.self.Add(c.ID)
		}
		return nil
	}, utils.DefaultRetryCount, utils.DefaultRetryDelay)
}

var _ platform.Platform = (*LinearPlatform)(nil)
var _ platform.PlatformReceiverAdapter = (*LinearReceiver)(nil)
var _ platform.PlatformSenderAdapter = (*LinearSender)(nil)
