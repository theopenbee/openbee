package api

import (
	"testing"
)

func TestEncodeMediaPaths(t *testing.T) {
	tests := []struct {
		name   string
		paths  []string
		text   string
		want   string
	}{
		{
			name:  "no files",
			paths: nil,
			text:  "hello",
			want:  "hello",
		},
		{
			name:  "single file",
			paths: []string{"photo.png"},
			text:  "see this",
			want:  "\x00[file] photo.png\nsee this",
		},
		{
			name:  "multiple files",
			paths: []string{"a.png", "b.pdf"},
			text:  "two files",
			want:  "\x00[file] a.png\n\x00[file] b.pdf\ntwo files",
		},
		{
			name:  "files with empty text",
			paths: []string{"x.jpg"},
			text:  " ",
			want:  "\x00[file] x.jpg\n ",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := encodeMediaPaths(tc.paths, tc.text)
			if got != tc.want {
				t.Errorf("encodeMediaPaths(%v, %q) = %q, want %q", tc.paths, tc.text, got, tc.want)
			}
		})
	}
}

func TestDecodeMediaPaths(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantPaths []string
		wantText  string
	}{
		{
			name:      "no files",
			content:   "hello",
			wantPaths: nil,
			wantText:  "hello",
		},
		{
			name:      "single file (new format)",
			content:   "\x00[file] photo.png\nhello",
			wantPaths: []string{"photo.png"},
			wantText:  "hello",
		},
		{
			name:      "multiple files (new format)",
			content:   "\x00[file] a.png\n\x00[file] b.pdf\ntwo files",
			wantPaths: []string{"a.png", "b.pdf"},
			wantText:  "two files",
		},
		{
			name:      "single file (legacy format)",
			content:   "[file] photo.png\nhello",
			wantPaths: []string{"photo.png"},
			wantText:  "hello",
		},
		{
			name:      "multiple files (legacy format)",
			content:   "[file] a.png\n[file] b.pdf\ntwo files",
			wantPaths: []string{"a.png", "b.pdf"},
			wantText:  "two files",
		},
		{
			name:      "user text starting with [file] is not decoded",
			content:   "[file] this is just text without NUL prefix",
			wantPaths: nil,
			wantText:  "[file] this is just text without NUL prefix",
		},
		{
			name:      "files with empty text",
			content:   "\x00[file] x.jpg\n ",
			wantPaths: []string{"x.jpg"},
			wantText:  " ",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotPaths, gotText := decodeMediaPaths(tc.content)
			if len(gotPaths) != len(tc.wantPaths) {
				t.Fatalf("decodeMediaPaths paths len = %d, want %d", len(gotPaths), len(tc.wantPaths))
			}
			for i, p := range gotPaths {
				if p != tc.wantPaths[i] {
					t.Errorf("paths[%d] = %q, want %q", i, p, tc.wantPaths[i])
				}
			}
			if gotText != tc.wantText {
				t.Errorf("text = %q, want %q", gotText, tc.wantText)
			}
		})
	}
}
