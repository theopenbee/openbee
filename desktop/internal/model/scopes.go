package model

import "strings"

var AllScopes = []string{"read:workers", "read:tasks", "read:messages"}

func ParseScopes(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if scope := strings.TrimSpace(part); scope != "" {
			out = append(out, scope)
		}
	}
	return out
}

func JoinScopes(scopes []string) string {
	return strings.Join(scopes, ",")
}
