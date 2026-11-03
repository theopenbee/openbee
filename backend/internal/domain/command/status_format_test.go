package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		assert.Equal(t, c.want, formatRelative(c.seconds))
	}
}

func TestFormatRelative_NegativeOrZero(t *testing.T) {
	// Clock skew or future timestamps must not panic; clamp to "0s".
	assert.Equal(t, "0s", formatRelative(-5))
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
		assert.Equal(t, c.want, shortExecID(c.in))
	}
}
