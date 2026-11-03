package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validKey returns a 32-byte (64 hex char) key for tests.
const validKey = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
const otherKey = "fffefdfcfbfaf9f8f7f6f5f4f3f2f1f0efeeedecebeae9e8e7e6e5e4e3e2e1e0"

func TestEncryptDecryptRoundtrip(t *testing.T) {
	gcm, err := NewGCM(validKey)
	require.NoError(t, err)

	plaintext := "hello, world!"
	ciphertext, err := EncryptGCM(gcm, plaintext)
	require.NoError(t, err)
	require.NotEmpty(t, ciphertext)

	decrypted, err := DecryptGCM(gcm, ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncryptDecryptEmptyString(t *testing.T) {
	gcm, err := NewGCM(validKey)
	require.NoError(t, err)

	plaintext := ""
	ciphertext, err := EncryptGCM(gcm, plaintext)
	require.NoError(t, err)

	decrypted, err := DecryptGCM(gcm, ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestDecryptFailsWithWrongKey(t *testing.T) {
	gcm, err := NewGCM(validKey)
	require.NoError(t, err)

	plaintext := "secret value"
	ciphertext, err := EncryptGCM(gcm, plaintext)
	require.NoError(t, err)

	gcmOther, err := NewGCM(otherKey)
	require.NoError(t, err)

	_, err = DecryptGCM(gcmOther, ciphertext)
	assert.Error(t, err, "decryption with wrong key should fail")
}

func TestEncryptProducesDifferentOutputs(t *testing.T) {
	gcm, err := NewGCM(validKey)
	require.NoError(t, err)

	plaintext := "same plaintext"
	ct1, err := EncryptGCM(gcm, plaintext)
	require.NoError(t, err)

	ct2, err := EncryptGCM(gcm, plaintext)
	require.NoError(t, err)

	assert.NotEqual(t, ct1, ct2, "two encryptions of same plaintext should differ due to random nonce")
}

func TestNewGCMInvalidKey(t *testing.T) {
	_, err := NewGCM("notahexkey")
	assert.Error(t, err)
}

func TestNewGCMInvalidKeyLength(t *testing.T) {
	// 48 hex chars = 24 bytes, should fail with a "got 24" message
	key24bytes := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f" // 64 chars = 32 bytes, adjust:
	key24bytes = key24bytes[:48]                                                        // 48 hex chars = 24 bytes
	_, err := NewGCM(key24bytes)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "got 24")
}

func TestDecryptInvalidBase64(t *testing.T) {
	gcm, err := NewGCM(validKey)
	require.NoError(t, err)

	_, err = DecryptGCM(gcm, "!!!not-base64!!!")
	assert.Error(t, err)
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	gcm, err := NewGCM(validKey)
	require.NoError(t, err)

	plaintext := "tamper me"
	ciphertext, err := EncryptGCM(gcm, plaintext)
	require.NoError(t, err)

	// Flip the last character to tamper with the authentication tag.
	tampered := ciphertext[:len(ciphertext)-1] + "X"
	if tampered == ciphertext {
		tampered = ciphertext[:len(ciphertext)-1] + "Y"
	}
	_, err = DecryptGCM(gcm, tampered)
	assert.Error(t, err, "decryption of tampered ciphertext should fail")
}

func TestMaskShortString(t *testing.T) {
	// len < 8 -> fully masked
	assert.Equal(t, "****", Mask(""))
	assert.Equal(t, "****", Mask("abc"))
	assert.Equal(t, "****", Mask("short"))   // 5 chars
	assert.Equal(t, "****", Mask("1234567")) // 7 chars
}

func TestMaskExactlyEight(t *testing.T) {
	// len == 8 -> first4 + "****" + last4
	assert.Equal(t, "exac****tly8", Mask("exactly8"))
}

func TestMaskLongString(t *testing.T) {
	// "mypassword123" -> 13 chars, first4="mypa", last4="d123"
	assert.Equal(t, "mypa****d123", Mask("mypassword123"))
}

func TestMaskUnicode(t *testing.T) {
	// 8 rune string with multibyte chars
	value := "你好世界ABCD" // 8 runes
	result := Mask(value)
	assert.Equal(t, "你好世界****ABCD", result)
}
