package task

import (
	"github.com/theopenbee/openbee/internal/domain/delivery"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

// DispatchTask is the unit of work sent to the TaskDispatcher by the TaskScheduler.
type DispatchTask struct {
	TaskID      string // task record ID
	WorkerID    string
	SessionKey  string
	Instruction string
	ReplyTo     platform.InboundMessage // platform info for result delivery
	TaskType    string                  // "immediate"|"countdown"|"scheduled"
	MessageID   string                  // originating platform_messages.id (for session lookup)
}

func (t DispatchTask) Proactive() bool {
	return t.SessionKey != "" && (t.TaskType == model.TaskTypeScheduled || t.TaskType == model.TaskTypeCountdown)
}

func (t DispatchTask) DeliveryTarget() delivery.Target {
	if t.Proactive() {
		return delivery.Target{SessionKey: t.SessionKey}
	}
	return delivery.Target{MessageID: t.MessageID}
}
