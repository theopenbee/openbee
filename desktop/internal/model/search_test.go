package model

import (
	"slices"
	"testing"
)

func labels(items []Searchable, order []int) []string {
	out := make([]string, len(order))
	for i, at := range order {
		out[i] = items[at].Label
	}
	return out
}

func TestRankSearchKeepsOrderWithoutQuery(t *testing.T) {
	items := []Searchable{{Label: "Bee"}, {Label: "Ada"}, {Label: "Zed"}}
	if got := RankSearch(items, "  "); !slices.Equal(got, []int{0, 1, 2}) {
		t.Fatalf("order = %v, want the given order", got)
	}
}

func TestRankSearchPrefersLabelPrefixes(t *testing.T) {
	items := []Searchable{
		{Label: "Open settings", Keywords: []string{"preferences"}},
		{Label: "Plugins", Description: "Settings for channels"},
		{Label: "Settings"},
		{Label: "Ship release notes"},
	}
	got := labels(items, RankSearch(items, "set"))
	want := []string{"Settings", "Open settings", "Plugins"}
	if !slices.Equal(got, want) {
		t.Fatalf("ranked %q, want %q", got, want)
	}
}

func TestRankSearchMatchesEveryWord(t *testing.T) {
	items := []Searchable{
		{Label: "Permissions", Keywords: []string{"Grace Hopper"}},
		{Label: "Constraints", Keywords: []string{"Grace Hopper"}},
		{Label: "Permissions", Keywords: []string{"Ada"}},
	}
	got := RankSearch(items, "grace perm")
	if !slices.Equal(got, []int{0}) {
		t.Fatalf("order = %v, want only the first item", got)
	}
}

func TestRankSearchFindsChineseInsideNames(t *testing.T) {
	items := []Searchable{
		{Label: "小乔", Description: "负责 openbee 开发"},
		{Label: "大桥", Description: "测试和验收"},
		{Label: "Atlas", Description: "研究和数据分析"},
	}
	if got := labels(items, RankSearch(items, "乔")); !slices.Equal(got, []string{"小乔"}) {
		t.Fatalf("乔 matched %q, want 小乔", got)
	}
	if got := labels(items, RankSearch(items, "验收")); !slices.Equal(got, []string{"大桥"}) {
		t.Fatalf("验收 matched %q, want 大桥", got)
	}
	if got := labels(items, RankSearch(items, "数据")); !slices.Equal(got, []string{"Atlas"}) {
		t.Fatalf("数据 matched %q, want Atlas", got)
	}
}

func TestRankSearchLatinNeedsWordStarts(t *testing.T) {
	items := []Searchable{{Label: "Grace Hopper"}, {Label: "Ada Lovelace", Description: "Data analysis"}}
	if got := RankSearch(items, "ce"); len(got) != 0 {
		t.Fatalf("ce matched %q, want none: two letters inside a word are not a match", labels(items, got))
	}
	if got := labels(items, RankSearch(items, "ace")); !slices.Equal(got, []string{"Grace Hopper", "Ada Lovelace"}) {
		t.Fatalf("ace matched %q, want both names", got)
	}
	if got := labels(items, RankSearch(items, "hop")); !slices.Equal(got, []string{"Grace Hopper"}) {
		t.Fatalf("hop matched %q, want Grace Hopper", got)
	}
	if got := labels(items, RankSearch(items, "analysis")); !slices.Equal(got, []string{"Ada Lovelace"}) {
		t.Fatalf("analysis matched %q, want Ada Lovelace", got)
	}
}

func TestRankSearchLooseLetters(t *testing.T) {
	items := []Searchable{{Label: "Share project"}, {Label: "Settings"}}
	if got := labels(items, RankSearch(items, "shpr")); !slices.Equal(got, []string{"Share project"}) {
		t.Fatalf("shpr matched %q, want Share project", got)
	}
}
