package pluginchannel

import (
	"fmt"
	"strings"
)

func BuildSessionKey(channelID, chatID, scope string) string {
	return channelID + ":" + chatID + ":" + scope
}

func SplitSessionKey(channelID, key string) (chatID, scope string, err error) {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) != 3 || parts[0] != channelID || parts[1] == "" || parts[2] == "" {
		return "", "", fmt.Errorf("invalid %s session key %q", channelID, key)
	}
	return parts[1], parts[2], nil
}
