package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

func newGCM(key string) (cipher.AEAD, error) {
	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: invalid hex key: %w", err)
	}
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("crypto: key must be 32 bytes (64 hex chars), got %d", len(keyBytes))
	}
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("crypto: create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create GCM: %w", err)
	}
	return gcm, nil
}

// NewGCM creates a reusable AES-256-GCM cipher from a hex-encoded key.
// Use DecryptGCM to decrypt multiple ciphertexts with the same key efficiently.
func NewGCM(key string) (cipher.AEAD, error) {
	return newGCM(key)
}

// DecryptGCM decrypts a ciphertext produced by Encrypt using a pre-created GCM cipher.
func DecryptGCM(gcm cipher.AEAD, ciphertext string) (string, error) {
	combined, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(combined) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertextWithTag := combined[:nonceSize], combined[nonceSize:]
	plainBytes, err := gcm.Open(nil, nonce, ciphertextWithTag, nil)
	if err != nil {
		return "", err
	}
	return string(plainBytes), nil
}

// Encrypt encrypts plaintext using AES-256-GCM with the given hex-encoded key.
// key must be a 64-character hex string (32 bytes).
func Encrypt(key, plaintext string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertextWithTag := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	combined := append(nonce, ciphertextWithTag...)
	return base64.StdEncoding.EncodeToString(combined), nil
}

// Decrypt decrypts ciphertext produced by Encrypt.
// key must be a 64-character hex string (32 bytes).
func Decrypt(key, ciphertext string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	combined, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(combined) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertextWithTag := combined[:nonceSize], combined[nonceSize:]
	plainBytes, err := gcm.Open(nil, nonce, ciphertextWithTag, nil)
	if err != nil {
		return "", err
	}
	return string(plainBytes), nil
}

// Mask returns a display-safe version of value.
// If len(value) >= 8: first4 + "****" + last4. Otherwise "****".
func Mask(value string) string {
	runes := []rune(value)
	if len(runes) < 8 {
		return "****"
	}
	return string(runes[:4]) + "****" + string(runes[len(runes)-4:])
}
