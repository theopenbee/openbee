package upgradecmd

import (
	"testing"
	"time"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest  string
		current string
		want    bool
	}{
		{"v1.2.1", "v1.2.0", true},
		{"v1.2.0", "v1.2.0", false},
		{"v1.2.0", "v1.3.0", false},
		{"dev", "dev", false},
	}
	for _, tc := range cases {
		if got := isNewer(tc.latest, tc.current); got != tc.want {
			t.Fatalf("isNewer(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestNormalizeVersionTag(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"v1.2.3", "v1.2.3", false},
		{"1.2.3", "v1.2.3", false},
		{"  v1.2.3\n", "v1.2.3", false},
		{"\t1.2.3 ", "v1.2.3", false},
		{"", "", true},
		{"   \n", "", true},
	}
	for _, tc := range cases {
		got, err := normalizeVersionTag(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("normalizeVersionTag(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("normalizeVersionTag(%q) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("normalizeVersionTag(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAPIClientTimeout(t *testing.T) {
	if apiClient.Timeout != 15*time.Second {
		t.Fatalf("apiClient.Timeout = %v, want 15s", apiClient.Timeout)
	}
}
