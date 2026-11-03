package plugin

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/cpu"
)

const BunVersion = "1.4.4"

var DefaultBunMirrors = []string{
	"https://github.com/oven-sh/bun/releases/download",
	"https://registry.npmmirror.com/-/binary/bun",
}

var bunChecksums = map[string]string{
	"darwin-aarch64":          "c240d7f0ccb90c263a849fcdb09b7ee62630d921348ed6aafda86b9d577bc510",
	"darwin-x64":              "50a3491f73abc2fe81673171a85762d3bc2cc1680ccecadd983de99adf77977e",
	"darwin-x64-baseline":     "d4a155adfbfd09155eb3ba05c9e3a544f39297d7a68616d3c7759f67ab5c47dc",
	"freebsd-aarch64":         "b1f3c95d82c1fac38ccc3cad3da1a80e30298ebfd999218d6a4fc05b34325337",
	"freebsd-x64":             "98f7672d5d0604161414f8b05ba01af9c17cf5abfcd37462737046bf76839ae9",
	"freebsd-x64-baseline":    "83bb4d3962a87cb107b75141ac71b8b1b8ab4dad690b1b71a1a0f372c4549c5b",
	"linux-aarch64":           "baa6bb36f51d6e58561e02bbea825038a9df454f673d6d776e8bbf86f8c7850e",
	"linux-aarch64-musl":      "57f01b02ac7113c61c935f28f38c2eab849e3c970dab21f8b640a87b1ef65009",
	"linux-x64":               "bf8ccbfb2b0b80159e0f9a6d15c55317add8077534dbc6130a3edb52fccb395c",
	"linux-x64-baseline":      "e0446eb55995772eb9894bebae9a6be368d985a8b9271ee124486f9946eedd76",
	"linux-x64-musl":          "a5a8ccc5e74dcb883fe974f125a7bdb3b41e5195ed1b870e73f17effd3eb2b2d",
	"linux-x64-musl-baseline": "6e9c37098b293920aed28bfb51ab240e26d23bfdd9bbfecb3ad428ac166549bc",
	"windows-aarch64":         "2d57a0eec0cb44d95c918c3f9734f60455a1306ae5085d76fc68f44e3ee30d09",
	"windows-x64":             "cd2c0fccc78f3ede1f50ca827a15f063eda13d4d150a3d0674f4f6e323d864a5",
	"windows-x64-baseline":    "0f20ce6674d4f2dbcc8c5d59fdf99f7ba45f8c3b367e0f5781cbfa30a1c78d99",
}

var (
	ErrUnsupportedPlatform = errors.New("no Bun build for this platform")
	ErrBunDownload         = errors.New("download Bun " + BunVersion)
	errBunUnusable         = errors.New("downloaded bun does not run")
	errDownloadStalled     = errors.New("no data received for too long")
)

var bunStallTimeout = 30 * time.Second

const staleTempAge = 10 * time.Minute

const maxBunArchiveSize = 512 << 20

func bunTarget(goos, goarch string, avx2, musl bool) (string, error) {
	arch := map[string]string{"amd64": "x64", "arm64": "aarch64"}[goarch]
	target := goos + "-" + arch
	if goos == "linux" && musl {
		target += "-musl"
	}
	if arch == "x64" && !avx2 {
		target += "-baseline"
	}
	if _, ok := bunChecksums[target]; !ok || arch == "" {
		return "", fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, goos, goarch)
	}
	return target, nil
}

func hasAVX2() bool {
	return cpu.X86.HasAVX2
}

func isMusl() bool {
	musl, _ := filepath.Glob("/lib/ld-musl-*.so.1")
	if len(musl) == 0 {
		return false
	}
	for _, pattern := range []string{"/lib/ld-linux-*.so.*", "/lib64/ld-linux-*.so.*"} {
		if glibc, _ := filepath.Glob(pattern); len(glibc) > 0 {
			return false
		}
	}
	return true
}

func installBun(ctx context.Context, client *http.Client, mirrors []string, target, dest string) error {
	var errs []error
	for _, mirror := range mirrors {
		err := downloadBun(ctx, client, mirror, target, dest)
		if err == nil {
			return nil
		}
		errs = append(errs, err)
		if ctx.Err() != nil || errors.Is(err, errBunUnusable) {
			break
		}
	}
	return errors.Join(errs...)
}

func downloadBun(ctx context.Context, client *http.Client, mirror, target, dest string) error {
	src := strings.TrimRight(mirror, "/") + "/bun-v" + BunVersion + "/bun-" + target + ".zip"
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	removeStaleTemps(dir)
	archive, err := os.CreateTemp(dir, ".bun-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	h := sha256.New()
	if err := fetch(ctx, client, src, io.MultiWriter(archive, h)); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != bunChecksums[target] {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", src, got, bunChecksums[target])
	}

	exe, err := extractBun(archive.Name(), dir, filepath.Base(dest))
	if err != nil {
		return err
	}
	defer os.Remove(exe)
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %w", errBunUnusable, err)
	}
	if v := strings.TrimSpace(string(out)); v != BunVersion {
		return fmt.Errorf("%w: it reports version %q, want %q", errBunUnusable, v, BunVersion)
	}
	if err := os.Rename(exe, dest); err != nil {
		if _, serr := os.Stat(dest); serr == nil {
			return nil
		}
		return err
	}
	return nil
}

func removeStaleTemps(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".bun-") {
			continue
		}
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > staleTempAge {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func fetch(ctx context.Context, client *http.Client, src string, w io.Writer) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	stall := time.AfterFunc(bunStallTimeout, func() { cancel(errDownloadStalled) })
	defer stall.Stop()
	fail := func(err error) error {
		if cause := context.Cause(ctx); errors.Is(cause, errDownloadStalled) {
			err = cause
		}
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("download %s: %w", src, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fail(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", src, resp.Status)
	}
	body := &progressReader{r: io.LimitReader(resp.Body, maxBunArchiveSize+1), onRead: func() { stall.Reset(bunStallTimeout) }}
	n, err := io.Copy(w, body)
	if err != nil {
		return fail(err)
	}
	if n > maxBunArchiveSize {
		return fmt.Errorf("download %s: larger than %d bytes", src, maxBunArchiveSize)
	}
	return nil
}

type progressReader struct {
	r      io.Reader
	onRead func()
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.onRead()
	}
	return n, err
}

func extractBun(archive, dir, name string) (string, error) {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", filepath.Base(archive), err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || filepath.Base(f.Name) != name {
			continue
		}
		src, err := f.Open()
		if err != nil {
			return "", err
		}
		defer src.Close()
		out, err := os.CreateTemp(dir, ".bun-*"+filepath.Ext(name))
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, src); err != nil {
			out.Close()
			os.Remove(out.Name())
			return "", err
		}
		if err := out.Close(); err != nil {
			os.Remove(out.Name())
			return "", err
		}
		if err := os.Chmod(out.Name(), 0o755); err != nil {
			os.Remove(out.Name())
			return "", err
		}
		return out.Name(), nil
	}
	return "", fmt.Errorf("%s not found in %s", name, filepath.Base(archive))
}
