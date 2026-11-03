package pluginchannel

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("OPENBEE_FAKE_PLUGIN") != "" {
		fakePluginMain()
		os.Exit(0)
	}
	if os.Getenv("OPENBEE_FAKE_HOST") != "" {
		fakeHostMain()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakePluginCommand() []string {
	return []string{os.Args[0], "-test.run=^$"}
}

func fakePluginEnv(mode string, extra ...string) []string {
	return append([]string{"OPENBEE_FAKE_PLUGIN=1", "FAKE_PLUGIN_MODE=" + mode, "GORACE=atexit_sleep_ms=0"}, extra...)
}

type fakeMsg struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

func fakePluginMain() {
	mode := os.Getenv("FAKE_PLUGIN_MODE")
	if f := os.Getenv("FAKE_PLUGIN_PIDFILE"); f != "" {
		_ = os.WriteFile(f, []byte(strconv.Itoa(os.Getpid())), 0o644)
	}
	if f := os.Getenv("FAKE_PLUGIN_COUNTER"); f != "" {
		fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintln(fh, "start")
			fh.Close()
		}
	}
	if mode == "ignore_shutdown" {
		signal.Ignore(syscall.SIGTERM)
	}
	fmt.Fprintln(os.Stderr, "fake plugin started")

	out := json.NewEncoder(os.Stdout)
	reply := func(id json.RawMessage, result any) {
		_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	replyErr := func(id json.RawMessage, code int, msg string) {
		_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
	}
	notify := func(method string, params any) {
		_ = out.Encode(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	}

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var msg fakeMsg
		if json.Unmarshal(sc.Bytes(), &msg) != nil {
			continue
		}
		switch msg.Method {
		case "initialize":
			if f := os.Getenv("FAKE_PLUGIN_INITFILE"); f != "" {
				_ = os.WriteFile(f, msg.Params, 0o644)
			}
			switch mode {
			case "config_invalid":
				replyErr(msg.ID, CodeConfigInvalid, "token is required")
				continue
			case "bad_protocol":
				reply(msg.ID, map[string]any{"protocol_version": 2})
				continue
			}
			reply(msg.ID, map[string]any{"protocol_version": 1})
			notify("log", map[string]any{"level": "info", "message": "fake plugin ready"})
			if mode == "crash_after_init" || mode == "count_starts" {
				os.Exit(1)
			}
			if mode == "emit_inbound" {
				notify("inbound", map[string]any{"chat_id": "c1", "sender_id": "u1", "text": "hi", "message_id": "m9"})
			}
		case "send":
			notify("inbound", map[string]any{"chat_id": "echo", "sender_id": "echo", "text": string(msg.Params)})
			reply(msg.ID, map[string]any{})
		case "shutdown":
			if mode == "ignore_shutdown" {
				continue
			}
			reply(msg.ID, map[string]any{})
			os.Exit(0)
		}
	}
	if mode == "ignore_shutdown" {
		select {}
	}
}

func fakeHostMain() {
	sup := NewSupervisor(Spec{
		ChannelID: "fake",
		Dir:       os.TempDir(),
		Command:   fakePluginCommand(),
		Env:       fakePluginEnv("ok", "FAKE_PLUGIN_PIDFILE="+os.Getenv("FAKE_HOST_PLUGIN_PIDFILE")),
		Init:      InitializeParams{ProtocolVersion: 1, ChannelID: "fake"},
	}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sup.Run(ctx)
}
