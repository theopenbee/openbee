package codemode

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	MaybeRunChild()
	os.Exit(m.Run())
}

func noCalls(context.Context, string, json.RawMessage) (any, error) {
	return nil, errors.New("no calls expected")
}

func TestRunFormatsResultAndConsole(t *testing.T) {
	out, err := Run(context.Background(), `
console.log("a", { b: 1 }, [2]);
return { ok: true };`, nil, noCalls)
	require.NoError(t, err)
	assert.Equal(t, "{\n  \"ok\": true\n}\n\nconsole:\na {\n  \"b\": 1\n} [\n  2\n]", out)
}

func TestRunReturnsNothing(t *testing.T) {
	out, err := Run(context.Background(), `const x = 1;`, nil, noCalls)
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestRunCallsMethods(t *testing.T) {
	var got []string
	call := func(_ context.Context, name string, args json.RawMessage) (any, error) {
		got = append(got, name+" "+string(args))
		if name == "fail" {
			return nil, errors.New("boom")
		}
		return map[string]any{"items": []string{"a", "b"}}, nil
	}
	out, err := Run(context.Background(), `
const { items } = await openbee.list({ page: 2 });
let caught = "";
try {
  await openbee.fail();
} catch (e) {
  caught = e.message;
}
return { items, caught, missing: typeof openbee.other };`, []string{"list", "fail"}, call)
	require.NoError(t, err)
	assert.JSONEq(t, `{"items":["a","b"],"caught":"boom","missing":"undefined"}`, out)
	assert.Equal(t, []string{`list {"page":2}`, "fail {}"}, got)
}

func TestRunReportsRejection(t *testing.T) {
	_, err := Run(context.Background(), `throw new Error("bad input");`, nil, noCalls)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Error: bad input")
}

func TestRunReportsSyntaxError(t *testing.T) {
	_, err := Run(context.Background(), `const x: string = "a";`, nil, noCalls)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SyntaxError")
}

func TestRunStopsAtDeadline(t *testing.T) {
	start := time.Now()
	_, err := run(context.Background(), `while (true) {}`, nil, noCalls, 200*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Less(t, time.Since(start), 10*time.Second)
}

func TestRunNeverSettledPromise(t *testing.T) {
	_, err := Run(context.Background(), `await new Promise(() => {});`, nil, noCalls)
	require.Error(t, err)
	assert.Equal(t, errPending.Error(), err.Error())
}

func TestRunTruncatesLongOutput(t *testing.T) {
	out, err := Run(context.Background(), `return "é".repeat(100000);`, nil, noCalls)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(out), maxOutput+100)
	assert.True(t, strings.HasSuffix(out, "[output truncated: return less data]"))
}

func TestRunHasNoHostAccess(t *testing.T) {
	out, err := Run(context.Background(), `return [typeof require, typeof fetch, typeof setTimeout, typeof process].join(",");`, nil, noCalls)
	require.NoError(t, err)
	assert.Equal(t, "undefined,undefined,undefined,undefined", out)
}

func TestRunLimitsRecursion(t *testing.T) {
	_, err := Run(context.Background(), `const f = (n) => f(n + 1); f(0);`, nil, noCalls)
	require.Error(t, err)
	assert.Equal(t, errCallDepth.Error(), err.Error())
}

func TestRunLimitsMemory(t *testing.T) {
	old := memoryLimit
	memoryLimit = 64 << 20
	t.Cleanup(func() { memoryLimit = old })
	_, err := Run(context.Background(), `const a = []; while (true) a.push("x".repeat(1 << 20) + a.length);`, nil, noCalls)
	require.Error(t, err)
	assert.Equal(t, errMemory.Error(), err.Error())
}

func TestRunSurvivesRunnerCrash(t *testing.T) {
	_, err := Run(context.Background(), `const a = [1]; a.push(a); return a.flat(Infinity);`, nil, noCalls)
	require.Error(t, err)
	assert.Contains(t, err.Error(), crashedError)

	out, err := Run(context.Background(), `return 1 + 1;`, nil, noCalls)
	require.NoError(t, err)
	assert.Equal(t, "2", out)
}

func TestServeRejectsUnlistedMethod(t *testing.T) {
	rep := serve(context.Background(), []string{"list"}, noCalls, &childCall{Name: "delete"})
	assert.Equal(t, "unknown method: delete", rep.Error)
}

func TestCrashReason(t *testing.T) {
	assert.Equal(t, "fatal error: stack overflow", crashReason("runtime: goroutine stack exceeds 67108864-byte limit\nfatal error: stack overflow\n\nstack:\n"))
	assert.Equal(t, "boom", crashReason("\nboom\nmore"))
	assert.Empty(t, crashReason(""))
}
