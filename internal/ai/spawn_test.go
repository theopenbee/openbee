package ai_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestSpawnSubprocess_HappyPath(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "log.txt")

	spec := ai.SubprocessSpec{
		Binary:  "/bin/sh",
		Args:    []string{"-c", "echo hello"},
		WorkDir: dir,
		LogPath: logPath,
		Env:     os.Environ(),
	}

	proc, out, err := ai.SpawnSubprocess(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if proc.PID() == 0 {
		t.Error("PID should be non-zero")
	}

	select {
	case ev := <-out:
		if ev.Type != ai.OutputDone {
			t.Errorf("want Done, got %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	data, _ := os.ReadFile(logPath)
	if string(data) != "hello\n" {
		t.Errorf("log content = %q", string(data))
	}
}

func TestSpawnSubprocess_NonZeroExit(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "log.txt")

	spec := ai.SubprocessSpec{
		Binary:  "/bin/sh",
		Args:    []string{"-c", "exit 7"},
		WorkDir: dir,
		LogPath: logPath,
		Env:     os.Environ(),
	}

	_, out, err := ai.SpawnSubprocess(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-out:
		if ev.Type != ai.OutputError {
			t.Errorf("want Error, got %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestSpawnSubprocess_OpenLogFailure(t *testing.T) {
	spec := ai.SubprocessSpec{
		Binary:  "/bin/sh",
		Args:    []string{"-c", "true"},
		LogPath: "/nonexistent-dir-zxywqv/log.txt",
		Env:     os.Environ(),
	}
	_, _, err := ai.SpawnSubprocess(context.Background(), spec)
	if err == nil {
		t.Fatal("want error opening log")
	}
}

func TestSpawnSubprocess_PostWaitOverride(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "log.txt")

	override := ai.Output{Type: ai.OutputError, Content: "custom"}
	spec := ai.SubprocessSpec{
		Binary:  "/bin/sh",
		Args:    []string{"-c", "echo ok"},
		WorkDir: dir,
		LogPath: logPath,
		Env:     os.Environ(),
		PostWait: func(waitErr error, _ string) *ai.Output {
			if waitErr != nil {
				return nil
			}
			return &override
		},
	}

	_, out, err := ai.SpawnSubprocess(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-out:
		if ev.Type != ai.OutputError || ev.Content != "custom" {
			t.Errorf("want custom override, got %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestSpawnSubprocess_StdinDelivered(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "log.txt")

	spec := ai.SubprocessSpec{
		Binary:  "/bin/sh",
		Args:    []string{"-c", "cat"},
		WorkDir: dir,
		LogPath: logPath,
		Env:     os.Environ(),
		Stdin:   "the-payload",
	}

	_, out, err := ai.SpawnSubprocess(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-out:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	data, _ := os.ReadFile(logPath)
	if string(data) != "the-payload" {
		t.Errorf("log content = %q", string(data))
	}
}
