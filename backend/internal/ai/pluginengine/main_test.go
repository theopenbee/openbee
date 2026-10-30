package pluginengine

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	fakeModeEnv = "OPENBEE_FAKE_ENGINE"
	fakeDirEnv  = "OPENBEE_FAKE_ENGINE_DIR"
)

type fakeRecord struct {
	Request  json.RawMessage   `json:"request"`
	Env      map[string]string `json:"env"`
	Cwd      string            `json:"cwd"`
	ChildPID int               `json:"child_pid,omitempty"`
}

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeModeEnv); mode != "" {
		os.Exit(runFakeEngine(mode))
	}
	os.Exit(m.Run())
}

func runFakeEngine(mode string) int {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read stdin:", err)
		return 2
	}
	var req struct {
		Method string `json:"method"`
		Params struct {
			SessionIDs []string `json:"session_ids"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		fmt.Fprintln(os.Stderr, "decode request:", err)
		return 2
	}
	rec := fakeRecord{Request: raw, Env: map[string]string{}}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "OPENBEE_") || strings.HasPrefix(k, "FAKE_") {
			rec.Env[k] = v
		}
	}
	rec.Cwd, _ = os.Getwd()

	switch mode {
	case "echo":
		writeRecord(rec)
		switch req.Method {
		case "check":
			respond(successResponse(req.Method))
		default:
			text, _ := json.Marshal(rec)
			respond(map[string]any{"result": map[string]any{"text": string(text), "error": ""}})
		}
	case "stray":
		writeRecord(rec)
		fmt.Fprintln(os.Stdout, "hello")
		fmt.Fprintln(os.Stdout, `{"foo":"bar"}`)
		fmt.Fprintln(os.Stderr, "fake stderr line")
		os.Stdout.WriteString(`{"result":{"text":"ok","error":""}}`)
	case "run-error":
		writeRecord(rec)
		respond(map[string]any{"result": map[string]any{"text": "partial reply", "error": "fakecli exited with code 2"}})
	case "check-fail":
		writeRecord(rec)
		respond(map[string]any{"error": map[string]any{"message": "fakecli not found on PATH"}})
	case "noresp":
		writeRecord(rec)
		fmt.Fprintln(os.Stderr, "starting")
		fmt.Fprintln(os.Stderr, "boom")
		return 1
	case "usage":
		writeRecord(rec)
		usage := map[string]any{}
		for i, id := range req.Params.SessionIDs {
			if strings.HasPrefix(id, "missing") {
				continue
			}
			usage[id] = []map[string]any{{
				"model":                 "model-" + id,
				"input_tokens":          i + 1,
				"output_tokens":         i + 2,
				"cache_creation_tokens": i + 3,
				"cache_read_tokens":     i + 4,
			}}
		}
		respond(map[string]any{"result": map[string]any{"usage": usage}})
	case "sleep-with-child":
		child, ok := spawnChild(&rec, nil)
		if !ok {
			return 2
		}
		_ = child.Wait()
		time.Sleep(5 * time.Minute)
	case "ok-exit1":
		writeRecord(rec)
		respond(successResponse(req.Method))
		return 1
	case "child-ok":
		if _, ok := spawnChild(&rec, nil); !ok {
			return 2
		}
		respond(successResponse(req.Method))
	case "child-noresp":
		if _, ok := spawnChild(&rec, nil); !ok {
			return 2
		}
		fmt.Fprintln(os.Stderr, "cli crashed")
		return 1
	case "child-error":
		if _, ok := spawnChild(&rec, nil); !ok {
			return 2
		}
		respond(failureResponse(req.Method))
	case "child-holds-stdout":
		if _, ok := spawnChild(&rec, os.Stdout); !ok {
			return 2
		}
	case "trap-term-child", "trap-term-child-exit1":
		if !spawnShellChild(&rec, fmt.Sprintf(`trap 'touch %s; exit 0' TERM; while :; do sleep 0.02; done`, termMarker())) {
			return 2
		}
		if mode == "trap-term-child-exit1" {
			return 1
		}
		time.Sleep(5 * time.Minute)
	case "ignore-term-child":
		if !spawnShellChild(&rec, `trap '' TERM; while :; do sleep 0.02; done`) {
			return 2
		}
		time.Sleep(5 * time.Minute)
	case "respond-then-block":
		respond(successResponse(req.Method))
		child, ok := spawnChild(&rec, nil)
		if !ok {
			return 2
		}
		_ = child.Wait()
		time.Sleep(5 * time.Minute)
	default:
		fmt.Fprintln(os.Stderr, "unknown fake mode:", mode)
		return 2
	}
	return 0
}

func spawnChild(rec *fakeRecord, stdout io.Writer) (*exec.Cmd, bool) {
	child := exec.Command("sleep", "300")
	child.Stdout = stdout
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start child:", err)
		return nil, false
	}
	rec.ChildPID = child.Process.Pid
	writeRecord(*rec)
	return child, true
}

func termMarker() string {
	return filepath.Join(os.Getenv(fakeDirEnv), "term-seen")
}

func spawnShellChild(rec *fakeRecord, script string) bool {
	child := exec.Command("sh", "-c", script)
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start child:", err)
		return false
	}
	time.Sleep(100 * time.Millisecond)
	rec.ChildPID = child.Process.Pid
	writeRecord(*rec)
	return true
}

func successResponse(method string) map[string]any {
	switch method {
	case "check":
		return map[string]any{"result": CheckResult{Version: "1.2.3", Path: "/opt/fake/bin/fakecli"}}
	case "usage":
		return map[string]any{"result": map[string]any{"usage": map[string]any{
			"s1": []map[string]any{{"model": "m1", "input_tokens": 7, "output_tokens": 8}},
		}}}
	}
	return map[string]any{"result": map[string]any{"text": "answer", "error": ""}}
}

func failureResponse(method string) map[string]any {
	if method == "run" {
		return map[string]any{"result": map[string]any{"text": "partial", "error": "cli failed"}}
	}
	return map[string]any{"error": map[string]any{"message": "cli failed"}}
}

func respond(v any) {
	b, _ := json.Marshal(v)
	os.Stdout.Write(append(b, '\n'))
}

func writeRecord(rec fakeRecord) {
	dir := os.Getenv(fakeDirEnv)
	if dir == "" {
		return
	}
	b, _ := json.Marshal(rec)
	name := filepath.Join(dir, "call-"+strconv.Itoa(os.Getpid())+".json")
	tmp := name + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, name)
	}
}
