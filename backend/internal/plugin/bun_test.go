package plugin

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBunTarget(t *testing.T) {
	cases := []struct {
		goos, goarch string
		avx2, musl   bool
		want         string
	}{
		{"darwin", "arm64", false, false, "darwin-aarch64"},
		{"darwin", "amd64", true, false, "darwin-x64"},
		{"darwin", "amd64", false, false, "darwin-x64-baseline"},
		{"linux", "amd64", true, false, "linux-x64"},
		{"linux", "amd64", false, false, "linux-x64-baseline"},
		{"linux", "amd64", true, true, "linux-x64-musl"},
		{"linux", "amd64", false, true, "linux-x64-musl-baseline"},
		{"linux", "arm64", false, false, "linux-aarch64"},
		{"linux", "arm64", false, true, "linux-aarch64-musl"},
		{"windows", "amd64", true, false, "windows-x64"},
		{"windows", "amd64", false, false, "windows-x64-baseline"},
		{"windows", "arm64", false, false, "windows-aarch64"},
		{"freebsd", "amd64", true, false, "freebsd-x64"},
		{"freebsd", "arm64", false, false, "freebsd-aarch64"},
		{"darwin", "arm64", false, true, "darwin-aarch64"},
	}
	for _, tc := range cases {
		got, err := bunTarget(tc.goos, tc.goarch, tc.avx2, tc.musl)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got, "%s/%s avx2=%v musl=%v", tc.goos, tc.goarch, tc.avx2, tc.musl)
	}

	for _, p := range [][2]string{{"linux", "386"}, {"linux", "riscv64"}, {"openbsd", "amd64"}, {"plan9", "arm64"}} {
		_, err := bunTarget(p[0], p[1], true, false)
		assert.True(t, errors.Is(err, ErrUnsupportedPlatform), "%s/%s: %v", p[0], p[1], err)
	}
}
