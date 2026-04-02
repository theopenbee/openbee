package platform

import "testing"

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal", "report.pdf", "report.pdf"},
		{"spaces preserved", "my report.pdf", "my report.pdf"},
		{"control chars", "file\x00name\x1f.txt", "file_name_.txt"},
		{"carriage return and newline", "file\r\nname.txt", "file_name.txt"},
		{"backslash", `file\name.txt`, "file_name.txt"},
		{"double quote", `file"name.txt`, "file_name.txt"},
		{"forward slash", "path/to/file.txt", "path_to_file.txt"},
		{"colon", "C:file.txt", "C_file.txt"},
		{"consecutive special chars", "a\x00\x01\x02b.txt", "a_b.txt"},
		{"empty string", "", ""},
		{"unicode preserved", "café.pdf", "café.pdf"},
		{"del char", "file\x7fname.txt", "file_name.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFileName(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeFileName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
