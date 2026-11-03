package upgradecmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// archiveEntry is one file written into a test archive.
type archiveEntry struct {
	name string
	body string
}

func writeTestZip(t *testing.T, path string, entries []archiveEntry) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err, "create zip")
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		require.NoError(t, err, "zip create %s", e.name)
		_, err = w.Write([]byte(e.body))
		require.NoError(t, err, "zip write %s", e.name)
	}
	require.NoError(t, zw.Close(), "close zip writer")
	require.NoError(t, f.Close(), "close zip file")
}

func writeTestTarGz(t *testing.T, path string, entries []archiveEntry) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err, "create tar.gz")
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o755, Size: int64(len(e.body)), Typeflag: tar.TypeReg}
		require.NoError(t, tw.WriteHeader(hdr), "tar header %s", e.name)
		_, err = tw.Write([]byte(e.body))
		require.NoError(t, err, "tar write %s", e.name)
	}
	require.NoError(t, tw.Close(), "close tar writer")
	require.NoError(t, gz.Close(), "close gzip writer")
	require.NoError(t, f.Close(), "close tar.gz file")
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
		got := releaseArchiveName("0.0.42", tc.goos, tc.goarch)
		require.Equal(t, tc.want, got, "releaseArchiveName(0.0.42, %s, %s)", tc.goos, tc.goarch)
	}
}

// TestExtractBinary merges the zip, tar.gz, and nested-zip variants of
// "extraction finds the binary": each builds a different archive layout and
// expects extractBinary to recover the same binary bytes.
func TestExtractBinary(t *testing.T) {
	cases := []struct {
		name  string
		build func(t *testing.T, dir string) string
		want  string
	}{
		{
			// Same layout as the real Windows release zip: files at the root.
			name: "FromZip",
			build: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "openbee-1.0.0-windows-amd64.zip")
				writeTestZip(t, path, []archiveEntry{
					{"LICENSE", "license"},
					{"README.md", "readme"},
					{"openbee.exe", "windows-binary"},
				})
				return path
			},
			want: "windows-binary",
		},
		{
			name: "FromTarGz",
			build: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "openbee-1.0.0-linux-amd64.tar.gz")
				writeTestTarGz(t, path, []archiveEntry{
					{"LICENSE", "license"},
					{"openbee", "unix-binary"},
				})
				return path
			},
			want: "unix-binary",
		},
		{
			// goreleaser's wrap_in_directory would nest the binary; extraction must still find it.
			name: "FromZipSubdirectory",
			build: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "openbee-1.0.0-windows-amd64.zip")
				writeTestZip(t, path, []archiveEntry{
					{"openbee-1.0.0-windows-amd64/LICENSE", "license"},
					{"openbee-1.0.0-windows-amd64/openbee.exe", "nested-binary"},
				})
				return path
			},
			want: "nested-binary",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.build(t, t.TempDir())
			var buf bytes.Buffer
			require.NoError(t, extractBinary(path, &buf))
			require.Equal(t, tc.want, buf.String())
		})
	}
}

func TestExtractBinaryMissingFromZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openbee-1.0.0-windows-amd64.zip")
	writeTestZip(t, path, []archiveEntry{{"LICENSE", "license"}})
	err := extractBinary(path, &bytes.Buffer{})
	require.ErrorIs(t, err, errBinaryNotFound)
}
