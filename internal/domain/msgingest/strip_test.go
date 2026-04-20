package msgingest

import (
	"testing"
)

func TestStripBotMentions(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		botNames []string
		want     string
	}{
		{
			name:     "prefix mention stripped",
			content:  "@机器人 /clear",
			botNames: []string{"机器人"},
			want:     "/clear",
		},
		{
			name:     "suffix mention stripped",
			content:  "/clear @机器人",
			botNames: []string{"机器人"},
			want:     "/clear",
		},
		{
			name:     "prefix mention with arg",
			content:  "@机器人 /clear 张三",
			botNames: []string{"机器人"},
			want:     "/clear 张三",
		},
		{
			name:     "suffix mention with arg",
			content:  "/clear 张三 @机器人",
			botNames: []string{"机器人"},
			want:     "/clear 张三",
		},
		{
			name:     "prefix mention engine command",
			content:  "@机器人 /engine codex",
			botNames: []string{"机器人"},
			want:     "/engine codex",
		},
		{
			name:     "no mention, no-op",
			content:  "/clear 张三",
			botNames: []string{"机器人"},
			want:     "/clear 张三",
		},
		{
			name:     "empty botNames, no-op",
			content:  "@机器人 /clear",
			botNames: []string{},
			want:     "@机器人 /clear",
		},
		{
			name:     "nil botNames, no-op",
			content:  "@机器人 /clear",
			botNames: nil,
			want:     "@机器人 /clear",
		},
		{
			name:     "case sensitive no match",
			content:  "@机器人 /clear",
			botNames: []string{"机器人Bot"},
			want:     "@机器人 /clear",
		},
		{
			name:     "multiple bot names matches second",
			content:  "@OpenBee /engine codex",
			botNames: []string{"机器人", "OpenBee"},
			want:     "/engine codex",
		},
		{
			name:     "entire content is mention",
			content:  "@机器人",
			botNames: []string{"机器人"},
			want:     "",
		},
		{
			name:     "mention mid-sentence no word boundary",
			content:  "prefix@机器人suffix",
			botNames: []string{"机器人"},
			want:     "prefix suffix",
		},
		{
			name:     "mention on its own line",
			content:  "hello\n@机器人\nworld",
			botNames: []string{"机器人"},
			want:     "hello world",
		},
		{
			name:     "mention with leading newline",
			content:  "@机器人\nhello",
			botNames: []string{"机器人"},
			want:     "hello",
		},
		{
			name:     "mention with trailing newline",
			content:  "hello\n@机器人",
			botNames: []string{"机器人"},
			want:     "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripBotMentions(tt.content, compileBotNameREs(tt.botNames))
			if got != tt.want {
				t.Errorf("stripBotMentions(%q, %v) = %q, want %q", tt.content, tt.botNames, got, tt.want)
			}
		})
	}
}
