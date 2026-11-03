package model

import (
	"fmt"
	"strings"
	"testing"
)

func seededHex(seed uint32, length int) string {
	state := seed
	var out strings.Builder
	for i := 0; i < length; i++ {
		state = state*1664525 + 1013904223
		fmt.Fprintf(&out, "%x", state>>28)
	}
	return out.String()
}

func seededUUID(seed uint32) string {
	hex := seededHex(seed, 32)
	return hex[:8] + "-" + hex[8:12] + "-" + hex[12:16] + "-" + hex[16:20] + "-" + hex[20:]
}

func TestAvatarIndexFNV(t *testing.T) {
	tests := []struct {
		id   string
		want int
	}{
		{"", 5},
		{"a", 4},
		{"foobar", 0},
		{"工", 0},
		{"🐝", 7},
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			if got := AvatarIndex(tc.id); got != tc.want {
				t.Fatalf("AvatarIndex(%q) = %d, want %d", tc.id, got, tc.want)
			}
		})
	}
}

func TestAvatarIndexStableAndInRange(t *testing.T) {
	if AvatarIndex("a") != AvatarIndex("a") {
		t.Fatal("AvatarIndex is not stable")
	}
	for seed := uint32(1); seed <= 200; seed++ {
		id := seededUUID(seed)
		if index := AvatarIndex(id); index < 0 || index > 7 {
			t.Fatalf("AvatarIndex(%q) = %d, outside 0..7", id, index)
		}
	}
}

func TestAvatarIndexSpreadsIDs(t *testing.T) {
	seen := map[int]bool{}
	for seed := uint32(1); seed <= 50; seed++ {
		seen[AvatarIndex(seededUUID(seed))] = true
	}
	if len(seen) < 6 {
		t.Fatalf("only %d distinct indexes over 50 ids", len(seen))
	}
}

func TestInitialsCJK(t *testing.T) {
	if got := Initials("张三"); got != "张" {
		t.Fatalf("got %q, want 张", got)
	}
}

func TestInitialsTwoWords(t *testing.T) {
	if got := Initials("maya lin"); got != "ML" {
		t.Fatalf("got %q, want ML", got)
	}
}

func TestInitialsSeparators(t *testing.T) {
	if got := Initials("front_end-dev"); got != "FE" {
		t.Fatalf("got %q, want FE", got)
	}
}

func TestInitialsEdges(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "?"},
		{"blank", " \t ", "?"},
		{"single word takes two letters", "bot", "BO"},
		{"single letter", "a", "A"},
		{"trims before looking", "  maya lin  ", "ML"},
		{"leading separator", "_front-end", "FE"},
		{"cjk after a latin letter is not cjk", "a张", "A张"},
		{"cjk word then latin", "张 lin", "张"},
		{"astral first letter", "🐝bot", "🐝B"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Initials(tc.in); got != tc.want {
				t.Fatalf("Initials(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
