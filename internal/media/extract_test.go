package media

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractText_PlainTextFile(t *testing.T) {
	s := &Service{}
	path := filepath.Join(t.TempDir(), "test.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	text, err := s.ExtractText(context.Background(), path)
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if text != "hello world" {
		t.Errorf("got %q, want %q", text, "hello world")
	}
}

func TestExtractText_PlainTextTruncation(t *testing.T) {
	s := &Service{}
	path := filepath.Join(t.TempDir(), "big.md")
	content := strings.Repeat("a", 60000)
	os.WriteFile(path, []byte(content), 0o644)

	text, err := s.ExtractText(context.Background(), path)
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if len(text) != maxExtractChars {
		t.Errorf("len = %d, want %d", len(text), maxExtractChars)
	}
}

func TestExtractText_UnsupportedExtension(t *testing.T) {
	s := &Service{}
	path := filepath.Join(t.TempDir(), "data.zip")
	os.WriteFile(path, []byte("PK\x03\x04"), 0o644)

	text, err := s.ExtractText(context.Background(), path)
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if text != "" {
		t.Errorf("expected empty text for unsupported extension, got %q", text)
	}
}

func TestExtractText_GoFile(t *testing.T) {
	s := &Service{}
	path := filepath.Join(t.TempDir(), "main.go")
	os.WriteFile(path, []byte("package main\n\nfunc main() {}"), 0o644)

	text, err := s.ExtractText(context.Background(), path)
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text, "package main") {
		t.Errorf("expected Go source content, got %q", text)
	}
}
