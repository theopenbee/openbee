package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testManifest() Manifest {
	return Manifest{
		ProtocolVersion: 1,
		ID:              "discord",
		Command:         []string{"node", "index.js"},
		Config: []ConfigField{
			{Key: "token", Type: FieldSecret, Required: true},
			{Key: "allowed_guilds", Type: FieldList},
			{Key: "max_media_size", Type: FieldInt, Default: float64(10485760)},
			{Key: "dm_only", Type: FieldBool, Default: false},
			{Key: "prefix", Type: FieldString},
		},
	}
}

func TestResolveConfig(t *testing.T) {
	cases := []struct {
		name    string
		section map[string]any
		check   func(t *testing.T, got map[string]any)
		wantErr string
	}{
		{
			name:    "defaults applied and reserved keys dropped",
			section: map[string]any{"enabled": true, "bot_name": "DBot", "token": "x"},
			check: func(t *testing.T, got map[string]any) {
				assert.Equal(t, "x", got["token"])
				assert.Equal(t, 10485760, got["max_media_size"])
				assert.Equal(t, false, got["dm_only"])
				assert.NotContains(t, got, "enabled")
				assert.NotContains(t, got, "bot_name")
				assert.NotContains(t, got, "prefix")
			},
		},
		{
			name:    "missing required secret",
			section: map[string]any{"enabled": true},
			wantErr: "token",
		},
		{
			name:    "empty required secret",
			section: map[string]any{"token": ""},
			wantErr: "token",
		},
		{
			name:    "list coerced to strings",
			section: map[string]any{"token": "x", "allowed_guilds": []any{"1", "2"}},
			check: func(t *testing.T, got map[string]any) {
				assert.Equal(t, []string{"1", "2"}, got["allowed_guilds"])
			},
		},
		{
			name:    "list with non-string item",
			section: map[string]any{"token": "x", "allowed_guilds": []any{"1", map[string]any{}}},
			wantErr: "allowed_guilds",
		},
		{
			name:    "int from yaml int",
			section: map[string]any{"token": "x", "max_media_size": 42},
			check: func(t *testing.T, got map[string]any) {
				assert.Equal(t, 42, got["max_media_size"])
			},
		},
		{
			name:    "int rejects string",
			section: map[string]any{"token": "x", "max_media_size": "abc"},
			wantErr: "max_media_size",
		},
		{
			name:    "int rejects fraction",
			section: map[string]any{"token": "x", "max_media_size": 1.5},
			wantErr: "max_media_size",
		},
		{
			name:    "bool rejects string",
			section: map[string]any{"token": "x", "dm_only": "yes"},
			wantErr: "dm_only",
		},
		{
			name:    "string rejects number",
			section: map[string]any{"token": 123},
			wantErr: "token",
		},
		{
			name:    "unknown key kept",
			section: map[string]any{"token": "x", "extra": "y"},
			check: func(t *testing.T, got map[string]any) {
				assert.Equal(t, "y", got["extra"])
			},
		},
		{
			name:    "nil section",
			section: nil,
			wantErr: "token",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveConfig(testManifest(), tc.section)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.check(t, got)
		})
	}
}

func TestResolveConfig_RequiredList(t *testing.T) {
	m := Manifest{ProtocolVersion: 1, ID: "x1", Command: []string{"a"}, Config: []ConfigField{{Key: "ids", Type: FieldList, Required: true}}}
	_, err := ResolveConfig(m, map[string]any{"ids": []any{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ids")
}
