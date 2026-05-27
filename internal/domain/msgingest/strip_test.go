package msgingest

import (
	"regexp"
	"testing"

	"github.com/theopenbee/openbee/internal/platform"
)

// buildREs builds the gateway's bot-name regex map keyed by the composite
// "<platform>:<account>" key the gateway uses at lookup time.
func buildREs(plat, name string) map[string]*regexp.Regexp {
	g := &Gateway{}
	WithAccountBotNames(map[string]string{platform.AccountKey(plat, "default"): name})(g)
	return g.botNameREs
}

func TestStripBotMention(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		platform    string // message platform passed to stripBotMention
		regPlatform string // platform key used when building the regex map; defaults to platform
		botName     string
		want        string
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
			name:        "unknown platform, no-op",
			content:     "@机器人 /clear",
			platform:    "other",
			regPlatform: "test", // bot registered for "test"; message from "other" → no-op
			botName:     "机器人",
			want:         "@机器人 /clear",
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
			regPlatform := tt.regPlatform
			if regPlatform == "" {
				regPlatform = tt.platform
			}
			g := &Gateway{botNameREs: buildREs(regPlatform, tt.botName)}
			msg := platform.InboundMessage{Content: tt.content, Platform: tt.platform, AccountName: "default"}
			got := g.stripBotMention(msg)
			if got != tt.want {
				t.Errorf("stripBotMention(%q, %q) = %q, want %q", tt.content, tt.platform, got, tt.want)
			}
		})
	}
}
