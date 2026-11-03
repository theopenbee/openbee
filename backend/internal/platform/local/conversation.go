package local

import (
	"fmt"
	"strings"
)

const BeeConversationID = "bee"

const workerConversationPrefix = "worker:"

func WorkerConversationID(workerID string) string {
	return workerConversationPrefix + workerID
}

func WorkerIDFromConversation(conversationID string) (string, bool) {
	id, ok := strings.CutPrefix(conversationID, workerConversationPrefix)
	if !ok || id == "" || strings.Contains(id, ":") {
		return "", false
	}
	return id, true
}

func WorkerSessionKey(userID, workerID string) string {
	return SessionKey(userID) + ":" + WorkerConversationID(workerID)
}

func ConversationSessionKey(userID, conversationID string) (string, error) {
	if conversationID == BeeConversationID {
		return SessionKey(userID), nil
	}
	if workerID, ok := WorkerIDFromConversation(conversationID); ok {
		return WorkerSessionKey(userID, workerID), nil
	}
	return "", fmt.Errorf("unknown conversation %q", conversationID)
}

func ParseSessionKey(sessionKey string) (userID, conversationID string, ok bool) {
	rest, ok := strings.CutPrefix(sessionKey, sessionKeyPrefix)
	if !ok {
		return "", "", false
	}
	userID, tail, hasTail := strings.Cut(rest, ":")
	if userID == "" {
		return "", "", false
	}
	if !hasTail {
		return userID, BeeConversationID, true
	}
	if _, ok := WorkerIDFromConversation(tail); !ok {
		return "", "", false
	}
	return userID, tail, true
}

func IsBeeSessionKey(sessionKey string) bool {
	_, conversationID, ok := ParseSessionKey(sessionKey)
	return ok && conversationID == BeeConversationID
}

func WorkerIDFromSessionKey(sessionKey string) (string, bool) {
	_, conversationID, ok := ParseSessionKey(sessionKey)
	if !ok {
		return "", false
	}
	return WorkerIDFromConversation(conversationID)
}

func WorkerScopedSessionKey(sessionKey, workerID string) string {
	userID, conversationID, ok := ParseSessionKey(sessionKey)
	if !ok || conversationID != BeeConversationID {
		return sessionKey
	}
	return WorkerSessionKey(userID, workerID)
}
