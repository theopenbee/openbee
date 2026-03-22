package weixin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadAndDecrypt(t *testing.T) {
	// Prepare encrypted content
	key := make([]byte, 16)
	rand.Read(key)
	plaintext := []byte("hello weixin cdn")
	ciphertext := encryptAES128ECB(plaintext, key)
	aesKeyBase64 := base64.StdEncoding.EncodeToString(key)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(ciphertext)
	}))
	defer srv.Close()

	got, err := downloadAndDecrypt(context.Background(), srv.URL, "param=test", aesKeyBase64)
	if err != nil {
		t.Fatalf("downloadAndDecrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("got %q, want %q", got, plaintext)
	}
}

func TestEncryptAndUpload(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		json.NewEncoder(w).Encode(map[string]string{"download_param": "dl-param-123"})
	}))
	defer srv.Close()

	plaintext := []byte("upload test data")
	dlParam, aesKeyHex, err := encryptAndUpload(context.Background(), plaintext, fmt.Sprintf("%s?upload=1", srv.URL), "filekey-1", srv.URL)
	if err != nil {
		t.Fatalf("encryptAndUpload: %v", err)
	}
	if dlParam == "" {
		t.Error("expected non-empty download_param")
	}
	if aesKeyHex == "" {
		t.Error("expected non-empty aesKey")
	}
	if len(receivedBody) == 0 {
		t.Error("expected upload body")
	}
}
