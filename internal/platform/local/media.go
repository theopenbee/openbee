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

func RenderAttachments(content, mediaDir string) string {
	paths, text := DecodeMediaPaths(content)
	if len(paths) == 0 {
		return content
	}
	var sb strings.Builder
	for _, p := range paths {
		sb.WriteString(attachmentPrefix)
		sb.WriteString(filepath.Join(mediaDir, p))
		sb.WriteByte('\n')
	}
	sb.WriteString(text)
	return sb.String()
}
