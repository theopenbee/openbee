package localchat

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/local"
)

var log = logger.With(zap.String("component", "localchat"))

type HandoffMessages interface {
	InsertProcessed(ctx context.Context, m store.ProcessedMsg) error
	DeleteByID(ctx context.Context, id string) error
}

type HandoffTasks interface {
	Create(ctx context.Context, t model.Task) (string, error)
}

type Handoff struct {
	msgs   HandoffMessages
	tasks  HandoffTasks
	sender platform.PlatformSenderAdapter
	pub    *local.EventPublisher
}

func NewHandoff(msgs HandoffMessages, tasks HandoffTasks, sender platform.PlatformSenderAdapter, pub *local.EventPublisher) *Handoff {
	return &Handoff{msgs: msgs, tasks: tasks, sender: sender, pub: pub}
}

func Applies(platformID, sessionKey string) bool {
	return platformID == local.PlatformID && local.IsBeeSessionKey(sessionKey)
}

func (h *Handoff) Create(ctx context.Context, t model.Task) (string, error) {
	userID, conversationID, ok := local.ParseSessionKey(t.SessionKey)
	if !ok || conversationID != local.BeeConversationID {
		return "", errors.New("handoff requires a local bee session key")
	}
	beeKey := t.SessionKey
	originMessageID := t.MessageID
	workerKey := local.WorkerSessionKey(userID, t.WorkerID)
	handoffID := uuid.New().String()
	now := time.Now().UnixMilli()

	if err := h.msgs.InsertProcessed(ctx, store.ProcessedMsg{
		ID:         handoffID,
		SessionKey: workerKey,
		Platform:   local.PlatformID,
		Content:    t.Instruction,
		SenderType: store.SenderTypeBee,
		ReceivedAt: now,
	}); err != nil {
		return "", err
	}

	t.MessageID = handoffID
	t.SessionKey = workerKey
	t.Platform = local.PlatformID
	taskID, err := h.tasks.Create(ctx, t)
	if err != nil {
		if delErr := h.msgs.DeleteByID(context.WithoutCancel(ctx), handoffID); delErr != nil {
			log.Error("handoff: roll back worker chat row", zap.String("messageID", handoffID), zap.Error(delErr))
		}
		return "", err
	}

	h.pub.Message(workerKey, local.ChatMessage{
		ID:      handoffID,
		Sender:  local.Sender{Type: local.SenderBee},
		Kind:    local.KindMessage,
		Content: t.Instruction,
		TS:      now,
	})

	h.sendCard(ctx, beeKey, originMessageID, taskID, t)
	return taskID, nil
}

func (h *Handoff) sendCard(ctx context.Context, beeKey, originMessageID, taskID string, t model.Task) {
	payload, err := json.Marshal(local.HandoffInfo{
		WorkerID:       t.WorkerID,
		TaskID:         taskID,
		TaskType:       t.Type,
		CronExpr:       t.CronExpr,
		ScheduledAt:    t.ScheduledAt,
		ConversationID: local.WorkerConversationID(t.WorkerID),
	})
	if err != nil {
		log.Error("handoff: encode card payload", zap.String("taskID", taskID), zap.Error(err))
		return
	}
	if err := h.sender.Send(ctx, platform.OutboundMessage{
		SessionKey:   beeKey,
		Content:      t.Instruction,
		SourceType:   store.SourceTypeBee,
		InboundMsgID: originMessageID,
		Kind:         store.OutboundKindHandoff,
		Payload:      string(payload),
	}); err != nil {
		log.Error("handoff: send card", zap.String("taskID", taskID), zap.Error(err))
	}
}
