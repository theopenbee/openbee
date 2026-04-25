package tokenstat

// SessionTokenUsage holds aggregated token usage for one (session, model) pair.
type SessionTokenUsage struct {
	SessionID           string
	AgentType           string
	Model               string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

// Parser reads a session's JSONL file(s) and returns per-model token usage.
type Parser interface {
	Parse(sessionID string) ([]SessionTokenUsage, error)
}
