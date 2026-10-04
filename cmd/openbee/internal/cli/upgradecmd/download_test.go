package upgradecmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadFileWritesBodyAndExtra(t *testing.T) {
	const body = "openbee-archive-bytes"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out")
	h := sha256.New()
	if err := downloadFile(srv.URL, dest, h); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != body {
		t.Fatalf("dest content = %q, want %q", got, body)
	}
	want := sha256.Sum256([]byte(body))
	if hex.EncodeToString(h.Sum(nil)) != hex.EncodeToString(want[:]) {
		t.Fatalf("extra writer did not receive the downloaded bytes")
	}
}

func TestDownloadFileNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out")
	if err := downloadFile(srv.URL, dest, nil); err == nil {
		t.Fatalf("downloadFile on 404 returned nil error")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("dest should not be created on non-200, stat err = %v", err)
	}
}

func TestParseChecksumFile(t *testing.T) {
	data := []byte("aaa111  openbee-1.0.0-linux-amd64.tar.gz\nbbb222  openbee-1.0.0-darwin-arm64.tar.gz\n")
	got, err := parseChecksumFile(data, "openbee-1.0.0-darwin-arm64.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksumFile: %v", err)
	}
	if got != "bbb222" {
		t.Fatalf("parseChecksumFile = %q, want %q", got, "bbb222")
	}
}

func TestParseChecksumFileMissing(t *testing.T) {
	data := []byte("aaa111  openbee-1.0.0-linux-amd64.tar.gz\n")
	if _, err := parseChecksumFile(data, "openbee-1.0.0-windows-amd64.tar.gz"); err == nil {
		t.Fatalf("parseChecksumFile for missing asset returned nil error")
	}
}

func TestParseChecksumFileBinaryMode(t *testing.T) {
	// sha256sum -b marks binary-mode entries with a leading '*'.
	data := []byte("aaa111 *openbee-1.0.0-windows-amd64.zip\n")
	got, err := parseChecksumFile(data, "openbee-1.0.0-windows-amd64.zip")
	if err != nil {
		t.Fatalf("parseChecksumFile(*name): %v", err)
	}
	if got != "aaa111" {
		t.Fatalf("parseChecksumFile(*name) = %q, want %q", got, "aaa111")
	}
}

func TestParseChecksumFileCRLF(t *testing.T) {
	data := []byte("aaa111  openbee-1.0.0-windows-amd64.zip\r\nbbb222  openbee-1.0.0-linux-amd64.tar.gz\r\n")
	got, err := parseChecksumFile(data, "openbee-1.0.0-windows-amd64.zip")
	if err != nil {
		t.Fatalf("parseChecksumFile(CRLF): %v", err)
	}
	if got != "aaa111" {
		t.Fatalf("parseChecksumFile(CRLF) = %q, want %q", got, "aaa111")
	}
}

func TestVerifyChecksumIgnoresHexCase(t *testing.T) {
	sum := sha256.Sum256([]byte("archive"))
	upper := strings.ToUpper(hex.EncodeToString(sum[:]))
	checksums := []byte(upper + "  openbee-1.0.0-linux-amd64.tar.gz\n")
	if err := verifyChecksum(checksums, "openbee-1.0.0-linux-amd64.tar.gz", sum[:]); err != nil {
		t.Fatalf("verifyChecksum with uppercase hex: %v", err)
	}
}

func TestVerifyChecksumMismatch(t *testing.T) {
	sum := sha256.Sum256([]byte("archive"))
	checksums := []byte(strings.Repeat("0", 64) + "  openbee-1.0.0-linux-amd64.tar.gz\n")
	err := verifyChecksum(checksums, "openbee-1.0.0-linux-amd64.tar.gz", sum[:])
	if err == nil || !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("verifyChecksum mismatch: err = %v, want SHA256 mismatch", err)
	}
}

func TestVerifyChecksumMissingEntry(t *testing.T) {
	sum := sha256.Sum256([]byte("archive"))
	checksums := []byte(strings.Repeat("0", 64) + "  openbee-1.0.0-linux-amd64.tar.gz\n")
	err := verifyChecksum(checksums, "openbee-1.0.0-windows-amd64.zip", sum[:])
	if err == nil || !strings.Contains(err.Error(), "in checksums.txt") {
		t.Fatalf("verifyChecksum missing entry: err = %v, want not-found error", err)
	}
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
	if err != nil {
		t.Fatalf("fetchVerifiedArchive: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if string(got) != body {
		t.Fatalf("archive content = %q, want %q", got, body)
	}
}

func TestFetchVerifiedArchiveAbortsWithoutChecksums(t *testing.T) {
	const name = "openbee-1.0.0-linux-amd64.tar.gz"
	srv := newReleaseServer(t, map[string]string{name: "archive-bytes"}) // checksums.txt -> 503

	dir := t.TempDir()
	_, err := fetchVerifiedArchive(srv.URL, name, dir)
	if err == nil || !strings.Contains(err.Error(), "checksums.txt") {
		t.Fatalf("fetchVerifiedArchive with checksums.txt unavailable: err = %v, want checksums.txt error", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(statErr) {
		t.Fatalf("archive should not be downloaded when checksums.txt is unavailable, stat err = %v", statErr)
	}
}

func TestFetchVerifiedArchiveMismatch(t *testing.T) {
	const name = "openbee-1.0.0-linux-amd64.tar.gz"
	srv := newReleaseServer(t, map[string]string{
		"checksums.txt": strings.Repeat("0", 64) + "  " + name + "\n",
		name:            "tampered-bytes",
	})

	_, err := fetchVerifiedArchive(srv.URL, name, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("fetchVerifiedArchive with wrong hash: err = %v, want SHA256 mismatch", err)
	}
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
		if (err != nil) != tc.wantErr {
			t.Fatalf("copyWithLimit(%q, limit 4) err = %v, wantErr %v", tc.body, err, tc.wantErr)
		}
		if !tc.wantErr && buf.String() != tc.body {
			t.Fatalf("copyWithLimit(%q) copied %q", tc.body, buf.String())
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
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("downloadFile against a stalled server: err = %v, want a timeout", err)
	}
}
