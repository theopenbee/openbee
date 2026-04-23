package usage

import "encoding/json"

type claudeResultEvent struct {
	Type         string            `json:"type"`
	TotalCostUSD float64           `json:"total_cost_usd"`
	Usage        claudeUsageFields `json:"usage"`
}

type claudeUsageFields struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

type claudeAssistantEvent struct {
	Message struct {
		Model string `json:"model"`
	} `json:"message"`
}

func extractClaudeModel(line string) string {
	var event claudeAssistantEvent
	if json.Unmarshal([]byte(line), &event) != nil {
		return ""
	}
	return event.Message.Model
}

func extractClaudeResult(line string, data *UsageData) {
	var event claudeResultEvent
	if json.Unmarshal([]byte(line), &event) != nil {
		return
	}
	data.InputTokens = event.Usage.InputTokens
	data.OutputTokens = event.Usage.OutputTokens
	data.CacheCreationTokens = event.Usage.CacheCreationInputTokens
	data.CacheReadTokens = event.Usage.CacheReadInputTokens
	data.TotalTokens = data.InputTokens + data.OutputTokens + data.CacheCreationTokens + data.CacheReadTokens
	data.CostUSD = event.TotalCostUSD
}
