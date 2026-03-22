package weixin

import (
	"crypto/aes"
	"fmt"
)

// encryptAES128ECB encrypts plaintext using AES-128-ECB with PKCS#7 padding.
// Note: ECB mode is inherently insecure for multi-block data; this is dictated
// by the WeChat CDN protocol, not a design choice.
func encryptAES128ECB(plaintext []byte, key []byte) []byte {
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

// decryptAES128ECB decrypts AES-128-ECB ciphertext and removes PKCS#7 padding.
func decryptAES128ECB(ciphertext []byte, key []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("weixin: ciphertext is empty")
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("weixin: ciphertext length %d not a multiple of %d", len(ciphertext), aes.BlockSize)
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

// pkcs7Pad pads data to a multiple of blockSize using PKCS#7.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	padding := make([]byte, padLen)
	for i := range padding {
		padding[i] = byte(padLen)
	}
	return append(data, padding...)
}

// pkcs7Unpad removes PKCS#7 padding.
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("weixin: invalid padded data length: %d", len(data))
	}
	padLen := int(data[len(data)-1])
	if padLen < 1 || padLen > blockSize || padLen > len(data) {
		return nil, fmt.Errorf("weixin: invalid PKCS#7 padding value: %d", padLen)
	}
	for i := len(data) - padLen; i < len(data); i++ {
		if data[i] != byte(padLen) {
			return nil, fmt.Errorf("weixin: PKCS#7 padding bytes inconsistent")
		}
	}
	return data[:len(data)-padLen], nil
}
