package upgradecmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDownloadFileWritesBodyAndExtra(t *testing.T) {
	const body = "openbee-archive-bytes"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out")
	h := sha256.New()
	require.NoError(t, downloadFile(srv.URL, dest, h))
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	require.Equal(t, body, string(got))
	want := sha256.Sum256([]byte(body))
	require.Equal(t, hex.EncodeToString(want[:]), hex.EncodeToString(h.Sum(nil)), "extra writer did not receive the downloaded bytes")
}

func TestDownloadFileNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out")
	err := downloadFile(srv.URL, dest, nil)
	require.Error(t, err, "downloadFile on 404 returned nil error")
	_, err = os.Stat(dest)
	require.True(t, os.IsNotExist(err), "dest should not be created on non-200, stat err = %v", err)
}

func TestParseChecksumFile(t *testing.T) {
	data := []byte("aaa111  openbee-1.0.0-linux-amd64.tar.gz\nbbb222  openbee-1.0.0-darwin-arm64.tar.gz\n")
	got, err := parseChecksumFile(data, "openbee-1.0.0-darwin-arm64.tar.gz")
	require.NoError(t, err)
	require.Equal(t, "bbb222", got)
}

func TestParseChecksumFileMissing(t *testing.T) {
	data := []byte("aaa111  openbee-1.0.0-linux-amd64.tar.gz\n")
	_, err := parseChecksumFile(data, "openbee-1.0.0-windows-amd64.tar.gz")
	require.Error(t, err, "parseChecksumFile for missing asset returned nil error")
}

func TestParseChecksumFileBinaryMode(t *testing.T) {
	// sha256sum -b marks binary-mode entries with a leading '*'.
	data := []byte("aaa111 *openbee-1.0.0-windows-amd64.zip\n")
	got, err := parseChecksumFile(data, "openbee-1.0.0-windows-amd64.zip")
	require.NoError(t, err)
	require.Equal(t, "aaa111", got)
}

func TestParseChecksumFileCRLF(t *testing.T) {
	data := []byte("aaa111  openbee-1.0.0-windows-amd64.zip\r\nbbb222  openbee-1.0.0-linux-amd64.tar.gz\r\n")
	got, err := parseChecksumFile(data, "openbee-1.0.0-windows-amd64.zip")
	require.NoError(t, err)
	require.Equal(t, "aaa111", got)
}

func TestVerifyChecksumIgnoresHexCase(t *testing.T) {
	sum := sha256.Sum256([]byte("archive"))
	upper := strings.ToUpper(hex.EncodeToString(sum[:]))
	checksums := []byte(upper + "  openbee-1.0.0-linux-amd64.tar.gz\n")
	err := verifyChecksum(checksums, "openbee-1.0.0-linux-amd64.tar.gz", sum[:])
	require.NoError(t, err, "verifyChecksum with uppercase hex")
}

func TestVerifyChecksumMismatch(t *testing.T) {
	sum := sha256.Sum256([]byte("archive"))
	checksums := []byte(strings.Repeat("0", 64) + "  openbee-1.0.0-linux-amd64.tar.gz\n")
	err := verifyChecksum(checksums, "openbee-1.0.0-linux-amd64.tar.gz", sum[:])
	require.Error(t, err, "verifyChecksum mismatch: want SHA256 mismatch")
	require.Contains(t, err.Error(), "SHA256 mismatch")
}

func TestVerifyChecksumMissingEntry(t *testing.T) {
	sum := sha256.Sum256([]byte("archive"))
	checksums := []byte(strings.Repeat("0", 64) + "  openbee-1.0.0-linux-amd64.tar.gz\n")
	err := verifyChecksum(checksums, "openbee-1.0.0-windows-amd64.zip", sum[:])
	require.Error(t, err, "verifyChecksum missing entry: want not-found error")
	require.Contains(t, err.Error(), "in checksums.txt")
}

// newReleaseServer serves files keyed by URL path (without the leading slash).
// Any other path answers 503, standing in for a flaky or failing CDN.
func newReleaseServer(t *testing.T, files map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchVerifiedArchive(t *testing.T) {
	const name = "openbee-1.0.0-linux-amd64.tar.gz"
	const body = "archive-bytes"
	sum := sha256.Sum256([]byte(body))
	srv := newReleaseServer(t, map[string]string{
		"checksums.txt": hex.EncodeToString(sum[:]) + "  " + name + "\n",
		name:            body,
	})

	path, err := fetchVerifiedArchive(srv.URL, name, t.TempDir())
	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err, "read archive")
	require.Equal(t, body, string(got))
}

func TestFetchVerifiedArchiveAbortsWithoutChecksums(t *testing.T) {
	const name = "openbee-1.0.0-linux-amd64.tar.gz"
	srv := newReleaseServer(t, map[string]string{name: "archive-bytes"}) // checksums.txt -> 503

	dir := t.TempDir()
	_, err := fetchVerifiedArchive(srv.URL, name, dir)
	require.Error(t, err, "fetchVerifiedArchive with checksums.txt unavailable: want checksums.txt error")
	require.Contains(t, err.Error(), "checksums.txt")
	_, statErr := os.Stat(filepath.Join(dir, name))
	require.True(t, os.IsNotExist(statErr), "archive should not be downloaded when checksums.txt is unavailable, stat err = %v", statErr)
}

func TestFetchVerifiedArchiveMismatch(t *testing.T) {
	const name = "openbee-1.0.0-linux-amd64.tar.gz"
	srv := newReleaseServer(t, map[string]string{
		"checksums.txt": strings.Repeat("0", 64) + "  " + name + "\n",
		name:            "tampered-bytes",
	})

	_, err := fetchVerifiedArchive(srv.URL, name, t.TempDir())
	require.Error(t, err, "fetchVerifiedArchive with wrong hash: want SHA256 mismatch")
	require.Contains(t, err.Error(), "SHA256 mismatch")
}

func TestCopyWithLimit(t *testing.T) {
	cases := []struct {
		body    string
		wantErr bool
	}{
		{"abc", false},  // under the limit
		{"abcd", false}, // exactly the limit
		{"abcde", true}, // one byte over
	}
	for _, tc := range cases {
		var buf bytes.Buffer
		err := copyWithLimit(&buf, strings.NewReader(tc.body), 4)
		require.Equal(t, tc.wantErr, err != nil, "copyWithLimit(%q, limit 4) err = %v", tc.body, err)
		if !tc.wantErr {
			require.Equal(t, tc.body, buf.String(), "copyWithLimit(%q) copied", tc.body)
		}
	}
}

func TestDownloadFileTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	orig := downloadClient
	downloadClient = &http.Client{Timeout: 100 * time.Millisecond}
	t.Cleanup(func() { downloadClient = orig })

	err := downloadFile(srv.URL, filepath.Join(t.TempDir(), "out"), nil)
	var netErr net.Error
	require.ErrorAs(t, err, &netErr, "downloadFile against a stalled server: want a timeout")
	require.True(t, netErr.Timeout())
}
