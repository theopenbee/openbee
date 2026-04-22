package platform

import "encoding/json"

// BuildPlatformContext encodes platform-native fields as a JSON string keyed by platform name.
func BuildPlatformContext(name string, fields map[string]string) string {
	b, _ := json.Marshal(map[string]map[string]string{name: fields})
	return string(b)
}
