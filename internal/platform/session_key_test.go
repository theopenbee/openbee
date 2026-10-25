package platform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionChatID(t *testing.T) {
	cases := []struct {
		name       string
		sessionKey string
		platformID string
		want       string
		wantErr    bool
	}{
		{"BetaGroup", "beta:chat_123:user_456", "beta", "chat_123", false},
		{"TelegramNegativeChat", "telegram:-100123:42", "telegram", "-100123", false},
		{"ChatOnly", "local:user-1", "local", "user-1", false},
		{"WrongPlatform", "beta:chat_123:user_456", "gamma", "", true},
		{"MissingChat", "gamma::u1", "gamma", "", true},
		{"NoSeparator", "gamma", "gamma", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SessionChatID(tc.sessionKey, tc.platformID)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
