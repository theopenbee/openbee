package ai

import "errors"

type TokenUsage struct {
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

var ErrSessionDataNotFound = errors.New("ai: session data not found")
