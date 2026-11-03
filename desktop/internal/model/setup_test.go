package model

import (
	"slices"
	"testing"
	"time"
)

func TestSetupLink(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"padded link", " https://a.b/c ", true},
		{"plain http", "http://a.b", true},
		{"with query", "https://a.b/c?d=1&e=2#f", true},
		{"link inside a sentence", "go to https://x", false},
		{"link followed by words", "https://a.b/c now", false},
		{"no scheme", "a.b/c", false},
		{"other scheme", "ftp://a.b", false},
		{"scheme only", "https://", false},
		{"uppercase scheme", "HTTPS://A.B", false},
		{"empty", "", false},
		{"unicode space inside", "https://a.b/ c", false},
		{"text before", "see https://docs.example for help", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SetupLink(tc.in); got != tc.want {
				t.Fatalf("SetupLink(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestLatestLink(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{
			"returns the last line that is a link, trimmed",
			[]string{"scan", "https://a.example/1", "waiting", "https://a.example/2 ", "scanned"},
			"https://a.example/2",
		},
		{"ignores links inside sentences", []string{"see https://docs.example for help"}, ""},
		{"no lines", nil, ""},
		{"a single link", []string{"https://a.example/1"}, "https://a.example/1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := LatestLink(tc.lines); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLastOutputLines(t *testing.T) {
	lines := []string{"1", "https://a.b/1", "2", "3", "4", "https://a.b/2", "5", "6", "7", "8"}
	tests := []struct {
		name  string
		lines []string
		n     int
		want  []string
	}{
		{"drops link lines and keeps the last six", lines, 6, []string{"3", "4", "5", "6", "7", "8"}},
		{"fewer lines than the limit", []string{"a", "https://x.y", "b"}, 6, []string{"a", "b"}},
		{"padded links are dropped too", []string{" https://x.y ", "a"}, 6, []string{"a"}},
		{"lines are returned as received", []string{" a "}, 6, []string{" a "}},
		{"nothing but links", []string{"https://x.y"}, 6, nil},
		{"no lines", nil, 6, nil},
		{"zero limit", []string{"a"}, 0, nil},
		{"negative limit", []string{"a"}, -2, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := LastOutputLines(tc.lines, tc.n); !slices.Equal(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLastOutputLinesDoesNotAliasTheInput(t *testing.T) {
	lines := []string{"a", "b", "c"}
	got := LastOutputLines(lines, 2)
	got[0] = "changed"
	if lines[1] != "b" {
		t.Fatalf("input was mutated: %v", lines)
	}
}

func TestElapsed(t *testing.T) {
	start := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		{"zero", start, "0:00"},
		{"seconds only", start.Add(12400 * time.Millisecond), "0:12"},
		{"one minute and a quarter", start.Add(75 * time.Second), "1:15"},
		{"two minutes five", start.Add(125 * time.Second), "2:05"},
		{"minutes are unbounded", start.Add(75*time.Minute + 3*time.Second), "75:03"},
		{"negative duration", start.Add(-5 * time.Millisecond), "0:00"},
		{"far in the past", start.Add(-time.Hour), "0:00"},
		{"just below a minute", start.Add(59999 * time.Millisecond), "0:59"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Elapsed(start, tc.now); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestElapsedUnknownStart(t *testing.T) {
	if got := Elapsed(time.Time{}, time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)); got != "0:00" {
		t.Fatalf("got %q, want 0:00 for a zero start", got)
	}
}
