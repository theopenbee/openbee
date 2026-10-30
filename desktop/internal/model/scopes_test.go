package model

import (
	"slices"
	"testing"
)

func TestParseScopes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty string", "", nil},
		{"comma separated", "read:workers,read:tasks", []string{"read:workers", "read:tasks"}},
		{"trims whitespace around entries", "read:workers, read:tasks", []string{"read:workers", "read:tasks"}},
		{"filters out blank entries", ",read:workers,", []string{"read:workers"}},
		{"only separators and spaces", " , ,", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseScopes(tc.in); !slices.Equal(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestJoinScopes(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"joins scopes with comma", []string{"read:workers", "read:tasks"}, "read:workers,read:tasks"},
		{"empty slice", []string{}, ""},
		{"nil slice", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := JoinScopes(tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestScopesRoundTrip(t *testing.T) {
	scopes := []string{"read:workers", "read:tasks"}
	if got := ParseScopes(JoinScopes(scopes)); !slices.Equal(got, scopes) {
		t.Fatalf("got %q, want %q", got, scopes)
	}
}

func TestAllScopes(t *testing.T) {
	want := []string{"read:workers", "read:tasks", "read:messages"}
	if !slices.Equal(AllScopes, want) {
		t.Fatalf("got %q, want %q", AllScopes, want)
	}
}
