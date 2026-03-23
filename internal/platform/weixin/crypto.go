package weixin

import (
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// encryptAesEcb encrypts plaintext using AES-128-ECB with PKCS7 padding.
func encryptAesEcb(plaintext, key []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(fmt.Sprintf("weixin: invalid AES key: %v", err))
	}

	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(ciphertext[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}
	return ciphertext
}

// decryptAesEcb decrypts AES-128-ECB ciphertext and removes PKCS7 padding.
func decryptAesEcb(ciphertext, key []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("weixin: ciphertext is empty")
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("weixin: ciphertext length %d not a multiple of block size", len(ciphertext))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("weixin: create cipher: %w", err)
	}

	plaintext := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += aes.BlockSize {
		block.Decrypt(plaintext[i:i+aes.BlockSize], ciphertext[i:i+aes.BlockSize])
	}

	return pkcs7Unpad(plaintext, aes.BlockSize)
}

// parseAesKey decodes a base64-encoded AES key, handling the dual-format:
// - 16 bytes after decode → use directly as AES-128 key
// - 32 bytes after decode → treat as hex string, hex-decode to 16 bytes
func parseAesKey(base64Key string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("weixin: decode aes key base64: %w", err)
	}
	switch len(decoded) {
	case 16:
		return decoded, nil
	case 32:
		key, err := hex.DecodeString(string(decoded))
		if err != nil {
			return nil, fmt.Errorf("weixin: decode aes key hex: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("weixin: unexpected aes key length %d (want 16 or 32)", len(decoded))
	}
}

// aesEcbPaddedSize returns the ciphertext size for a given plaintext size.
//
// For plaintextSize < 16, the WeChat spec formula applies:
//
//	((plaintextSize + 1) / 16 + 1) * 16
//
// This adds an extra block compared to standard PKCS7 when plaintextSize == 15.
// For plaintextSize >= 16, standard PKCS7 padded size is used:
//
//	(plaintextSize / 16 + 1) * 16
func aesEcbPaddedSize(plaintextSize int) int {
	if plaintextSize < 16 {
		return ((plaintextSize+1)/16 + 1) * 16
	}
	return (plaintextSize/16 + 1) * 16
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - (len(data) % blockSize)
	padded := make([]byte, len(data)+padLen)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}
	return padded
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("weixin: pkcs7 unpad: empty data")
	}
	padLen := int(data[len(data)-1])
	if padLen < 1 || padLen > blockSize || padLen > len(data) {
		return nil, fmt.Errorf("weixin: pkcs7 invalid padding value: %d", padLen)
	}
	for i := len(data) - padLen; i < len(data); i++ {
		if data[i] != byte(padLen) {
			return nil, fmt.Errorf("weixin: pkcs7 padding bytes inconsistent")
		}
	}
	return data[:len(data)-padLen], nil
}
