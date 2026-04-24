package model

type TokenStats struct {
	ID                  string `json:"id" db:"id"`
	SessionID           string `json:"session_id" db:"session_id"`
	AgentType           string `json:"agent_type" db:"agent_type"`
	Model               string `json:"model" db:"model"`
	InputTokens         int64  `json:"input_tokens" db:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens" db:"output_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens" db:"cache_creation_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens" db:"cache_read_tokens"`
	SyncedAt            int64  `json:"synced_at" db:"synced_at"`
}
