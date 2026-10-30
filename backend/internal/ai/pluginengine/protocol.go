package pluginengine

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/theopenbee/openbee/internal/plugin"
)

const ProtocolVersion = plugin.EngineProtocolVersion

const (
	methodRun   = "run"
	methodCheck = "check"
	methodUsage = "usage"
)

type request struct {
	ProtocolVersion int            `json:"protocol_version"`
	Method          string         `json:"method"`
	OpenbeeVersion  string         `json:"openbee_version"`
	EngineID        string         `json:"engine_id"`
	Language        string         `json:"language"`
	Config          map[string]any `json:"config"`
	DataDir         string         `json:"data_dir"`
	StateDir        string         `json:"state_dir"`
	Params          any            `json:"params"`
}

type runParams struct {
	Role      string     `json:"role"`
	WorkDir   string     `json:"work_dir"`
	Prompt    string     `json:"prompt"`
	SessionID string     `json:"session_id"`
	Resume    bool       `json:"resume"`
	ExtraArgs []string   `json:"extra_args"`
	LogPath   string     `json:"log_path"`
	MCP       *mcpParams `json:"mcp,omitempty"`
}

type mcpParams struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	APIKeyEnv string `json:"api_key_env"`
}

type usageParams struct {
	SessionIDs []string `json:"session_ids"`
}

type runResult struct {
	Text  string `json:"text"`
	Error string `json:"error"`
}

type CheckResult struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

type usageResult struct {
	Usage map[string][]usageEntry `json:"usage"`
}

type usageEntry struct {
	Model               string `json:"model"`
	InputTokens         int64  `json:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens"`
}

type response struct {
	Result json.RawMessage
	Err    string
}

func encodeRequest(spec Spec, method string, params any) ([]byte, error) {
	config := spec.Config
	if config == nil {
		config = map[string]any{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(request{
		ProtocolVersion: ProtocolVersion,
		Method:          method,
		OpenbeeVersion:  spec.OpenbeeVersion,
		EngineID:        spec.ID,
		Language:        spec.Language,
		Config:          config,
		DataDir:         spec.DataDir,
		StateDir:        spec.StateDir,
		Params:          params,
	})
	if err != nil {
		return nil, fmt.Errorf("encode engine request: %w", err)
	}
	return buf.Bytes(), nil
}

func parseResponse(line []byte) (*response, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(line, &fields) != nil {
		return nil, false
	}
	result, hasResult := fields["result"]
	rawErr, hasErr := fields["error"]
	if !hasResult && !hasErr {
		return nil, false
	}
	resp := &response{Result: result}
	if hasErr && !bytes.Equal(bytes.TrimSpace(rawErr), []byte("null")) {
		resp.Err = errorMessage(rawErr)
	}
	return resp, true
}

func errorMessage(raw json.RawMessage) string {
	var obj struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Message != "" {
		return obj.Message
	}
	var text string
	if json.Unmarshal(raw, &text) == nil && text != "" {
		return text
	}
	return string(raw)
}

func decodeResult(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}
