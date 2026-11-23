package ai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTakeModelArg(t *testing.T) {
	cases := []struct {
		raw, name, rest string
		ok              bool
	}{
		{"--model opus", "opus", "", true},
		{"--effort high --model opus --verbose", "opus", "--effort high --verbose", true},
		{"--model=deepseek/deepseek-flash", "deepseek/deepseek-flash", "", true},
		{"--model a --model=b", "b", "", true},
		{`--model "opus" --append-system-prompt "be brief" --name 'it'"'"'s'`, "opus", `--append-system-prompt 'be brief' --name 'it'\''s'`, true},
		{"-m o3", "", "", false},
		{"--model", "", "", false},
		{"--model=", "", "", false},
		{"--models opus", "", "", false},
		{`--model "opus`, "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		name, rest, ok := TakeModelArg(tc.raw)
		assert.Equal(t, tc.ok, ok, tc.raw)
		assert.Equal(t, tc.name, name, tc.raw)
		assert.Equal(t, tc.rest, rest, tc.raw)
	}
}

func TestJoinCLIArgsRoundTrips(t *testing.T) {
	args := []string{"--x", "", "two words", `quo"te`, "it's", `back\slash`, "tab\there", "多字节"}
	parsed, err := splitCLIArgs(JoinCLIArgs(args))
	require.NoError(t, err)
	assert.Equal(t, args, parsed)
}

func TestMoveModelArgs(t *testing.T) {
	args := map[string]string{
		"claude":   "--model opus",
		"pi":       "--model deepseek/deepseek-flash --thinking high",
		"codex":    "--model o3",
		"opencode": "--model " + strings.Repeat("m", MaxModelLength+1),
		"gemini":   "--sandbox",
	}
	models := map[string]string{"codex": "o4-mini", "gemini": ""}

	assert.True(t, MoveModelArgs(args, models))
	assert.Equal(t, map[string]string{
		"pi":       "--thinking high",
		"codex":    "--model o3",
		"opencode": "--model " + strings.Repeat("m", MaxModelLength+1),
		"gemini":   "--sandbox",
	}, args)
	assert.Equal(t, map[string]string{"claude": "opus", "pi": "deepseek/deepseek-flash", "codex": "o4-mini", "gemini": ""}, models)

	assert.False(t, MoveModelArgs(map[string]string{"pi": "--thinking high"}, map[string]string{}))
}

func TestResolveEngineModel(t *testing.T) {
	own := map[string]string{"pi": "own"}
	global := map[string]string{"pi": "global", "claude": "opus"}
	assert.Equal(t, "own", ResolveEngineModel("pi", own, global))
	assert.Equal(t, "opus", ResolveEngineModel("claude", own, global))
	assert.Equal(t, "", ResolveEngineModel("codex", own, global))
	assert.Equal(t, "", ResolveEngineModel("pi"))
}

func TestParseEngineModelsJSON(t *testing.T) {
	assert.Equal(t, map[string]string{"pi": "x"}, ParseEngineModelsJSON(`{"pi":" x ","claude":"","":"y"}`))
	assert.Nil(t, ParseEngineModelsJSON(""))
	assert.Nil(t, ParseEngineModelsJSON("{"))
}
