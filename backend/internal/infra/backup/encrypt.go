package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/scrypt"
)

const (
	saltSize = 32
	keySize  = 32 // AES-256
	// scrypt parameters: N=32768, r=8, p=1 — tuned for ~100ms on modern hardware
	scryptN = 32768
	scryptR = 8
	scryptP = 1
)

// EncryptFile reads src, encrypts it with AES-256-GCM using a key derived
// from password via scrypt, and writes [salt(32) | nonce(12) | ciphertext] to dst.
func EncryptFile(src, dst, password string) error {
	plaintext, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}

	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("generate salt: %w", err)
	}

	key, err := deriveKey(password, salt)
	if err != nil {
		return err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	// sealedData is: nonce || encrypted_plaintext || GCM_tag
	sealedData := gcm.Seal(nonce, nonce, plaintext, nil)

	// Output: salt | sealedData (nonce + ciphertext + tag)
	out := make([]byte, 0, saltSize+len(sealedData))
	out = append(out, salt...)
	out = append(out, sealedData...)

	return os.WriteFile(dst, out, 0600)
}

// DecryptFile reads an encrypted file produced by EncryptFile and writes the
// plaintext to dst. Returns an error containing "incorrect password or corrupted file"
// if decryption fails.
func DecryptFile(src, dst, password string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read encrypted file: %w", err)
	}
	if len(data) < saltSize+12 {
		return fmt.Errorf("incorrect password or corrupted file")
	}

	salt := data[:saltSize]
	rest := data[saltSize:]

	key, err := deriveKey(password, salt)
	if err != nil {
		return err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonceSize := gcm.NonceSize()
	if len(rest) < nonceSize {
		return fmt.Errorf("incorrect password or corrupted file")
	}
	nonce, ciphertext := rest[:nonceSize], rest[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("incorrect password or corrupted file")
	}

	return os.WriteFile(dst, plaintext, 0600)
}

func deriveKey(password string, salt []byte) ([]byte, error) {
	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, keySize)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	return key, nil
}
