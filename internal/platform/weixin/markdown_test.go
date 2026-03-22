package weixin

import "testing"

func TestMarkdownToPlainText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello world", "hello world"},
		{"bold", "this is **bold** text", "this is bold text"},
		{"italic", "this is *italic* text", "this is italic text"},
		{"inline code", "use `fmt.Println`", "use fmt.Println"},
		{"link", "visit [Google](https://google.com)", "visit Google (https://google.com)"},
		{"image", "see ![alt](https://img.png) here", "see  here"},
		{"heading", "# Title\n## Subtitle", "Title\nSubtitle"},
		{"code block", "before\n```go\nfmt.Println(\"hi\")\n```\nafter", "before\nfmt.Println(\"hi\")\nafter"},
		{"code block no lang", "```\ncode\n```", "code"},
		{"mixed", "# Title\n**bold** and *italic* with `code`", "Title\nbold and italic with code"},
		{"strikethrough", "this is ~~deleted~~ text", "this is deleted text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := markdownToPlainText(tt.input)
			if got != tt.want {
				t.Errorf("markdownToPlainText(%q)\ngot:  %q\nwant: %q", tt.input, got, tt.want)
			}
		})
	}
}
