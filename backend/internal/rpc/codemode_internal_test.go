package rpc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunScriptStopsAtDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := (&Server{}).runScript(ctx, caller{}, `while (true) {}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestRunScriptNeverSettledPromise(t *testing.T) {
	_, err := (&Server{}).runScript(context.Background(), caller{}, `await new Promise(() => {});`)
	assert.ErrorIs(t, err, errScriptPending)
}

func TestRunScriptFormatsResultAndConsole(t *testing.T) {
	out, err := (&Server{}).runScript(context.Background(), caller{}, `
console.log("a", { b: 1 }, [2]);
return { ok: true };`)
	require.NoError(t, err)
	assert.Equal(t, "{\n  \"ok\": true\n}\n\nconsole:\na {\n  \"b\": 1\n} [\n  2\n]", out)
}

func TestRunScriptReturnsNothing(t *testing.T) {
	out, err := (&Server{}).runScript(context.Background(), caller{}, `const x = 1;`)
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestRunScriptTruncatesLongOutput(t *testing.T) {
	out, err := (&Server{}).runScript(context.Background(), caller{}, `return "é".repeat(100000);`)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(out), maxScriptOutput+100)
	assert.True(t, strings.HasSuffix(out, "[output truncated: return less data]"))
}

func TestRunScriptHasNoHostAccess(t *testing.T) {
	out, err := (&Server{}).runScript(context.Background(), caller{}, `return [typeof require, typeof fetch, typeof setTimeout, typeof process].join(",");`)
	require.NoError(t, err)
	assert.Equal(t, "undefined,undefined,undefined,undefined", out)
}

func TestWorkerDeclarationListsOnlyWorkerTools(t *testing.T) {
	decl := caller{workerID: "w"}.declaration()
	assert.Contains(t, decl, "send_message(")
	assert.Contains(t, decl, "list_outbound_messages(")
	assert.NotContains(t, decl, "delete_worker(")
	assert.Contains(t, caller{}.declaration(), "delete_worker(")
}

func TestRunScriptLimitsRecursion(t *testing.T) {
	_, err := (&Server{}).runScript(context.Background(), caller{}, `const f = (n) => f(n + 1); f(0);`)
	require.Error(t, err)
}

func TestRunScriptLimitsMemory(t *testing.T) {
	old := scriptMemoryLimit
	scriptMemoryLimit = 64 << 20
	t.Cleanup(func() { scriptMemoryLimit = old })
	_, err := (&Server{}).runScript(context.Background(), caller{}, `const a = []; while (true) a.push("x".repeat(1 << 20) + a.length);`)
	assert.ErrorIs(t, err, errScriptMemory)
}
