package model

import (
	"strings"
	"unicode"
	"unicode/utf16"
)

const (
	avatarColors = 8
	fnvOffset    = 0x811c9dc5
	fnvPrime     = 0x01000193
	cjkFirst     = 0x4e00
	cjkLast      = 0x9fff
)

func AvatarIndex(id string) int {
	hash := uint32(fnvOffset)
	for _, unit := range utf16.Encode([]rune(id)) {
		hash ^= uint32(unit)
		hash *= fnvPrime
	}
	return int(hash % avatarColors)
}

func Initials(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "?"
	}
	runes := []rune(trimmed)
	if runes[0] >= cjkFirst && runes[0] <= cjkLast {
		return string(runes[0])
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return unicode.IsSpace(r) || r == '_' || r == '-'
	})
	if len(parts) >= 2 {
		return strings.ToUpper(string([]rune(parts[0])[0]) + string([]rune(parts[1])[0]))
	}
	return strings.ToUpper(string(runes[:min(2, len(runes))]))
}
