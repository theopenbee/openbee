package platform

import (
	"fmt"
	"strings"
)

func SessionChatID(sessionKey, platformID string) (string, error) {
	parts := strings.SplitN(sessionKey, ":", 3)
	if len(parts) < 2 || parts[0] != platformID || parts[1] == "" {
		return "", fmt.Errorf("invalid %s session key %q", platformID, sessionKey)
	}
	return parts[1], nil
}
