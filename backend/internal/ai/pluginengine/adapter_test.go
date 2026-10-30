package pluginengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	ai "github.com/theopenbee/openbee/internal/ai"
)

type sentRequest struct {
	ProtocolVersion int             `json:"protocol_version"`
	Method          string          `json:"method"`
	OpenbeeVersion  string          `json:"openbee_version"`
	EngineID        string          `json:"engine_id"`
	Language        string          `json:"language"`
	Config          map[string]any  `json:"config"`
	DataDir         string          `json:"data_dir"`
	StateDir        string          `json:"state_dir"`
	Params          json.RawMessage `json:"params"`
}

func fakeSpec(t *testing.T, mode string) (Spec, string) {
	t.Helper()
	exe, err := filepath.Abs(os.Args[0])
	require.NoError(t, err)
	recordDir := t.TempDir()
	t.Setenv(fakeModeEnv, mode)
	t.Setenv(fakeDirEnv, recordDir)
	t.Setenv("GORACE", strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0"))
	return Spec{
		ID:             "fake",
		Dir:            t.TempDir(),
		Command:        []string{exe},
		Config:         map[string]any{"path": "/opt/fake/bin/fakecli", "model": "m1"},
		DataDir:        "/home/u/.openbee/plugins/data/fake",
		StateDir:       "/home/u/.openbee/.fake",
		Language:       "zh",
		OpenbeeVersion: "0.0.47",
	}, recordDir
}

func awaitOutput(t *testing.T, res ai.RunResult) ai.Output {
	t.Helper()
	var out ai.Output
	select {
	case o, ok := <-res.Output:
		require.True(t, ok, "output channel closed without an event")
		out = o
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for engine output")
	}
	select {
	case _, ok := <-res.Output:
		require.False(t, ok, "output channel yielded a second event")
	case <-time.After(10 * time.Second):
		t.Fatal("output channel was not closed")
	}
	return out
}

func readRecords(t *testing.T, dir string) []fakeRecord {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(dir, "call-*.json"))
	require.NoError(t, err)
	sort.Strings(names)
	recs := make([]fakeRecord, 0, len(names))
	for _, name := range names {
		b, err := os.ReadFile(name)
		require.NoError(t, err)
		var rec fakeRecord
		require.NoError(t, json.Unmarshal(b, &rec))
		recs = append(recs, rec)
	}
	return recs
}

func decodeRequest(t *testing.T, raw json.RawMessage) (sentRequest, map[string]any) {
	t.Helper()
	var req sentRequest
	require.NoError(t, json.Unmarshal(raw, &req))
	var params map[string]any
	require.NoError(t, json.Unmarshal(req.Params, &params))
	return req, params
}

func decodeEcho(t *testing.T, text string) fakeRecord {
	t.Helper()
	var rec fakeRecord
	require.NoError(t, json.Unmarshal([]byte(text), &rec))
	return rec
}

func sameDir(t *testing.T, want, got string) {
	t.Helper()
	w, err := filepath.EvalSymlinks(want)
	require.NoError(t, err)
	g, err := filepath.EvalSymlinks(got)
	require.NoError(t, err)
	assert.Equal(t, w, g)
}

func TestRun_SendsRequestFields(t *testing.T) {
	t.Run("with mcp", func(t *testing.T) {
		spec, _ := fakeSpec(t, "echo")
		t.Setenv("OPENBEE_URL", "http://127.0.0.1:9999/")
		t.Setenv(ai.APIKeyEnv, "outer-key")
		logPath := filepath.Join(t.TempDir(), "exec.log")

		res, err := New(spec).Run(context.Background(), "/home/u/work", "hello there", ai.RunOptions{
			SessionID: "sess-1",
			Resume:    true,
			Role:      ai.RoleWorker,
			APIKey:    "secret-key-123",
			ExtraEnv:  []string{"FAKE_EXTRA=from-worker"},
			ExtraArgs: []string{"--model", "opus"},
		}, logPath)
		require.NoError(t, err)
		require.NotNil(t, res.Process)
		assert.NotZero(t, res.Process.PID())
		assert.Equal(t, ai.Output{Type: ai.OutputDone}, awaitOutput(t, res))

		rec := decodeEcho(t, res.ExtractResult("ignored"))
		req, params := decodeRequest(t, rec.Request)
		assert.Equal(t, 1, req.ProtocolVersion)
		assert.Equal(t, "run", req.Method)
		assert.Equal(t, "0.0.47", req.OpenbeeVersion)
		assert.Equal(t, "fake", req.EngineID)
		assert.Equal(t, "zh", req.Language)
		assert.Equal(t, map[string]any{"path": "/opt/fake/bin/fakecli", "model": "m1"}, req.Config)
		assert.Equal(t, "/home/u/.openbee/plugins/data/fake", req.DataDir)
		assert.Equal(t, "/home/u/.openbee/.fake", req.StateDir)
		assert.Equal(t, map[string]any{
			"role":       "worker",
			"work_dir":   "/home/u/work",
			"prompt":     "hello there",
			"session_id": "sess-1",
			"resume":     true,
			"extra_args": []any{"--model", "opus"},
			"log_path":   logPath,
			"mcp": map[string]any{
				"name":        "openbee",
				"url":         "http://127.0.0.1:9999/mcp",
				"api_key_env": "OPENBEE_API_KEY",
			},
		}, params)

		assert.Equal(t, "secret-key-123", rec.Env[ai.APIKeyEnv])
		assert.Equal(t, "from-worker", rec.Env["FAKE_EXTRA"])
		assert.NotContains(t, string(rec.Request), "secret-key-123")
		assert.NotContains(t, string(rec.Request), "outer-key")
		sameDir(t, spec.Dir, rec.Cwd)

		_, statErr := os.Stat(logPath)
		assert.True(t, os.IsNotExist(statErr), "core must not create the log file")
	})

	t.Run("without mcp", func(t *testing.T) {
		spec, _ := fakeSpec(t, "echo")
		spec.Config = nil
		t.Setenv("OPENBEE_URL", "")

		res, err := New(spec).Run(context.Background(), "/home/u/bee", "p", ai.RunOptions{Role: ai.RoleBee}, "/nonexistent/dir/exec.log")
		require.NoError(t, err)
		assert.Equal(t, ai.Output{Type: ai.OutputDone}, awaitOutput(t, res))

		rec := decodeEcho(t, res.ExtractResult(""))
		req, params := decodeRequest(t, rec.Request)
		assert.Equal(t, map[string]any{}, req.Config)
		assert.NotContains(t, params, "mcp")
		assert.Equal(t, []any{}, params["extra_args"])
		assert.Equal(t, "bee", params["role"])
		assert.Equal(t, false, params["resume"])
		assert.Equal(t, "", params["session_id"])
	})
}

func TestRun_LargeUnicodePrompt(t *testing.T) {
	spec, _ := fakeSpec(t, "echo")
	t.Setenv("OPENBEE_URL", "")
	chunk := "line with \"quotes\", a backslash \\, <tags> & ampersands, tabs\tand emoji 🐝🚀 中文\n"
	var b strings.Builder
	for b.Len() < 3<<20 {
		b.WriteString(chunk)
	}
	prompt := b.String()

	res, err := New(spec).Run(context.Background(), t.TempDir(), prompt, ai.RunOptions{Role: ai.RoleWorker}, "")
	require.NoError(t, err)
	assert.Equal(t, ai.Output{Type: ai.OutputDone}, awaitOutput(t, res))

	rec := decodeEcho(t, res.ExtractResult(""))
	_, params := decodeRequest(t, rec.Request)
	got, ok := params["prompt"].(string)
	require.True(t, ok)
	assert.Equal(t, len(prompt), len(got))
	assert.Equal(t, sha256.Sum256([]byte(prompt)), sha256.Sum256([]byte(got)))
}

func TestRun_IgnoresStrayStdout(t *testing.T) {
	spec, _ := fakeSpec(t, "stray")
	core, logs := observer.New(zapcore.DebugLevel)
	a := New(spec)
	a.log = zap.New(core)

	res, err := a.Run(context.Background(), t.TempDir(), "p", ai.RunOptions{Role: ai.RoleWorker}, "")
	require.NoError(t, err)
	assert.Equal(t, ai.Output{Type: ai.OutputDone}, awaitOutput(t, res))
	assert.Equal(t, "ok", res.ExtractResult(""))

	var stray []string
	for _, e := range logs.FilterLevelExact(zapcore.WarnLevel).All() {
		stray = append(stray, e.ContextMap()["line"].(string))
	}
	assert.Equal(t, []string{"hello", `{"foo":"bar"}`}, stray)
	assert.Equal(t, 1, logs.FilterLevelExact(zapcore.InfoLevel).FilterMessage("fake stderr line").Len())
}

func TestRun_ErrorResponse(t *testing.T) {
	t.Run("result error", func(t *testing.T) {
		spec, _ := fakeSpec(t, "run-error")
		res, err := New(spec).Run(context.Background(), t.TempDir(), "p", ai.RunOptions{Role: ai.RoleWorker}, "")
		require.NoError(t, err)
		assert.Equal(t, ai.Output{Type: ai.OutputError, Content: "fakecli exited with code 2"}, awaitOutput(t, res))
		assert.Equal(t, "partial reply", res.ExtractResult(""))
	})

	t.Run("protocol error", func(t *testing.T) {
		spec, _ := fakeSpec(t, "check-fail")
		res, err := New(spec).Run(context.Background(), t.TempDir(), "p", ai.RunOptions{Role: ai.RoleWorker}, "")
		require.NoError(t, err)
		assert.Equal(t, ai.Output{Type: ai.OutputError, Content: "fakecli not found on PATH"}, awaitOutput(t, res))
		assert.Equal(t, "", res.ExtractResult(""))
	})
}

func TestRun_NoResponse(t *testing.T) {
	spec, _ := fakeSpec(t, "noresp")
	res, err := New(spec).Run(context.Background(), t.TempDir(), "p", ai.RunOptions{Role: ai.RoleWorker}, "")
	require.NoError(t, err)
	out := awaitOutput(t, res)
	assert.Equal(t, ai.OutputError, out.Type)
	assert.Contains(t, out.Content, "boom")
	assert.Equal(t, "engine plugin exited without a response: boom", out.Content)
	assert.Equal(t, "", res.ExtractResult(""))
}

func observedAdapter(t *testing.T, mode string) (*Adapter, *observer.ObservedLogs) {
	t.Helper()
	spec, _ := fakeSpec(t, mode)
	core, logs := observer.New(zapcore.DebugLevel)
	a := New(spec)
	a.log = zap.New(core)
	return a, logs
}

func assertExitLogged(t *testing.T, logs *observer.ObservedLogs, code int64) {
	t.Helper()
	entries := logs.FilterLevelExact(zapcore.WarnLevel).FilterMessage("engine plugin exited with a non-zero status after responding").All()
	require.Len(t, entries, 1)
	assert.Equal(t, code, entries[0].ContextMap()["exit_code"])
}

func TestRun_ResponseWinsOverNonZeroExit(t *testing.T) {
	a, logs := observedAdapter(t, "ok-exit1")
	res, err := a.Run(context.Background(), t.TempDir(), "p", ai.RunOptions{Role: ai.RoleWorker}, "")
	require.NoError(t, err)
	assert.Equal(t, ai.Output{Type: ai.OutputDone}, awaitOutput(t, res))
	assert.Equal(t, "answer", res.ExtractResult(""))
	assertExitLogged(t, logs, 1)
}

func TestCheck_ResponseWinsOverNonZeroExit(t *testing.T) {
	a, logs := observedAdapter(t, "ok-exit1")
	got, err := a.Check(context.Background())
	require.NoError(t, err)
	assert.Equal(t, CheckResult{Version: "1.2.3", Path: "/opt/fake/bin/fakecli"}, got)
	assertExitLogged(t, logs, 1)
}

func TestUsage_ResponseWinsOverNonZeroExit(t *testing.T) {
	a, logs := observedAdapter(t, "ok-exit1")
	got, err := a.CollectTokenUsage(context.Background(), []string{"s1"})
	require.NoError(t, err)
	assert.Equal(t, map[string][]ai.TokenUsage{"s1": {{Model: "m1", InputTokens: 7, OutputTokens: 8}}}, got)
	assertExitLogged(t, logs, 1)
}

func TestRun_MissingCommandFails(t *testing.T) {
	spec, _ := fakeSpec(t, "echo")
	spec.Command = []string{filepath.Join(t.TempDir(), "missing-engine")}
	_, err := New(spec).Run(context.Background(), t.TempDir(), "p", ai.RunOptions{Role: ai.RoleWorker}, "")
	require.Error(t, err)
}

func TestUsage_Batch(t *testing.T) {
	spec, recordDir := fakeSpec(t, "usage")
	t.Setenv(ai.APIKeyEnv, "outer-key")

	got, err := New(spec).CollectTokenUsage(context.Background(), []string{"s1", "s2", "missing-1"})
	require.NoError(t, err)
	assert.Equal(t, map[string][]ai.TokenUsage{
		"s1": {{Model: "model-s1", InputTokens: 1, OutputTokens: 2, CacheCreationTokens: 3, CacheReadTokens: 4}},
		"s2": {{Model: "model-s2", InputTokens: 2, OutputTokens: 3, CacheCreationTokens: 4, CacheReadTokens: 5}},
	}, got)

	recs := readRecords(t, recordDir)
	require.Len(t, recs, 1)
	req, params := decodeRequest(t, recs[0].Request)
	assert.Equal(t, "usage", req.Method)
	assert.Equal(t, map[string]any{"session_ids": []any{"s1", "s2", "missing-1"}}, params)
	assert.NotContains(t, recs[0].Env, ai.APIKeyEnv)
}

func TestUsage_EmptyIDsNoProcess(t *testing.T) {
	spec, recordDir := fakeSpec(t, "usage")
	a := New(spec)

	for _, ids := range [][]string{nil, {}} {
		got, err := a.CollectTokenUsage(context.Background(), ids)
		require.NoError(t, err)
		assert.Empty(t, got)
	}
	assert.Empty(t, readRecords(t, recordDir))
}

func TestCheck_OKAndError(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		spec, recordDir := fakeSpec(t, "echo")
		t.Setenv(ai.APIKeyEnv, "outer-key")

		got, err := New(spec).Check(context.Background())
		require.NoError(t, err)
		assert.Equal(t, CheckResult{Version: "1.2.3", Path: "/opt/fake/bin/fakecli"}, got)

		recs := readRecords(t, recordDir)
		require.Len(t, recs, 1)
		req, params := decodeRequest(t, recs[0].Request)
		assert.Equal(t, "check", req.Method)
		assert.Equal(t, map[string]any{}, params)
		assert.NotContains(t, recs[0].Env, ai.APIKeyEnv)
		sameDir(t, spec.Dir, recs[0].Cwd)
	})

	t.Run("error", func(t *testing.T) {
		spec, _ := fakeSpec(t, "check-fail")
		_, err := New(spec).Check(context.Background())
		require.Error(t, err)
		assert.Equal(t, "fakecli not found on PATH", err.Error())
	})
}

func TestLineWriter_DropsOverlongLine(t *testing.T) {
	var lines []string
	overflows := 0
	w := &lineWriter{
		max:        8,
		onLine:     func(b []byte) { lines = append(lines, string(b)) },
		onOverflow: func() { overflows++ },
	}
	for _, chunk := range []string{"short\r\n", "far too ", "long line\n", "\n", "ok", "\nta", "il"} {
		n, err := w.Write([]byte(chunk))
		require.NoError(t, err)
		assert.Equal(t, len(chunk), n)
	}
	w.flush()
	assert.Equal(t, []string{"short", "ok", "tail"}, lines)
	assert.Equal(t, 1, overflows)
}

func TestParseResponse(t *testing.T) {
	cases := []struct {
		line    string
		ok      bool
		result  string
		errText string
	}{
		{`{"result":{"text":"hi"}}`, true, `{"text":"hi"}`, ""},
		{`{"error":{"message":"bad"}}`, true, "", "bad"},
		{`{"error":"plain"}`, true, "", "plain"},
		{`{"result":null,"error":null}`, true, "null", ""},
		{`{"foo":1}`, false, "", ""},
		{`hello`, false, "", ""},
		{`[1,2]`, false, "", ""},
	}
	for _, c := range cases {
		resp, ok := parseResponse([]byte(c.line))
		require.Equal(t, c.ok, ok, c.line)
		if !ok {
			continue
		}
		assert.Equal(t, c.result, string(bytes.TrimSpace(resp.Result)), c.line)
		assert.Equal(t, c.errText, resp.Err, c.line)
	}
}

func TestStart_CancelStopsTheProcessTree(t *testing.T) {
	spec, _ := fakeSpec(t, "echo")
	a := New(spec)
	p, err := a.start(context.Background(), methodCheck, struct{}{}, func(runtimeEnv []string) []string {
		return append(ai.BuildBaseEnv(), runtimeEnv...)
	})
	require.NoError(t, err)
	defer p.finish()
	require.NoError(t, p.wait())
	assert.NotNil(t, p.cmd.Cancel, "cancelling the context must stop the plugin's whole process tree on every platform")
}
