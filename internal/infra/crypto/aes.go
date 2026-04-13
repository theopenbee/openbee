package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
)

// Encrypt encrypts plaintext using AES-256-GCM, returns base64-encoded ciphertext with nonce prefix.
// key must be a 64-character hex string (32 bytes).
func Encrypt(key, plaintext string) (string, error) {
	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertextWithTag := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	combined := append(nonce, ciphertextWithTag...)
	return base64.StdEncoding.EncodeToString(combined), nil
}

// Decrypt decrypts ciphertext produced by Encrypt, returns plaintext.
// key must be a 64-character hex string (32 bytes).
func Decrypt(key, ciphertext string) (string, error) {
	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		return "", err
	}
	combined, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(combined) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertextWithTag := combined[:nonceSize], combined[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextWithTag, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// Mask generates a masked display value.
// If len(value) >= 8: returns first 4 chars + "****" + last 4 chars.
// If len(value) < 8: returns "****".
func Mask(value string) string {
	runes := []rune(value)
	if len(runes) < 8 {
		return "****"
	}
	return string(runes[:4]) + "****" + string(runes[len(runes)-4:])
}
