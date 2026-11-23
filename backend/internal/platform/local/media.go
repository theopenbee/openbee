package local

import (
	"path/filepath"
	"strings"
)

const fileMediaMarker = "\x00[file]"

const legacyFileMediaMarker = "[file]"

const fileMediaPrefix = fileMediaMarker + " "
const legacyFileMediaPrefix = legacyFileMediaMarker + " "

const attachmentPrefix = "[attachment] "

func EncodeMediaPaths(paths []string, text string) string {
	if len(paths) == 0 {
		return text
	}
	var sb strings.Builder
	for _, p := range paths {
		sb.WriteString(fileMediaMarker)
		sb.WriteByte(' ')
		sb.WriteString(p)
		sb.WriteByte('\n')
	}
	sb.WriteString(text)
	return sb.String()
}

func DecodeMediaPaths(content string) ([]string, string) {
	var paths []string
	for {
		var rest string
		switch {
		case strings.HasPrefix(content, fileMediaPrefix):
			rest = content[len(fileMediaPrefix):]
		case strings.HasPrefix(content, legacyFileMediaPrefix):
			rest = content[len(legacyFileMediaPrefix):]
		default:
			return paths, content
		}
		filename, after, ok := strings.Cut(rest, "\n")
		if !ok {
			break
		}
		paths = append(paths, filename)
		content = after
	}
	return paths, content
}

func MergeMediaParts(parts []string) string {
	var paths, texts []string
	for _, part := range parts {
		p, text := DecodeMediaPaths(part)
		paths = append(paths, p...)
		if text != "" {
			texts = append(texts, text)
		}
	}
	return EncodeMediaPaths(paths, strings.Join(texts, "\n\n"))
}

func RenderAttachments(content, mediaDir string) string {
	paths, text := DecodeMediaPaths(content)
	if len(paths) == 0 {
		return content
	}
	var sb strings.Builder
	for _, p := range paths {
		name := filepath.Base(p)
		if name == "." || name == ".." || name == string(filepath.Separator) {
			continue
		}
		sb.WriteString(attachmentPrefix)
		sb.WriteString(filepath.Join(mediaDir, name))
		sb.WriteByte('\n')
	}
	sb.WriteString(text)
	return sb.String()
}
