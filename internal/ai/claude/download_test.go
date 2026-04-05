package claude

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMapArch(t *testing.T) {
	tests := []struct {
		goarch string
		want   string
	}{
		{"amd64", "x64"},
		{"arm64", "arm64"},
		{"386", "386"},
		{"riscv64", "riscv64"},
	}
	for _, tt := range tests {
		if got := mapArch(tt.goarch); got != tt.want {
			t.Errorf("mapArch(%q) = %q, want %q", tt.goarch, got, tt.want)
		}
	}
}

func TestIsMuslWith(t *testing.T) {
	found := isMuslWith(func(pattern string) ([]string, error) {
		return []string{"/lib/ld-musl-x86_64.so.1"}, nil
	})
	if !found {
		t.Error("expected true when musl linker found")
	}

	notFound := isMuslWith(func(pattern string) ([]string, error) {
		return nil, nil
	})
	if notFound {
		t.Error("expected false when no musl linker")
	}

	errCase := isMuslWith(func(pattern string) ([]string, error) {
		return nil, fmt.Errorf("permission denied")
	})
	if errCase {
		t.Error("expected false (fail-open) when glob errors")
	}
}

func TestIsSupportedPlatform(t *testing.T) {
	supported := []claudePlatform{
		{"darwin", "arm64", ""},
		{"darwin", "x64", ""},
		{"linux", "arm64", ""},
		{"linux", "x64", ""},
		{"linux", "arm64", "musl"},
		{"linux", "x64", "musl"},
	}
	for _, p := range supported {
		if !isSupportedPlatform(p) {
			t.Errorf("expected supported: %+v", p)
		}
	}

	unsupported := []claudePlatform{
		{"windows", "x64", ""},
		{"windows", "arm64", ""},
		{"freebsd", "x64", ""},
		{"linux", "386", ""},
		{"darwin", "x64", "musl"},
		{"darwin", "arm64", "musl"},
	}
	for _, p := range unsupported {
		if isSupportedPlatform(p) {
			t.Errorf("expected unsupported: %+v", p)
		}
	}
}

func TestBuildClaudeDownloadURL(t *testing.T) {
	const version = "v1.2.3"
	tests := []struct {
		platform claudePlatform
		want     string
	}{
		{
			claudePlatform{"darwin", "arm64", ""},
			gitHubRelBase + "/v1.2.3/claude-1.2.3-darwin-arm64",
		},
		{
			claudePlatform{"darwin", "x64", ""},
			gitHubRelBase + "/v1.2.3/claude-1.2.3-darwin-x64",
		},
		{
			claudePlatform{"linux", "x64", ""},
			gitHubRelBase + "/v1.2.3/claude-1.2.3-linux-x64",
		},
		{
			claudePlatform{"linux", "arm64", "musl"},
			gitHubRelBase + "/v1.2.3/claude-1.2.3-linux-arm64-musl",
		},
	}
	for _, tt := range tests {
		got := buildClaudeDownloadURL(tt.platform, version)
		if got != tt.want {
			t.Errorf("buildClaudeDownloadURL(%+v, %q) = %q, want %q", tt.platform, version, got, tt.want)
		}
	}
}

func TestFetchLatestClaudeVersion(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		status  int
		want    string
		wantErr bool
	}{
		{
			name:   "v-prefixed tag",
			body:   `{"tag_name": "v1.2.3"}`,
			status: http.StatusOK,
			want:   "v1.2.3",
		},
		{
			name:   "tag without v prefix",
			body:   `{"tag_name": "2.0.0"}`,
			status: http.StatusOK,
			want:   "v2.0.0",
		},
		{
			name:    "empty tag",
			body:    `{"tag_name": ""}`,
			status:  http.StatusOK,
			wantErr: true,
		},
		{
			name:    "non-200 status",
			body:    `{}`,
			status:  http.StatusNotFound,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()

			orig := GitHubAPI
			GitHubAPI = srv.URL
			defer func() { GitHubAPI = orig }()

			got, err := fetchLatestClaudeVersion("")
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectPlatform(t *testing.T) {
	p := detectPlatform()
	if p.os == "" {
		t.Error("detectPlatform() returned empty os")
	}
	if p.arch == "" {
		t.Error("detectPlatform() returned empty arch")
	}
	if !isSupportedPlatform(p) {
		t.Errorf("current platform %+v should be supported", p)
	}
}

