package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/theopenbee/openbee/internal/ai"
)

func TestAggregateUsage_AddsAcrossLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	body := `{"model":"m1","in":10,"out":2}
{"model":"m1","in":5,"out":1}
{"model":"m2","in":7,"out":3}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	type line struct {
		Model string `json:"model"`
		In    int64  `json:"in"`
		Out   int64  `json:"out"`
	}
	usages, err := AggregateUsage[line](path, func(l line, agg map[string]*ai.TokenUsage) {
		if l.Model == "" {
			return
		}
		u := agg[l.Model]
		if u == nil {
			u = &ai.TokenUsage{Model: l.Model}
			agg[l.Model] = u
		}
		u.InputTokens += l.In
		u.OutputTokens += l.Out
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 2 {
		t.Fatalf("want 2 models, got %d", len(usages))
	}
	byModel := map[string]ai.TokenUsage{}
	for _, u := range usages {
		byModel[u.Model] = u
	}
	if byModel["m1"].InputTokens != 15 || byModel["m1"].OutputTokens != 3 {
		t.Errorf("m1 wrong: %+v", byModel["m1"])
	}
	if byModel["m2"].InputTokens != 7 || byModel["m2"].OutputTokens != 3 {
		t.Errorf("m2 wrong: %+v", byModel["m2"])
	}
}

func TestAggregateUsage_MissingFile(t *testing.T) {
	_, err := AggregateUsage[struct{}]("/nonexistent/path/abc.jsonl", func(struct{}, map[string]*ai.TokenUsage) {})
	if err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestAggregateUsage_SkipsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	body := `not-json
{"model":"m1","in":3,"out":1}
also-not-json
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	type line struct {
		Model string `json:"model"`
		In    int64  `json:"in"`
		Out   int64  `json:"out"`
	}
	usages, err := AggregateUsage[line](path, func(l line, agg map[string]*ai.TokenUsage) {
		if l.Model == "" {
			return
		}
		u := agg[l.Model]
		if u == nil {
			u = &ai.TokenUsage{Model: l.Model}
			agg[l.Model] = u
		}
		u.InputTokens += l.In
		u.OutputTokens += l.Out
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 || usages[0].InputTokens != 3 {
		t.Errorf("got %+v", usages)
	}
}
