package weixin

import (
	"crypto/aes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	plaintext := []byte("hello weixin media content!")
	encrypted := encryptAesEcb(plaintext, key)
	assert.Equal(t, 0, len(encrypted)%aes.BlockSize)

	decrypted, err := decryptAesEcb(encrypted, key)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncryptDecrypt_AlignedSize(t *testing.T) {
	key := make([]byte, 16)
	_, err := rand.Read(key)
	require.NoError(t, err)

	// Exactly 16 bytes — needs full padding block
	plaintext := make([]byte, 16)
	_, err = rand.Read(plaintext)
	require.NoError(t, err)

	encrypted := encryptAesEcb(plaintext, key)
	decrypted, err := decryptAesEcb(encrypted, key)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestDecryptAesEcb_EmptyInput(t *testing.T) {
	key := make([]byte, 16)
	_, err := decryptAesEcb([]byte{}, key)
	assert.Error(t, err)
}

func TestDecryptAesEcb_InvalidLength(t *testing.T) {
	key := make([]byte, 16)
	_, err := decryptAesEcb([]byte("not16aligned"), key)
	assert.Error(t, err)
}

func TestAesEcbPaddedSize(t *testing.T) {
	// aesEcbPaddedSize is the WeChat spec formula for reporting filesize to the API.
	// It uses ((n+1)/16+1)*16, which differs from standard PKCS7 at n=16k-1.
	// This is intentional — the server expects this formula.
	tests := []struct {
		input int
		want  int
	}{
		{0, 16},
		{1, 16},
		{14, 16},
		{15, 32},   // spec formula: (16/16+1)*16=32; standard PKCS7 would be 16
		{16, 32},
		{31, 32},
		{32, 48},
	}
	for _, tt := range tests {
		got := aesEcbPaddedSize(tt.input)
		assert.Equal(t, tt.want, got, "aesEcbPaddedSize(%d)", tt.input)
	}
}

// TestAesEcbPaddedSize_TypicalCases verifies the formula matches actual
// encryptAesEcb output sizes for typical inputs.
func TestAesEcbPaddedSize_TypicalCases(t *testing.T) {
	key := make([]byte, 16)
	rand.Read(key)
	for _, n := range []int{0, 1, 10, 16, 100, 1024} {
		data := make([]byte, n)
		encrypted := encryptAesEcb(data, key)
		reported := aesEcbPaddedSize(n)
		assert.GreaterOrEqual(t, reported, len(encrypted),
			"reported size should be >= actual encrypted size for n=%d", n)
	}
}

func TestParseAesKey_Raw16Bytes(t *testing.T) {
	rawKey := make([]byte, 16)
	_, err := rand.Read(rawKey)
	require.NoError(t, err)

	b64 := base64.StdEncoding.EncodeToString(rawKey)
	got, err := parseAesKey(b64)
	require.NoError(t, err)
	assert.Equal(t, rawKey, got)
}

func TestParseAesKey_HexString32Bytes(t *testing.T) {
	rawKey := make([]byte, 16)
	_, err := rand.Read(rawKey)
	require.NoError(t, err)

	hexStr := hex.EncodeToString(rawKey) // 32 ASCII bytes
	b64 := base64.StdEncoding.EncodeToString([]byte(hexStr))
	got, err := parseAesKey(b64)
	require.NoError(t, err)
	assert.Equal(t, rawKey, got)
}

func TestParseAesKey_InvalidBase64(t *testing.T) {
	_, err := parseAesKey("not-valid!!!")
	assert.Error(t, err)
}

func TestParseAesKey_WrongLength(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("too-short"))
	_, err := parseAesKey(b64)
	assert.Error(t, err)
}
