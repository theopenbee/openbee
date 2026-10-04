package task

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
