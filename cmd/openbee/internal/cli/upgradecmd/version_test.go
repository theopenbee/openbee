package upgradecmd

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		got := isNewer(tc.latest, tc.current)
		require.Equal(t, tc.want, got, "isNewer(%q, %q)", tc.latest, tc.current)
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
			require.Error(t, err, "normalizeVersionTag(%q) = %q, want error", tc.in, got)
			continue
		}
		require.NoError(t, err, "normalizeVersionTag(%q)", tc.in)
		require.Equal(t, tc.want, got, "normalizeVersionTag(%q)", tc.in)
	}
}

func TestFetchLatestVersion(t *testing.T) {
	t.Setenv(githubTokenEnv, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"), "want none without %s", githubTokenEnv)
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer srv.Close()

	tag, version, err := fetchLatestVersion(srv.URL)
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", tag)
	require.Equal(t, "v1.2.3", version)
}

func TestFetchLatestVersionKeepsTagVerbatim(t *testing.T) {
	t.Setenv(githubTokenEnv, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":" V1.2.3\n"}`))
	}))
	defer srv.Close()

	tag, version, err := fetchLatestVersion(srv.URL)
	require.NoError(t, err)
	require.Equal(t, "V1.2.3", tag)
	require.Equal(t, "v1.2.3", version)
}

func TestFetchLatestVersionRejectsBarePrefixTag(t *testing.T) {
	t.Setenv(githubTokenEnv, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"V"}`))
	}))
	defer srv.Close()

	tag, _, err := fetchLatestVersion(srv.URL)
	require.Error(t, err, "fetchLatestVersion with tag \"V\" = %q, want error", tag)
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
		require.Equal(t, tc.wantBase, base, "releaseDownload(%q) base", tc.tag)
		require.Equal(t, tc.wantArchiveName, archiveName, "releaseDownload(%q) archiveName", tc.tag)
	}
}

func TestFetchLatestVersionSendsToken(t *testing.T) {
	t.Setenv(githubTokenEnv, "test-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer srv.Close()

	_, _, err := fetchLatestVersion(srv.URL)
	require.NoError(t, err)
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
	require.Error(t, err, "fetchLatestVersion on rate limit returned nil error")
	for _, want := range []string{"rate limit", githubTokenEnv} {
		require.Contains(t, err.Error(), want, "rate-limit error does not mention %q", want)
	}
}

func TestFetchLatestVersionPlainForbidden(t *testing.T) {
	t.Setenv(githubTokenEnv, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	_, _, err := fetchLatestVersion(srv.URL)
	require.Error(t, err, "plain 403: want a non-rate-limit error mentioning 403")
	require.NotContains(t, err.Error(), "rate limit")
	require.Contains(t, err.Error(), "403")
}

func TestFetchLatestVersionBadToken(t *testing.T) {
	t.Setenv(githubTokenEnv, "expired-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, _, err := fetchLatestVersion(srv.URL)
	require.Error(t, err, "401 with token set: want an error pointing at %s", githubTokenEnv)
	require.Contains(t, err.Error(), githubTokenEnv)
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
	require.ErrorAs(t, err, &netErr, "fetchLatestVersion against a stalled server: want a timeout")
	require.True(t, netErr.Timeout())
}
