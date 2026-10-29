package pi

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestInvokerRunLoadsMCPExtension(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the pi binary")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OPENBEE_URL", "http://127.0.0.1:9000/")
	out := filepath.Join(t.TempDir(), "out")
	bin := filepath.Join(t.TempDir(), "pi")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\necho \"$@\" > "+out+"\necho \"$OPENBEE_MCP_URL $OPENBEE_API_KEY\" >> "+out+"\n"), 0o755))

	inv, err := NewInvoker(bin, nil)
	require.NoError(t, err)
	ext, err := os.ReadFile(filepath.Join(home, ".openbee", ".pi", "openbee-mcp.js"))
	require.NoError(t, err)
	assert.Contains(t, string(ext), "registerMcpServer")

	_, ch, err := inv.Run(context.Background(), t.TempDir(), "hello",
		ai.RunOptions{SessionID: "4d0ce91b-0856-44e2-b0d7-7765d824bba3", APIKey: "tok", ExtraArgs: []string{"--model", "x"}},
		filepath.Join(t.TempDir(), "pi.log"))
	require.NoError(t, err)
	for range ch {
	}

	got, err := os.ReadFile(out)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(got)), "\n")
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "-e "+filepath.Join(home, ".openbee", ".pi", "openbee-mcp.js")+" --model x")
	assert.Equal(t, "http://127.0.0.1:9000/mcp tok", lines[1])
}
