package media

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/nguyenthenguyen/docx"
)

const maxExtractChars = 50000

// plainTextExts lists file extensions that are treated as plain text.
var plainTextExts = map[string]bool{
	".txt": true, ".md": true, ".csv": true, ".json": true,
	".xml": true, ".yaml": true, ".yml": true, ".html": true, ".htm": true,
	".log": true, ".conf": true, ".ini": true,
	".sh": true, ".py": true, ".js": true, ".ts": true,
	".css": true, ".sql": true, ".go": true, ".java": true,
	".rs": true, ".rb": true, ".php": true,
}

// ExtractText extracts text content from a file based on its extension.
// Returns ("", nil) for unsupported file types.
func (s *Service) ExtractText(_ context.Context, path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case plainTextExts[ext]:
		return extractPlainText(path)
	case ext == ".pdf":
		return extractPDF(path)
	case ext == ".docx":
		return extractDOCX(path)
	default:
		return "", nil
	}
}

func extractPlainText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read plain text: %w", err)
	}
	text := string(data)
	if len(text) > maxExtractChars {
		text = text[:maxExtractChars]
	}
	return text, nil
}

func extractPDF(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("open PDF: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(text)
		buf.WriteString("\n")
		if buf.Len() > maxExtractChars {
			break
		}
	}
	text := buf.String()
	if len(text) > maxExtractChars {
		text = text[:maxExtractChars]
	}
	return text, nil
}

func extractDOCX(path string) (string, error) {
	r, err := docx.ReadDocxFile(path)
	if err != nil {
		return "", fmt.Errorf("open DOCX: %w", err)
	}
	defer r.Close()

	doc := r.Editable()
	text := doc.GetContent()
	// The docx library returns XML-ish content; extract text between tags
	text = stripXMLTags(text)
	if len(text) > maxExtractChars {
		text = text[:maxExtractChars]
	}
	return text, nil
}

// stripXMLTags removes XML/HTML tags from a string, preserving text content.
func stripXMLTags(s string) string {
	var buf strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
