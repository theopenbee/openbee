package model

// UsageRecord holds token consumption and cost for a single execution.
type UsageRecord struct {
	ID                  string  `json:"id" db:"id"`
	ExecutionID         string  `json:"execution_id" db:"execution_id"`
	Model               string  `json:"model" db:"model"`
	InputTokens         int64   `json:"input_tokens" db:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens" db:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens" db:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens" db:"cache_read_tokens"`
	TotalTokens         int64   `json:"total_tokens" db:"total_tokens"`
	CostUSD             float64 `json:"cost_usd" db:"cost_usd"`
	SyncedAt            int64   `json:"synced_at" db:"synced_at"`
}

// UnsyncedExecution is a lightweight query result used by UsageSyncer.
type UnsyncedExecution struct {
	ID      string
	LogPath string
}
