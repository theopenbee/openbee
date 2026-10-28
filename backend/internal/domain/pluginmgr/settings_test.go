package pluginmgr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/plugin"
)

func row(id, config string) model.PluginSettings {
	return model.PluginSettings{ID: id, Config: config}
}

const testSecret = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

func telegramManifest() plugin.Manifest {
	return plugin.Manifest{
		ProtocolVersion: 1,
		ID:              "telegram",
		Command:         []string{"node", "dist/index.js"},
		Config: []plugin.ConfigField{
			{Key: "token", Type: plugin.FieldSecret, Required: true},
			{Key: "auth_code", Type: plugin.FieldString, RandomDefault: true},
			{Key: "allowed_chats", Type: plugin.FieldList},
			{Key: "max_media_size", Type: plugin.FieldInt, Default: 10},
			{Key: "debug", Type: plugin.FieldBool},
		},
	}
}

func TestMergeConfig_KeepsSecretWhenEmpty(t *testing.T) {
	m := telegramManifest()
	current := map[string]any{"token": "123456:ABCDEFGH", "auth_code": "abc"}
	for _, update := range []map[string]any{{"token": ""}, {"token": nil}, {}} {
		got, err := MergeConfig(m, current, update)
		require.NoError(t, err)
		assert.Equal(t, "123456:ABCDEFGH", got["token"])
	}
	got, err := MergeConfig(m, current, map[string]any{"token": "new-token"})
	require.NoError(t, err)
	assert.Equal(t, "new-token", got["token"])
}

func TestMergeConfig_RejectsUnknownKey(t *testing.T) {
	_, err := MergeConfig(telegramManifest(), nil, map[string]any{"token": "t", "proxy": "x"})
	assert.Equal(t, CodeUnknownField, apperr.Code(err))
	assert.Equal(t, "proxy", apperr.Params(err)["key"])
}

func TestMergeConfig_RejectsReservedKey(t *testing.T) {
	for _, key := range []string{"enabled", "bot_name"} {
		_, err := MergeConfig(telegramManifest(), nil, map[string]any{"token": "t", key: "x"})
		assert.Equal(t, CodeUnknownField, apperr.Code(err), key)
	}
}

func TestMergeConfig_AppliesRandomDefaultOnce(t *testing.T) {
	m := telegramManifest()
	first, err := MergeConfig(m, nil, map[string]any{"token": "t"})
	require.NoError(t, err)
	code, _ := first["auth_code"].(string)
	assert.Len(t, code, 16)

	second, err := MergeConfig(m, first, map[string]any{"debug": true})
	require.NoError(t, err)
	assert.Equal(t, code, second["auth_code"])
}

func TestMergeConfig_ExplicitEmptyStaysEmpty(t *testing.T) {
	got, err := MergeConfig(telegramManifest(), nil, map[string]any{"token": "t", "auth_code": ""})
	require.NoError(t, err)
	assert.Equal(t, "", got["auth_code"])
}

func TestMergeConfig_ClearsNonSecretOnNull(t *testing.T) {
	current := map[string]any{"token": "t", "auth_code": "a", "allowed_chats": []any{"1"}}
	got, err := MergeConfig(telegramManifest(), current, map[string]any{"allowed_chats": nil})
	require.NoError(t, err)
	_, present := got["allowed_chats"]
	assert.False(t, present)
}

func TestMergeConfig_DropsFieldsTheManifestNoLongerDeclares(t *testing.T) {
	current := map[string]any{"token": "t", "auth_code": "a", "removed": "x"}
	got, err := MergeConfig(telegramManifest(), current, nil)
	require.NoError(t, err)
	_, present := got["removed"]
	assert.False(t, present)
}

func TestMergeConfig_RequiredMissing(t *testing.T) {
	_, err := MergeConfig(telegramManifest(), nil, map[string]any{"debug": true})
	assert.Equal(t, CodeConfigInvalid, apperr.Code(err))
	assert.Contains(t, err.Error(), "token")
}

func TestMergeConfig_WrongType(t *testing.T) {
	_, err := MergeConfig(telegramManifest(), nil, map[string]any{"token": "t", "max_media_size": "big"})
	assert.Equal(t, CodeConfigInvalid, apperr.Code(err))
}

func TestMaskConfig_MasksSecrets(t *testing.T) {
	got := MaskConfig(telegramManifest(), map[string]any{
		"token":     "123456:ABCDEFGH",
		"auth_code": "abc",
		"debug":     true,
		"removed":   "x",
	})
	assert.Equal(t, map[string]any{"token": "****EFGH", "auth_code": "abc", "debug": true}, got)
	assert.Equal(t, map[string]any{"token": "****"}, MaskConfig(telegramManifest(), map[string]any{"token": "short"}))
}

func TestMaskConfig_ShortSecretsFullyHidden(t *testing.T) {
	for _, secret := range []string{"s3cr3tpw", "abcdefghijk"} {
		got := MaskConfig(telegramManifest(), map[string]any{"token": secret})
		assert.Equal(t, "****", got["token"], secret)
	}
	got := MaskConfig(telegramManifest(), map[string]any{"token": "abcdefghijkl"})
	assert.Equal(t, "****ijkl", got["token"])
}

func TestMergeConfig_KeepsSecretWhenMaskedValueSentBack(t *testing.T) {
	m := telegramManifest()
	for _, secret := range []string{"123456:ABCDEFGHIJ", "short"} {
		current := map[string]any{"token": secret, "auth_code": "a"}
		masked := MaskConfig(m, current)["token"]
		got, err := MergeConfig(m, current, map[string]any{"token": masked})
		require.NoError(t, err)
		assert.Equal(t, secret, got["token"], secret)
	}
}

func TestCodec_RoundTrip(t *testing.T) {
	c, err := NewCodec(testSecret, true)
	require.NoError(t, err)
	in := Settings{Enabled: true, BotName: "bee", Config: map[string]any{"token": "secret-token", "max_media_size": 10.0, "allowed_chats": []any{"1"}}}
	row, err := c.Encode(in, "telegram")
	require.NoError(t, err)
	assert.Equal(t, "telegram", row.ID)
	assert.True(t, row.Enabled)
	assert.Equal(t, "bee", row.BotName)
	assert.NotContains(t, row.Config, "secret-token")

	out, err := c.Decode(row)
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

func TestCodec_DecodeEmptyConfig(t *testing.T) {
	c, err := NewCodec(testSecret, true)
	require.NoError(t, err)
	out, err := c.Decode(row("telegram", ""))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{}, out.Config)
}

func TestCodec_DecodeWithWrongKeyFails(t *testing.T) {
	c, err := NewCodec(testSecret, true)
	require.NoError(t, err)
	r, err := c.Encode(Settings{Config: map[string]any{"token": "t"}}, "telegram")
	require.NoError(t, err)
	other, err := NewCodec("ff02030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20", true)
	require.NoError(t, err)
	_, err = other.Decode(r)
	assert.Error(t, err)
}

func TestCodec_RefusesWithoutConfiguredSecret(t *testing.T) {
	c, err := NewCodec(testSecret, false)
	require.NoError(t, err)
	_, err = c.Encode(Settings{Config: map[string]any{"token": "t"}}, "telegram")
	assert.Equal(t, CodeEnvSecretMissing, apperr.Code(err))
}
