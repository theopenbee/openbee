package msgingest

import (
	"regexp"
	"testing"
)

func buildREs(platform, name string) map[string]*regexp.Regexp {
	if name == "" {
		return nil
	}
	return map[string]*regexp.Regexp{
		platform: regexp.MustCompile(`\s*@` + regexp.QuoteMeta(name) + `\s*`),
	}
}

func TestStripBotMention(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		platform string
		botName  string
		want     string
	}{
		{
			name:     "prefix mention stripped",
			content:  "@机器人 /clear",
			platform: "test",
			botName:  "机器人",
			want:     "/clear",
		},
		{
			name:     "suffix mention stripped",
			content:  "/clear @机器人",
			platform: "test",
			botName:  "机器人",
			want:     "/clear",
		},
		{
			name:     "prefix mention with arg",
			content:  "@机器人 /clear 张三",
			platform: "test",
			botName:  "机器人",
			want:     "/clear 张三",
		},
		{
			name:     "suffix mention with arg",
			content:  "/clear 张三 @机器人",
			platform: "test",
			botName:  "机器人",
			want:     "/clear 张三",
		},
		{
			name:     "prefix mention engine command",
			content:  "@机器人 /engine codex",
			platform: "test",
			botName:  "机器人",
			want:     "/engine codex",
		},
		{
			name:     "no mention, no-op",
			content:  "/clear 张三",
			platform: "test",
			botName:  "机器人",
			want:     "/clear 张三",
		},
		{
			name:     "empty botName, no-op",
			content:  "@机器人 /clear",
			platform: "test",
			botName:  "",
			want:     "@机器人 /clear",
		},
		{
			name:     "unknown platform, no-op",
			content:  "@机器人 /clear",
			platform: "other",
			botName:  "机器人",
			want:     "@机器人 /clear",
		},
		{
			name:     "case sensitive no match",
			content:  "@机器人 /clear",
			platform: "test",
			botName:  "机器人Bot",
			want:     "@机器人 /clear",
		},
		{
			name:     "entire content is mention",
			content:  "@机器人",
			platform: "test",
			botName:  "机器人",
			want:     "",
		},
		{
			name:     "mention mid-sentence no word boundary",
			content:  "prefix@机器人suffix",
			platform: "test",
			botName:  "机器人",
			want:     "prefix suffix",
		},
		{
			name:     "mention on its own line",
			content:  "hello\n@机器人\nworld",
			platform: "test",
			botName:  "机器人",
			want:     "hello world",
		},
		{
			name:     "mention with leading newline",
			content:  "@机器人\nhello",
			platform: "test",
			botName:  "机器人",
			want:     "hello",
		},
		{
			name:     "mention with trailing newline",
			content:  "hello\n@机器人",
			platform: "test",
			botName:  "机器人",
			want:     "hello",
		},
		// Regression: DingTalk name is prefix of WeCom name; WeCom message must not be mangled.
		{
			name:     "prefix collision - wecom message unaffected by dingtalk name",
			content:  "@openbee本地测试 @someone hello",
			platform: "wecom",
			botName:  "openbee本地测试",
			want:     "@someone hello",
		},
		{
			name:     "prefix collision - dingtalk message unaffected by wecom name",
			content:  "@openbee hello",
			platform: "dingtalk",
			botName:  "openbee",
			want:     "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := buildREs(tt.platform, tt.botName)
			got := stripBotMention(tt.content, tt.platform, res)
			if got != tt.want {
				t.Errorf("stripBotMention(%q, %q) = %q, want %q", tt.content, tt.platform, got, tt.want)
			}
		})
	}
}
