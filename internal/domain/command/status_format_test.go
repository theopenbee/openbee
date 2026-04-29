package command

import "testing"

func TestFormatRelative(t *testing.T) {
	cases := []struct {
		seconds int64
		want    string
	}{
		{0, "0s"},
		{59, "59s"},
		{60, "1m"},
		{61, "1m"},
		{3599, "59m"},
		{3600, "1h"},
		{86399, "23h"},
		{86400, "1d"},
		{172800, "2d"},
	}
	for _, c := range cases {
		if got := formatRelative(c.seconds); got != c.want {
			t.Errorf("formatRelative(%d) = %q, want %q", c.seconds, got, c.want)
		}
	}
}

func TestFormatRelative_NegativeOrZero(t *testing.T) {
	// Clock skew or future timestamps must not panic; clamp to "0s".
	if got := formatRelative(-5); got != "0s" {
		t.Errorf("formatRelative(-5) = %q, want %q", got, "0s")
	}
}

func TestShortExecID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"abc", "abc"},
		{"abcdef12", "abcdef12"},
		{"abcdef1234567890", "abcdef12"},
	}
	for _, c := range cases {
		if got := shortExecID(c.in); got != c.want {
			t.Errorf("shortExecID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
