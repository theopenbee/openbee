package platform

import (
	"encoding/json"
	"sync"
)

func MarshalContext(name string, v any) string {
	b, _ := json.Marshal(map[string]any{name: v})
	return string(b)
}

var (
	extractorsMu sync.RWMutex
	extractors   = map[string]func(string) string{}
)

func RegisterExtractor(name string, fn func(string) string) {
	extractorsMu.Lock()
	extractors[name] = fn
	extractorsMu.Unlock()
}

func ExtractContext(platformName, raw string) string {
	if raw == "" {
		return ""
	}
	extractorsMu.RLock()
	fn, ok := extractors[platformName]
	extractorsMu.RUnlock()
	if !ok {
		return ""
	}
	return fn(raw)
}
