package upgradecmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// archiveEntry is one file written into a test archive.
type archiveEntry struct {
	name string
	body string
}

func writeTestZip(t *testing.T, path string, entries []archiveEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("zip create %s: %v", e.name, err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatalf("zip write %s: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}
}

func writeTestTarGz(t *testing.T, path string, entries []archiveEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create tar.gz: %v", err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o755, Size: int64(len(e.body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header %s: %v", e.name, err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatalf("tar write %s: %v", e.name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close tar.gz file: %v", err)
	}
}

func TestReleaseArchiveName(t *testing.T) {
	cases := []struct {
		goos, goarch string
		want         string
	}{
		{"windows", "amd64", "openbee-0.0.42-windows-amd64.zip"},
		{"windows", "arm64", "openbee-0.0.42-windows-arm64.zip"},
		{"linux", "amd64", "openbee-0.0.42-linux-amd64.tar.gz"},
		{"darwin", "arm64", "openbee-0.0.42-darwin-arm64.tar.gz"},
	}
	for _, tc := range cases {
		if got := releaseArchiveName("0.0.42", tc.goos, tc.goarch); got != tc.want {
			t.Fatalf("releaseArchiveName(0.0.42, %s, %s) = %q, want %q", tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestExtractBinaryFromZip(t *testing.T) {
	// Same layout as the real Windows release zip: files at the root.
	path := filepath.Join(t.TempDir(), "openbee-1.0.0-windows-amd64.zip")
	writeTestZip(t, path, []archiveEntry{
		{"LICENSE", "license"},
		{"README.md", "readme"},
		{"openbee.exe", "windows-binary"},
	})
	var buf bytes.Buffer
	if err := extractBinary(path, &buf); err != nil {
		t.Fatalf("extractBinary(zip): %v", err)
	}
	if buf.String() != "windows-binary" {
		t.Fatalf("extracted %q, want %q", buf.String(), "windows-binary")
	}
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openbee-1.0.0-linux-amd64.tar.gz")
	writeTestTarGz(t, path, []archiveEntry{
		{"LICENSE", "license"},
		{"openbee", "unix-binary"},
	})
	var buf bytes.Buffer
	if err := extractBinary(path, &buf); err != nil {
		t.Fatalf("extractBinary(tar.gz): %v", err)
	}
	if buf.String() != "unix-binary" {
		t.Fatalf("extracted %q, want %q", buf.String(), "unix-binary")
	}
}

func TestExtractBinaryFromZipSubdirectory(t *testing.T) {
	// goreleaser's wrap_in_directory would nest the binary; extraction must still find it.
	path := filepath.Join(t.TempDir(), "openbee-1.0.0-windows-amd64.zip")
	writeTestZip(t, path, []archiveEntry{
		{"openbee-1.0.0-windows-amd64/LICENSE", "license"},
		{"openbee-1.0.0-windows-amd64/openbee.exe", "nested-binary"},
	})
	var buf bytes.Buffer
	if err := extractBinary(path, &buf); err != nil {
		t.Fatalf("extractBinary(nested zip): %v", err)
	}
	if buf.String() != "nested-binary" {
		t.Fatalf("extracted %q, want %q", buf.String(), "nested-binary")
	}
}

func TestExtractBinaryMissingFromZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openbee-1.0.0-windows-amd64.zip")
	writeTestZip(t, path, []archiveEntry{{"LICENSE", "license"}})
	err := extractBinary(path, &bytes.Buffer{})
	if !errors.Is(err, errBinaryNotFound) {
		t.Fatalf("extractBinary on zip without binary: err = %v, want errBinaryNotFound", err)
	}
}
