package worker

import (
	"sync"
	"testing"
)

func TestActiveLogRegistry_RegisterAndGet(t *testing.T) {
	r := NewActiveLogRegistry()
	writeLine := r.Register("exec-1")

	writeLine("hello")
	writeLine("world")

	content, ok := r.Get("exec-1")
	if !ok {
		t.Fatal("expected ok=true for registered id")
	}
	if content != "hello\nworld\n" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestActiveLogRegistry_GetUnknown(t *testing.T) {
	r := NewActiveLogRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected ok=false for unknown id")
	}
}

func TestActiveLogRegistry_UnregisterStopsLive(t *testing.T) {
	r := NewActiveLogRegistry()
	writeLine := r.Register("exec-2")
	writeLine("line1")
	r.Unregister("exec-2")

	_, ok := r.Get("exec-2")
	if ok {
		t.Error("expected ok=false after unregister")
	}
}

func TestActiveLogRegistry_UnregisterAbsentIsNoop(t *testing.T) {
	r := NewActiveLogRegistry()
	// Must not panic
	r.Unregister("never-registered")
}

func TestActiveLogRegistry_RegisterDuplicateIsNoop(t *testing.T) {
	r := NewActiveLogRegistry()
	write1 := r.Register("exec-3")
	write2 := r.Register("exec-3") // duplicate — must not panic, must return no-op

	write1("from-first")
	write2("from-second") // no-op, must not write anything

	content, _ := r.Get("exec-3")
	if content != "from-first\n" {
		t.Errorf("duplicate Register must be a no-op, got: %q", content)
	}
}

func TestActiveLogRegistry_ConcurrentWrites(t *testing.T) {
	r := NewActiveLogRegistry()
	writeLine := r.Register("exec-4")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			writeLine("line")
		}()
	}
	wg.Wait()

	content, _ := r.Get("exec-4")
	// 100 lines × "line\n" = 500 bytes
	if len(content) != 500 {
		t.Errorf("expected 500 bytes, got %d", len(content))
	}
}
