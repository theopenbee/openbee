package weixin

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestPKCS7PadUnpad(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		blockSz int
	}{
		{"empty", []byte{}, 16},
		{"1 byte", []byte{0x42}, 16},
		{"15 bytes", bytes.Repeat([]byte{0xAA}, 15), 16},
		{"16 bytes exact", bytes.Repeat([]byte{0xBB}, 16), 16},
		{"17 bytes", bytes.Repeat([]byte{0xCC}, 17), 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			padded := pkcs7Pad(tt.input, tt.blockSz)
			if len(padded)%tt.blockSz != 0 {
				t.Fatalf("padded length %d not multiple of %d", len(padded), tt.blockSz)
			}
			unpadded, err := pkcs7Unpad(padded, tt.blockSz)
			if err != nil {
				t.Fatalf("unpad: %v", err)
			}
			if !bytes.Equal(unpadded, tt.input) {
				t.Errorf("roundtrip failed: got %x, want %x", unpadded, tt.input)
			}
		})
	}
}

func TestEncryptDecryptAES128ECBRoundtrip(t *testing.T) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		plain []byte
	}{
		{"short", []byte("hello world")},
		{"exact block", bytes.Repeat([]byte{0x41}, 16)},
		{"multi block", bytes.Repeat([]byte{0x42}, 100)},
		{"large", bytes.Repeat([]byte{0x43}, 4096)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cipher, err := encryptAES128ECB(tt.plain, key)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			if len(cipher)%16 != 0 {
				t.Fatalf("ciphertext length %d not multiple of 16", len(cipher))
			}
			plain, err := decryptAES128ECB(cipher, key)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if !bytes.Equal(plain, tt.plain) {
				t.Errorf("roundtrip failed for %s", tt.name)
			}
		})
	}
}

func TestDecryptAES128ECBInvalidPadding(t *testing.T) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	// Create a ciphertext with corrupted padding
	plain := []byte("test data 12345")
	cipher, err := encryptAES128ECB(plain, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	// Corrupt last byte
	cipher[len(cipher)-1] ^= 0xFF

	_, err = decryptAES128ECB(cipher, key)
	if err == nil {
		t.Error("expected error for corrupted padding")
	}
}
