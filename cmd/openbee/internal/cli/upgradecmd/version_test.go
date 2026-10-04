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
		{"V1.2.3", "v1.2.3", false},
		{" V1.2.3\n", "v1.2.3", false},
		{"  v1.2.3\n", "v1.2.3", false},
		{"\t1.2.3 ", "v1.2.3", false},
		{"", "", true},
		{"   \n", "", true},
		{"v", "", true},
		{"V", "", true},
		{" v\n", "", true},
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

	tag, version, err := fetchLatestVersion(srv.URL)
	if err != nil {
		t.Fatalf("fetchLatestVersion: %v", err)
	}
	if tag != "v1.2.3" || version != "v1.2.3" {
		t.Fatalf("fetchLatestVersion = (%q, %q), want (%q, %q)", tag, version, "v1.2.3", "v1.2.3")
	}
}

func TestFetchLatestVersionKeepsTagVerbatim(t *testing.T) {
	t.Setenv(githubTokenEnv, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":" V1.2.3\n"}`))
	}))
	defer srv.Close()

	tag, version, err := fetchLatestVersion(srv.URL)
	if err != nil {
		t.Fatalf("fetchLatestVersion: %v", err)
	}
	if tag != "V1.2.3" || version != "v1.2.3" {
		t.Fatalf("fetchLatestVersion = (%q, %q), want (%q, %q)", tag, version, "V1.2.3", "v1.2.3")
	}
}

func TestFetchLatestVersionRejectsBarePrefixTag(t *testing.T) {
	t.Setenv(githubTokenEnv, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"V"}`))
	}))
	defer srv.Close()

	if tag, _, err := fetchLatestVersion(srv.URL); err == nil {
		t.Fatalf("fetchLatestVersion with tag \"V\" = %q, want error", tag)
	}
}

func TestReleaseDownload(t *testing.T) {
	cases := []struct {
		tag             string
		wantBase        string
		wantArchiveName string
	}{
		{"v0.0.43", githubRelBase + "/v0.0.43", "openbee-0.0.43-linux-amd64.tar.gz"},
		// GitHub tags are case-sensitive and goreleaser's {{ .Version }} strips only
		// a lowercase "v", so an uppercase tag must reach both paths unchanged.
		{"V0.0.43", githubRelBase + "/V0.0.43", "openbee-V0.0.43-linux-amd64.tar.gz"},
		{"0.0.43", githubRelBase + "/0.0.43", "openbee-0.0.43-linux-amd64.tar.gz"},
	}
	for _, tc := range cases {
		base, archiveName := releaseDownload(tc.tag, "linux", "amd64")
		if base != tc.wantBase || archiveName != tc.wantArchiveName {
			t.Fatalf("releaseDownload(%q) = (%q, %q), want (%q, %q)", tc.tag, base, archiveName, tc.wantBase, tc.wantArchiveName)
		}
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

	if _, _, err := fetchLatestVersion(srv.URL); err != nil {
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

	_, _, err := fetchLatestVersion(srv.URL)
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

	_, _, err := fetchLatestVersion(srv.URL)
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

	_, _, err := fetchLatestVersion(srv.URL)
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

	_, _, err := fetchLatestVersion(srv.URL)
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("fetchLatestVersion against a stalled server: err = %v, want a timeout", err)
	}
}
