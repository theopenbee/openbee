package tokenstat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/theopenbee/openbee/internal/infra/config"
)

type piParser struct {
	sessionsDir string
}

func NewPiParser() Parser {
	dir := os.Getenv("PI_AGENT_DIR")
	if dir == "" {
		dir = config.DefaultPiSessionsDir()
	}
	return &piParser{sessionsDir: dir}
}

type piJSONLLine struct {
	Type    string `json:"type"`
	Message struct {
		Role  string `json:"role"`
		Model string `json:"model"`
		Usage *struct {
			Input      int64 `json:"input"`
			Output     int64 `json:"output"`
			CacheWrite int64 `json:"cacheWrite"`
			CacheRead  int64 `json:"cacheRead"`
		} `json:"usage"`
	} `json:"message"`
}

func (p *piParser) Parse(sessionID string) ([]SessionTokenUsage, error) {
	entries, err := os.ReadDir(p.sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrSessionDataNotFound, p.sessionsDir)
		}
		return nil, fmt.Errorf("read pi sessions dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".jsonl")
		idx := strings.Index(name, "_")
		if idx == -1 || name[idx+1:] != sessionID {
			continue
		}
		return piParse(sessionID, filepath.Join(p.sessionsDir, entry.Name()))
	}
	return nil, fmt.Errorf("%w: pi session file not found for session %s", ErrSessionDataNotFound, sessionID)
}

func piParse(sessionID, path string) ([]SessionTokenUsage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open pi session file: %w", err)
	}
	defer f.Close()

	agg := map[string]*SessionTokenUsage{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	for scanner.Scan() {
		var line piJSONLLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		if line.Type != "message" || line.Message.Role != "assistant" || line.Message.Usage == nil {
			continue
		}
		m := "[pi]" + line.Message.Model
		u := getOrCreate(agg, sessionID, "pi", m)
		u.InputTokens += line.Message.Usage.Input
		u.OutputTokens += line.Message.Usage.Output
		u.CacheCreationTokens += line.Message.Usage.CacheWrite
		u.CacheReadTokens += line.Message.Usage.CacheRead
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan pi session file: %w", err)
	}
	return mapValues(agg), nil
}
