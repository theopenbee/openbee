package model

import (
	"slices"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func TestNormalizeEnvKey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"uppercases and drops invalid characters", "db-url 1", "DBURL1"},
		{"keeps underscores", "a_b", "A_B"},
		{"already normal", "DATABASE_URL", "DATABASE_URL"},
		{"drops punctuation and symbols", "a.b/c:d=e", "ABCDE"},
		{"drops non-ascii letters", "clé_ü", "CL_"},
		{"empty", "", ""},
		{"only invalid characters", " -.", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeEnvKey(tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func envConfig(key, masked string) api.EnvConfig {
	return api.EnvConfig{ID: "e-" + key, Key: key, Masked: masked}
}

func TestEnvKeyExists(t *testing.T) {
	existing := []api.EnvConfig{envConfig("API_KEY", "****"), envConfig("DB_URL", "****")}
	tests := []struct {
		name     string
		key      string
		existing []api.EnvConfig
		want     bool
	}{
		{"present", "DB_URL", existing, true},
		{"absent", "OTHER", existing, false},
		{"case sensitive", "db_url", existing, false},
		{"empty key never exists", "", existing, false},
		{"empty list", "DB_URL", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := EnvKeyExists(tc.key, tc.existing); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEffectiveEnvWorkerOverrides(t *testing.T) {
	global := []api.EnvConfig{envConfig("A", "g-a"), envConfig("B", "g-b")}
	worker := []api.EnvConfig{envConfig("B", "w-b")}
	got := EffectiveEnv(global, worker)
	want := []EffectiveVar{
		{Key: "A", Masked: "g-a", Source: "global"},
		{Key: "B", Masked: "w-b", Source: "worker"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestEffectiveEnvMergesAndSorts(t *testing.T) {
	global := []api.EnvConfig{envConfig("ZED", "z"), envConfig("MID", "m")}
	worker := []api.EnvConfig{envConfig("ALPHA", "a")}
	got := EffectiveEnv(global, worker)
	keys := make([]string, len(got))
	for i, v := range got {
		keys[i] = v.Key
	}
	if want := []string{"ALPHA", "MID", "ZED"}; !slices.Equal(keys, want) {
		t.Fatalf("got %v, want %v", keys, want)
	}
	if got[0].Source != "worker" || got[1].Source != "global" || got[2].Source != "global" {
		t.Fatalf("unexpected sources: %v", got)
	}
}

func TestEffectiveEnvCollation(t *testing.T) {
	var global []api.EnvConfig
	for _, key := range []string{"AWSKEY", "AWS2_KEY", "B", "AWS_KEY", "_X", "1X", "A", "A1", "A-", "A_"} {
		global = append(global, envConfig(key, "m"))
	}
	got := EffectiveEnv(global, nil)
	keys := make([]string, len(got))
	for i, v := range got {
		keys[i] = v.Key
	}
	want := []string{"_X", "1X", "A", "A_", "A-", "A1", "AWS_KEY", "AWS2_KEY", "AWSKEY", "B"}
	if !slices.Equal(keys, want) {
		t.Fatalf("got %v, want %v", keys, want)
	}
}

func TestEffectiveEnvEmpty(t *testing.T) {
	if got := EffectiveEnv(nil, nil); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

func TestHasEdgeSpace(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"value", false},
		{"in side", false},
		{" lead", true},
		{"trail ", true},
		{"tab\t", true},
		{"line\n", true},
		{" ", true},
	}
	for _, tc := range tests {
		if got := HasEdgeSpace(tc.in); got != tc.want {
			t.Errorf("HasEdgeSpace(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
