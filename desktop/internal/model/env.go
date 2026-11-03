package model

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const (
	sourceGlobal = "global"
	sourceWorker = "worker"
)

type EffectiveVar struct{ Key, Masked, Source string }

func NormalizeEnvKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func EnvKeyExists(key string, existing []api.EnvConfig) bool {
	if key == "" {
		return false
	}
	return slices.ContainsFunc(existing, func(e api.EnvConfig) bool { return e.Key == key })
}

func EffectiveEnv(global, worker []api.EnvConfig) []EffectiveVar {
	merged := make(map[string]EffectiveVar, len(global)+len(worker))
	for _, e := range global {
		merged[e.Key] = EffectiveVar{Key: e.Key, Masked: e.Masked, Source: sourceGlobal}
	}
	for _, e := range worker {
		merged[e.Key] = EffectiveVar{Key: e.Key, Masked: e.Masked, Source: sourceWorker}
	}
	out := make([]EffectiveVar, 0, len(merged))
	for _, v := range merged {
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b EffectiveVar) int { return compareEnvKeys(a.Key, b.Key) })
	return out
}

func HasEdgeSpace(v string) bool {
	return v != strings.TrimSpace(v)
}

func envKeyRank(r rune) int {
	switch {
	case r == '_':
		return 0
	case unicode.IsDigit(r):
		return 2
	case unicode.IsLetter(r):
		return 3
	}
	return 1
}

func compareEnvKeys(a, b string) int {
	ra, rb := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	for i := 0; i < len(ra) && i < len(rb); i++ {
		if ra[i] == rb[i] {
			continue
		}
		if c := cmp.Compare(envKeyRank(ra[i]), envKeyRank(rb[i])); c != 0 {
			return c
		}
		return cmp.Compare(ra[i], rb[i])
	}
	if c := cmp.Compare(len(ra), len(rb)); c != 0 {
		return c
	}
	return strings.Compare(b, a)
}
