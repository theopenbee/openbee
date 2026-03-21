package worker

import (
	"testing"
)

func TestManager_RegistryRoundTrip(t *testing.T) {
	// Verify the registry behaves correctly when used the same way launchRuntime does.
	registry := NewActiveLogRegistry()

	_, ok := registry.Get("exec-x")
	if ok {
		t.Error("registry should be empty initially")
	}

	writeLine := registry.Register("exec-x")
	writeLine("hello from worker")

	content, ok := registry.Get("exec-x")
	if !ok {
		t.Error("expected content after registration")
	}
	if content != "hello from worker\n" {
		t.Errorf("unexpected content: %q", content)
	}

	registry.Unregister("exec-x")
	_, ok = registry.Get("exec-x")
	if ok {
		t.Error("expected registry to be empty after unregister")
	}
}
