package upgradecmd

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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
