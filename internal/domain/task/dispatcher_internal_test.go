package task

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/theopenbee/openbee/internal/domain/delivery"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/platform"
)

func TestBuildInstruction_WithPlatformContext(t *testing.T) {
	platform.RegisterExtractor("testplatform", func(_ string) string {
		return `{"feishu":{"open_id":"ou_abc","chat_id":"oc_xyz"}}`
	})
	task := DispatchTask{
		TaskID:    "task-1",
		MessageID: "msg-1",
		ReplyTo: platform.InboundMessage{
			Platform: "testplatform",
			Raw:      "any-raw",
		},
		Instruction: "do something",
	}
	got := buildInstruction(task)

	assert.Contains(t, got, `"platform_context"`)
	assert.Contains(t, got, `"ou_abc"`)
	assert.Contains(t, got, "do something")
}

func TestBuildInstruction_NoPlatformContext(t *testing.T) {
	task := DispatchTask{
		TaskID:      "task-1",
		MessageID:   "msg-1",
		Instruction: "do something",
	}
	got := buildInstruction(task)

	assert.NotContains(t, got, `"platform_context"`, "platform_context should be omitted when empty")
}

func TestBuildInstruction_ProactiveTaskUsesSessionKey(t *testing.T) {
	for _, taskType := range []string{model.TaskTypeScheduled, model.TaskTypeCountdown} {
		t.Run(taskType, func(t *testing.T) {
			got := buildInstruction(DispatchTask{
				TaskID:      "task-1",
				MessageID:   "msg-1",
				SessionKey:  "feishu:oc_1:ou_1",
				TaskType:    taskType,
				Instruction: "report",
			})
			assert.Contains(t, got, `<task_meta>{"session_key":"feishu:oc_1:ou_1","task_id":"task-1"}</task_meta>`)
			assert.NotContains(t, got, "msg-1")
		})
	}
}

func TestBuildInstruction_ImmediateTaskUsesMessageID(t *testing.T) {
	got := buildInstruction(DispatchTask{
		TaskID:      "task-1",
		MessageID:   "msg-1",
		SessionKey:  "feishu:oc_1:ou_1",
		TaskType:    model.TaskTypeImmediate,
		Instruction: "report",
	})
	assert.Contains(t, got, `<task_meta>{"message_id":"msg-1","task_id":"task-1"}</task_meta>`)
	assert.NotContains(t, got, "session_key")
}

func TestDispatchTask_DeliveryTarget(t *testing.T) {
	cases := []struct {
		name string
		task DispatchTask
		want delivery.Target
	}{
		{"Immediate", DispatchTask{MessageID: "m", SessionKey: "s", TaskType: model.TaskTypeImmediate}, delivery.Target{MessageID: "m"}},
		{"Scheduled", DispatchTask{MessageID: "m", SessionKey: "s", TaskType: model.TaskTypeScheduled}, delivery.Target{SessionKey: "s"}},
		{"Countdown", DispatchTask{MessageID: "m", SessionKey: "s", TaskType: model.TaskTypeCountdown}, delivery.Target{SessionKey: "s"}},
		{"ScheduledWithoutSession", DispatchTask{MessageID: "m", TaskType: model.TaskTypeScheduled}, delivery.Target{MessageID: "m"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.task.DeliveryTarget())
		})
	}
}
