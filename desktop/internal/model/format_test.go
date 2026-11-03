package model

import (
	"testing"
	"time"
)

var zone = time.FixedZone("UTC+8", 8*3600)

func at(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, zone)
}

func TestListTime(t *testing.T) {
	now := at(2026, 10, 27, 15, 30)

	tests := []struct {
		name          string
		ts            time.Time
		now           time.Time
		lang          string
		wantText      string
		wantYesterday bool
	}{
		{"today", at(2026, 10, 27, 9, 5), now, "en", "09:05", false},
		{"yesterday", at(2026, 10, 26, 23, 59), now, "en", "", true},
		{"this year", at(2026, 3, 4, 8, 0), now, "en", "3/4", false},
		{"older year", at(2025, 12, 31, 8, 0), now, "en", "12/31/2025", false},
		{"midnight itself is today", at(2026, 10, 27, 0, 0), at(2026, 10, 27, 0, 1), "en", "00:00", false},
		{"a minute before midnight is yesterday", at(2026, 10, 26, 23, 59), at(2026, 10, 27, 0, 1), "en", "", true},
		{"two days back is a date", at(2026, 10, 25, 23, 59), at(2026, 10, 27, 0, 1), "en", "10/25", false},
		{"yesterday across a month boundary", at(2026, 2, 28, 20, 0), at(2026, 3, 1, 8, 0), "en", "", true},
		{"yesterday across a year boundary", at(2025, 12, 31, 20, 0), at(2026, 1, 1, 8, 0), "en", "", true},
		{"yesterday across a leap day", at(2024, 2, 29, 20, 0), at(2024, 3, 1, 8, 0), "en", "", true},
		{"zh yesterday", at(2026, 10, 26, 10, 0), now, "zh", "", true},
		{"zh this year", at(2026, 3, 4, 8, 0), now, "zh", "3/4", false},
		{"zh older year", at(2025, 3, 4, 8, 0), now, "zh", "2025/3/4", false},
		{"zh today", at(2026, 10, 27, 14, 2), now, "zh", "14:02", false},
		{"zh-CN is zh", at(2025, 3, 4, 8, 0), now, "zh-CN", "2025/3/4", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text, yesterday := ListTime(tc.ts.UnixMilli(), tc.now, tc.lang)
			if text != tc.wantText || yesterday != tc.wantYesterday {
				t.Fatalf("got (%q, %v), want (%q, %v)", text, yesterday, tc.wantText, tc.wantYesterday)
			}
		})
	}
}

func TestListTimeZh(t *testing.T) {
	text, yesterday := ListTime(at(2025, 3, 3, 8, 0).UnixMilli(), at(2026, 10, 27, 15, 30), "zh")
	if text != "2025/3/3" || yesterday {
		t.Fatalf("got (%q, %v), want (\"2025/3/3\", false)", text, yesterday)
	}
}

func TestListTimeUsesTheZoneOfNow(t *testing.T) {
	ts := time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC).UnixMilli()

	east, _ := ListTime(ts, at(2026, 10, 28, 9, 0), "en")
	west, _ := ListTime(ts, time.Date(2026, 10, 27, 21, 0, 0, 0, time.FixedZone("UTC-5", -5*3600)), "en")

	if east != "04:00" {
		t.Fatalf("east of UTC got %q, want 04:00", east)
	}
	if west != "15:00" {
		t.Fatalf("west of UTC got %q, want 15:00", west)
	}
}

func TestMessageTime(t *testing.T) {
	now := at(2026, 10, 27, 15, 30)

	tests := []struct {
		name string
		ts   int64
		lang string
		want string
	}{
		{"zero timestamp", 0, "en", ""},
		{"today", at(2026, 10, 27, 9, 5).UnixMilli(), "en", "09:05"},
		{"24-hour clock en", at(2026, 10, 27, 14, 32).UnixMilli(), "en", "14:32"},
		{"24-hour clock zh", at(2026, 10, 27, 14, 32).UnixMilli(), "zh", "14:32"},
		{"other day", at(2026, 10, 20, 9, 5).UnixMilli(), "en", "Oct 20, 09:05"},
		{"year is never shown", at(2025, 3, 3, 9, 5).UnixMilli(), "en", "Mar 3, 09:05"},
		{"midnight", at(2026, 10, 27, 0, 0).UnixMilli(), "en", "00:00"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MessageTime(tc.ts, now, tc.lang); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMessageTimeZh(t *testing.T) {
	got := MessageTime(at(2026, 10, 27, 9, 5).UnixMilli(), at(2026, 10, 28, 15, 30), "zh")
	if got != "10月27日 09:05" {
		t.Fatalf("got %q, want %q", got, "10月27日 09:05")
	}
}

func TestRelativeTime(t *testing.T) {
	now := at(2026, 10, 27, 15, 30)
	ago := func(d time.Duration) int64 { return now.Add(-d).UnixMilli() }

	tests := []struct {
		name string
		ts   int64
		want Relative
	}{
		{"missing", 0, Relative{}},
		{"just now", ago(59 * time.Second), Relative{Unit: "now"}},
		{"future", ago(-time.Minute), Relative{Unit: "now"}},
		{"one minute", ago(60 * time.Second), Relative{Unit: "minutes", N: 1}},
		{"last minute", ago(59*time.Minute + 59*time.Second), Relative{Unit: "minutes", N: 59}},
		{"one hour", ago(time.Hour), Relative{Unit: "hours", N: 1}},
		{"last hour", ago(23*time.Hour + 59*time.Minute), Relative{Unit: "hours", N: 23}},
		{"one day", ago(24 * time.Hour), Relative{Unit: "days", N: 1}},
		{"many days", ago(72*time.Hour + 5*time.Minute), Relative{Unit: "days", N: 3}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RelativeTime(tc.ts, now); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
