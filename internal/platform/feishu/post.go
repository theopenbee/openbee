package feishu

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PostParseResult contains the parsed output of a Feishu post (rich text) message.
type PostParseResult struct {
	TextContent string
	ImageKeys   []string
	MediaKeys   []MediaKeyInfo
}

// MediaKeyInfo holds a file key and optional file name from a post media element.
type MediaKeyInfo struct {
	FileKey  string
	FileName string
}

// postBody represents the title + content structure found inside post payloads.
type postBody struct {
	Title   string          `json:"title"`
	Content json.RawMessage `json:"content"`
}

// ParsePostContent parses a Feishu post message content string into structured data.
// Tries three formats: direct, locale-wrapped, double-wrapped (post > locale).
func ParsePostContent(content string) (*PostParseResult, error) {
	if content == "" {
		return nil, fmt.Errorf("empty post content")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("parse post JSON: %w", err)
	}

	// Try format 1: direct {"title": "...", "content": [[...]]}
	if _, hasContent := raw["content"]; hasContent {
		var body postBody
		if err := json.Unmarshal([]byte(content), &body); err == nil {
			return parsePostBody(body)
		}
	}

	// Try format 2: locale-wrapped {"zh_cn": {"title": "...", "content": [[...]]}}
	body, found := findLocaleBody(raw)
	if found {
		return parsePostBody(body)
	}

	// Try format 3: double-wrapped {"post": {"zh_cn": {...}}}
	if postRaw, ok := raw["post"]; ok {
		var postMap map[string]json.RawMessage
		if err := json.Unmarshal(postRaw, &postMap); err == nil {
			body, found := findLocaleBody(postMap)
			if found {
				return parsePostBody(body)
			}
		}
	}

	return nil, fmt.Errorf("unrecognized post content format")
}

// findLocaleBody tries to find a postBody in a map of locale keys.
func findLocaleBody(m map[string]json.RawMessage) (postBody, bool) {
	for _, v := range m {
		var body postBody
		if err := json.Unmarshal(v, &body); err == nil && body.Content != nil {
			return body, true
		}
	}
	return postBody{}, false
}

// parsePostBody renders a postBody into a PostParseResult.
func parsePostBody(body postBody) (*PostParseResult, error) {
	var paragraphs []json.RawMessage
	if err := json.Unmarshal(body.Content, &paragraphs); err != nil {
		return nil, fmt.Errorf("parse content paragraphs: %w", err)
	}

	result := &PostParseResult{}
	var textParts []string

	if body.Title != "" {
		textParts = append(textParts, body.Title)
	}

	for _, paraRaw := range paragraphs {
		var elements []map[string]any
		if err := json.Unmarshal(paraRaw, &elements); err != nil {
			continue
		}
		var paraText strings.Builder
		for _, elem := range elements {
			tag, _ := elem["tag"].(string)
			switch tag {
			case "text":
				text, _ := elem["text"].(string)
				text = applyStyles(text, elem)
				paraText.WriteString(text)

			case "a":
				text, _ := elem["text"].(string)
				href, _ := elem["href"].(string)
				paraText.WriteString(fmt.Sprintf("[%s](%s)", text, href))

			case "at":
				name, _ := elem["user_name"].(string)
				if name == "" {
					name, _ = elem["user_id"].(string)
				}
				paraText.WriteString(mentionPrefix + name)

			case "img":
				key, _ := elem["image_key"].(string)
				if key != "" {
					result.ImageKeys = append(result.ImageKeys, key)
				}

			case "media":
				fileKey, _ := elem["file_key"].(string)
				fileName, _ := elem["file_name"].(string)
				if fileKey != "" {
					result.MediaKeys = append(result.MediaKeys, MediaKeyInfo{
						FileKey:  fileKey,
						FileName: fileName,
					})
				}

			case "code_block", "pre":
				text, _ := elem["text"].(string)
				if text == "" {
					text, _ = elem["content"].(string)
				}
				lang, _ := elem["language"].(string)
				paraText.WriteString(fmt.Sprintf("\n```%s\n%s\n```\n", lang, text))

			case "code":
				text, _ := elem["text"].(string)
				paraText.WriteString("`" + text + "`")

			case "emotion":
				emoji, _ := elem["emoji_type"].(string)
				if emoji == "" {
					emoji, _ = elem["emoji"].(string)
				}
				paraText.WriteString(emoji)

			case "br":
				paraText.WriteString("\n")

			case "hr":
				paraText.WriteString("\n---\n")
			}
		}
		if paraText.Len() > 0 {
			textParts = append(textParts, paraText.String())
		}
	}

	result.TextContent = strings.Join(textParts, "\n")
	return result, nil
}

// BuildPostContent wraps a markdown string into a Feishu post message content JSON.
// The resulting string is suitable for use as the content field with msg_type "post".
// Each line is placed in its own paragraph row so that newlines render correctly.
func BuildPostContent(markdown string) string {
	type mdElem struct {
		Tag  string `json:"tag"`
		Text string `json:"text"`
	}
	type postLang struct {
		Title   string     `json:"title"`
		Content [][]mdElem `json:"content"`
	}
	lines := strings.Split(markdown, "\n")
	rows := make([][]mdElem, len(lines))
	for i, line := range lines {
		rows[i] = []mdElem{{"md", line}}
	}
	payload := map[string]postLang{
		"zh_cn": {
			Title:   "",
			Content: rows,
		},
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// applyStyles wraps text with markdown formatting based on the style array.
func applyStyles(text string, elem map[string]any) string {
	styles, ok := elem["style"].([]any)
	if !ok {
		return text
	}
	for _, s := range styles {
		style, _ := s.(string)
		switch style {
		case "bold":
			text = "**" + text + "**"
		case "italic":
			text = "*" + text + "*"
		case "code":
			text = "`" + text + "`"
		case "strikethrough":
			text = "~~" + text + "~~"
		}
	}
	return text
}
