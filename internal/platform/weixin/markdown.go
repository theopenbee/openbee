package weixin

import (
	"regexp"
	"strings"
)

var (
	reCodeBlock     = regexp.MustCompile("(?s)```[a-zA-Z]*\n?(.*?)```")
	reImage         = regexp.MustCompile(`!\[[^\]]*\]\([^)]+\)`)
	reLink          = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reBold          = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reStrikethrough = regexp.MustCompile(`~~(.+?)~~`)
	reItalic        = regexp.MustCompile(`\*(.+?)\*`)
	reInlineCode    = regexp.MustCompile("`([^`]+)`")
	reHeading       = regexp.MustCompile(`(?m)^#{1,6}\s+`)
)

// markdownToPlainText strips Markdown formatting for WeChat's plaintext-only display.
func markdownToPlainText(text string) string {
	// Code blocks: keep content, remove fences (trim trailing newline from captured content)
	text = reCodeBlock.ReplaceAllStringFunc(text, func(match string) string {
		sub := reCodeBlock.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		return strings.TrimRight(sub[1], "\n")
	})
	// Images: remove entirely
	text = reImage.ReplaceAllString(text, "")
	// Links: [text](url) → text (url)
	text = reLink.ReplaceAllString(text, "$1 ($2)")
	// Bold
	text = reBold.ReplaceAllString(text, "$1")
	// Strikethrough
	text = reStrikethrough.ReplaceAllString(text, "$1")
	// Italic
	text = reItalic.ReplaceAllString(text, "$1")
	// Inline code
	text = reInlineCode.ReplaceAllString(text, "$1")
	// Headings
	text = reHeading.ReplaceAllString(text, "")
	// Clean up extra blank lines
	text = strings.TrimSpace(text)
	return text
}
