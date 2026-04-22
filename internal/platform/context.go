package platform

import (
	"encoding/json"
	"sync"
)

// BuildPlatformContext encodes platform-native fields as a JSON string keyed by platform name.
func BuildPlatformContext(name string, fields map[string]string) string {
	b, _ := json.Marshal(map[string]map[string]string{name: fields})
	return string(b)
}

var (
	extractorsMu sync.RWMutex
	extractors   = map[string]func(string) string{}
)

// RegisterExtractor registers a platform-specific context extractor.
// Called once at server startup per enabled platform.
func RegisterExtractor(name string, fn func(string) string) {
	extractorsMu.Lock()
	extractors[name] = fn
	extractorsMu.Unlock()
}

// ExtractContext returns a platform_context JSON string for the given platform
// and raw event payload. Returns "" if no extractor is registered or raw is empty.
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
