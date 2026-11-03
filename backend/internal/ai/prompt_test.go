package ai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkerPersona_Full(t *testing.T) {
	got := WorkerPersona("mybot", "does things", "remember X")
	assert.Contains(t, got, "## Role\n")
	assert.Contains(t, got, "You are a Worker in an AI team.")
	assert.Contains(t, got, "## Identity\n")
	assert.Contains(t, got, "Name: mybot")
	assert.Contains(t, got, "Description: does things")
	assert.Contains(t, got, "## Work Constraints")
	assert.Contains(t, got, "remember X")
	assert.NotContains(t, got, "openbee-worker", "persona must NOT contain skill rule directive")
}

func TestWorkerPersona_Empty(t *testing.T) {
	got := WorkerPersona("", "", "")
	assert.Equal(t, "## Role\nYou are a Worker in an AI team.\n", got)
}

func TestBuildWorkerSessionPrefix_WithPersona(t *testing.T) {
	persona := WorkerPersona("貂蝉", "负责 openbee 开发", "称呼老板")
	got := BuildWorkerSessionPrefix(persona)

	wants := []string{
		"## Step 1: Initialize your role",
		"[MANDATORY] You MUST invoke the openbee-worker skill immediately, before producing any other output.",
		"<worker_persona>",
		"Name: 貂蝉",
		"Description: 负责 openbee 开发",
		"称呼老板",
		"</worker_persona>",
	}
	for _, w := range wants {
		assert.Contains(t, got, w)
	}
	assert.True(t, strings.HasSuffix(got, "## Step 2: Execute the task\n"))
	assert.Less(t, strings.Index(got, "</worker_persona>"), strings.Index(got, "## Step 2:"), "persona block must precede Step 2")
}

func TestBuildWorkerSessionPrefix_NoPersona(t *testing.T) {
	got := BuildWorkerSessionPrefix("")

	assert.NotContains(t, got, "<worker_persona>", "persona is empty, expected no persona block")
	assert.Contains(t, got, "## Step 1: Initialize your role")
	assert.True(t, strings.HasSuffix(got, "## Step 2: Execute the task\n"))
}

func TestBuildBeeSessionPrefix(t *testing.T) {
	got := BuildBeeSessionPrefix()

	assert.Contains(t, got, "openbee-bee")
	assert.NotContains(t, got, "<worker_persona>", "bee prefix must not include persona")
	assert.True(t, strings.HasSuffix(got, "## Step 2: Handle the messages below\n"))
}
