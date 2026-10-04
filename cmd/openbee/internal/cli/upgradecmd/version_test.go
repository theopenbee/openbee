package upgradecmd

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestFetchLatestVersion(t *testing.T) {
	t.Setenv(githubTokenEnv, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want none without %s", got, githubTokenEnv)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer srv.Close()

	got, err := fetchLatestVersion(srv.URL)
	if err != nil {
		t.Fatalf("fetchLatestVersion: %v", err)
	}
	if got != "v1.2.3" {
		t.Fatalf("fetchLatestVersion = %q, want %q", got, "v1.2.3")
	}
}

func TestFetchLatestVersionSendsToken(t *testing.T) {
	t.Setenv(githubTokenEnv, "test-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer srv.Close()

	if _, err := fetchLatestVersion(srv.URL); err != nil {
		t.Fatalf("fetchLatestVersion: %v", err)
	}
}

func TestFetchLatestVersionRateLimited(t *testing.T) {
	t.Setenv(githubTokenEnv, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1767225600")
		http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := fetchLatestVersion(srv.URL)
	if err == nil {
		t.Fatalf("fetchLatestVersion on rate limit returned nil error")
	}
	for _, want := range []string{"rate limit", githubTokenEnv} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("rate-limit error %q does not mention %q", err, want)
		}
	}
}

func TestFetchLatestVersionPlainForbidden(t *testing.T) {
	t.Setenv(githubTokenEnv, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := fetchLatestVersion(srv.URL)
	if err == nil || strings.Contains(err.Error(), "rate limit") || !strings.Contains(err.Error(), "403") {
		t.Fatalf("plain 403: err = %v, want a non-rate-limit error mentioning 403", err)
	}
}

func TestFetchLatestVersionBadToken(t *testing.T) {
	t.Setenv(githubTokenEnv, "expired-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := fetchLatestVersion(srv.URL)
	if err == nil || !strings.Contains(err.Error(), githubTokenEnv) {
		t.Fatalf("401 with token set: err = %v, want an error pointing at %s", err, githubTokenEnv)
	}
}

func TestFetchLatestVersionTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	orig := apiClient
	apiClient = &http.Client{Timeout: 100 * time.Millisecond}
	t.Cleanup(func() { apiClient = orig })

	_, err := fetchLatestVersion(srv.URL)
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("fetchLatestVersion against a stalled server: err = %v, want a timeout", err)
	}
}
