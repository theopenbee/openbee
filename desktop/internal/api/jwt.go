package api

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

const expiryMargin = 60 * time.Second

func tokenExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp float64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(claims.Exp * 1000)), true
}

func needsRefresh(token string, now time.Time) bool {
	exp, ok := tokenExpiry(token)
	if !ok {
		return true
	}
	return !exp.After(now.Add(expiryMargin))
}
