package utils

import "testing"

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"empty", "", 5, ""},
		{"under_limit", "abc", 5, "abc"},
		{"at_limit", "abcde", 5, "abcde"},
		{"over_limit_ascii", "abcdef", 5, "abcde…"},
		{"cjk_under", "中文", 5, "中文"},
		{"cjk_over", "中文中文中文", 4, "中文中文…"},
		{"max_zero_empty", "", 0, ""},
		{"max_zero_nonempty", "abc", 0, "…"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TruncateRunes(c.in, c.max); got != c.want {
				t.Errorf("TruncateRunes(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
			}
		})
	}
}
