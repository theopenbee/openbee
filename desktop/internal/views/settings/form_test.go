package settings

import "testing"

func TestDefaultTextMatchesTheWebHint(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"missing", nil, ""},
		{"string", "https://api.telegram.org", "https://api.telegram.org"},
		{"small int", float64(30), "30"},
		{"million", float64(1000000), "1000000"},
		{"billion", float64(1e9), "1000000000"},
		{"fraction", 0.25, "0.25"},
		{"negative", float64(-1500000), "-1500000"},
		{"bool", true, "true"},
		{"list", []any{"general", float64(2000000), true}, "general,2000000,true"},
		{"list with null", []any{"a", nil, "b"}, "a,,b"},
		{"empty list", []any{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultText(tc.in); got != tc.want {
				t.Fatalf("defaultText(%#v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
