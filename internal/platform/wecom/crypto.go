package wecom

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"strings"
)

// DecryptFile decrypts a WeCom media file using AES-256-CBC.
//
// aesKeyBase64 is the Base64-encoded 256-bit (32-byte) key provided in the
// message body (image.aeskey / file.aeskey). The IV is the first 16 bytes of
// the decoded key. Padding is PKCS#7 with a 32-byte block size — WeCom pads to
// 32-byte multiples rather than the standard AES 16-byte block size, so
// auto-padding must be disabled and padding removed manually.
func DecryptFile(encrypted []byte, aesKeyBase64 string) ([]byte, error) {
	if len(encrypted) == 0 {
		return nil, fmt.Errorf("decryptFile: encrypted data is empty")
	}

	key, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(aesKeyBase64, "="))
	if err != nil {
		return nil, fmt.Errorf("decryptFile: decode aesKey: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("decryptFile: expected 32-byte key, got %d bytes", len(key))
	}

	iv := key[:16]
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("decryptFile: create cipher: %w", err)
	}

	if len(encrypted)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("decryptFile: ciphertext length %d is not a multiple of AES block size", len(encrypted))
	}

	decrypted := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(decrypted, encrypted)

	// Manual PKCS#7 unpadding — WeCom uses 32-byte blocks, not 16.
	padLen := int(decrypted[len(decrypted)-1])
	if padLen < 1 || padLen > 32 || padLen > len(decrypted) {
		return nil, fmt.Errorf("decryptFile: invalid PKCS#7 padding value: %d", padLen)
	}
	for i := len(decrypted) - padLen; i < len(decrypted); i++ {
		if decrypted[i] != byte(padLen) {
			return nil, fmt.Errorf("decryptFile: PKCS#7 padding bytes inconsistent")
		}
	}
	return decrypted[:len(decrypted)-padLen], nil
}
