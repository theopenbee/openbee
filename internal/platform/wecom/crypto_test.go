package wecom

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// encryptForTest encrypts plaintext with AES-256-CBC using the WeCom scheme:
// IV = key[:16], PKCS#7 padding to 32-byte block multiples.
func encryptForTest(t *testing.T, plaintext, key []byte) []byte {
	t.Helper()
	iv := key[:16]
	block, err := aes.NewCipher(key)
	require.NoError(t, err)

	// PKCS#7 pad to 32-byte multiple
	padLen := 32 - (len(plaintext) % 32)
	padded := make([]byte, len(plaintext)+padLen)
	copy(padded, plaintext)
	for i := len(plaintext); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}

	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, padded)
	return encrypted
}

func TestDecryptFile_RoundTrip(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	aesKeyB64 := base64.StdEncoding.EncodeToString(key)

	plaintext := []byte("hello, wecom media content!")
	encrypted := encryptForTest(t, plaintext, key)

	got, err := DecryptFile(encrypted, aesKeyB64)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)
}

func TestDecryptFile_AlignedPlaintext(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	aesKeyB64 := base64.StdEncoding.EncodeToString(key)

	// Exactly 32 bytes — requires a full 32-byte padding block
	plaintext := make([]byte, 32)
	_, err = rand.Read(plaintext)
	require.NoError(t, err)
	encrypted := encryptForTest(t, plaintext, key)

	got, err := DecryptFile(encrypted, aesKeyB64)
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)
}

func TestDecryptFile_EmptyInput(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	aesKeyB64 := base64.StdEncoding.EncodeToString(key)

	_, err = DecryptFile([]byte{}, aesKeyB64)
	assert.Error(t, err)
}

func TestDecryptFile_InvalidBase64Key(t *testing.T) {
	_, err := DecryptFile([]byte("somedata"), "not-valid-base64!!!")
	assert.Error(t, err)
}

func TestDecryptFile_ShortKey(t *testing.T) {
	// 16-byte key encodes to 24 base64 chars; DecryptFile should reject non-32-byte keys.
	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)
	aesKeyB64 := base64.StdEncoding.EncodeToString(key)

	_, err = DecryptFile([]byte("anydatawontmatter"), aesKeyB64)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "32-byte key")
}
