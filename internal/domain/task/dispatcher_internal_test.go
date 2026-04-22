package task

import (
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/platform"
)

func TestBuildInstruction_WithPlatformContext(t *testing.T) {
	ctx := `{"feishu":{"open_id":"ou_abc","chat_id":"oc_xyz"}}`
	task := DispatchTask{
		TaskID:    "task-1",
		MessageID: "msg-1",
		ReplyTo: platform.InboundMessage{
			PlatformContext: ctx,
		},
		Instruction: "do something",
	}
	got := buildInstruction(task)

	if !strings.Contains(got, `"platform_context"`) {
		t.Errorf("expected platform_context in task_meta, got: %q", got)
	}
	if !strings.Contains(got, `"ou_abc"`) {
		t.Errorf("expected open_id value in task_meta, got: %q", got)
	}
	if !strings.Contains(got, "do something") {
		t.Errorf("expected instruction in output, got: %q", got)
	}
}

func TestBuildInstruction_NoPlatformContext(t *testing.T) {
	task := DispatchTask{
		TaskID:      "task-1",
		MessageID:   "msg-1",
		Instruction: "do something",
	}
	got := buildInstruction(task)

	if strings.Contains(got, `"platform_context"`) {
		t.Errorf("platform_context should be omitted when empty, got: %q", got)
	}
}
