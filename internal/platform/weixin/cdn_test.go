package weixin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadAndDecrypt(t *testing.T) {
	// Prepare encrypted content
	key := make([]byte, 16)
	rand.Read(key)
	plaintext := []byte("hello weixin cdn")
	ciphertext, err := encryptAES128ECB(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
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
