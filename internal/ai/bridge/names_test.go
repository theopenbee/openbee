package bridge

import (
	"reflect"
	"testing"

	ai "github.com/theopenbee/openbee/internal/ai"
)

func TestEngineConstantsMatchInternalAI(t *testing.T) {
	if EngineClaude != ai.EngineClaude || EngineCodex != ai.EngineCodex ||
		EnginePi != ai.EnginePi || EngineKimi != ai.EngineKimi {
		t.Fatalf("bridge engine constants drift from internal/ai")
	}
}

func TestAllEnginesMatchInternalAI(t *testing.T) {
	got := AllEngines()
	want := ai.AllEngines()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AllEngines: got %v, want %v", got, want)
	}
}

func TestEnabledEnginesFiltersAndPreservesCanonicalOrder(t *testing.T) {
	b := &bridgeImpl{engines: map[string]ai.EngineAdapter{
		ai.EngineCodex:  nil,
		ai.EngineClaude: nil,
	}}
	got := b.EnabledEngines()
	want := []string{ai.EngineClaude, ai.EngineCodex}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EnabledEngines: got %v, want %v", got, want)
	}
}

func TestIsEnabled(t *testing.T) {
	b := &bridgeImpl{engines: map[string]ai.EngineAdapter{ai.EngineClaude: nil}}
	if !b.IsEnabled(ai.EngineClaude) || b.IsEnabled(ai.EngineCodex) {
		t.Fatalf("IsEnabled wrong")
	}
}
