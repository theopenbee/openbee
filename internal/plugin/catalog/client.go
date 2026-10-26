package catalog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxDocumentBytes = 16 << 20
	maxTarballBytes  = 32 << 20
)

type Options struct {
	Registries []string
	HTTP       *http.Client
	CachePath  string
	TTL        time.Duration
	Override   string
	Now        func() time.Time
}

type Client struct {
	opts      Options
	mu        sync.Mutex
	cached    *Catalog
	fetchedAt time.Time
}

func NewClient(o Options) *Client {
	if len(o.Registries) == 0 {
		o.Registries = DefaultRegistries
	}
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if o.TTL == 0 {
		o.TTL = 10 * time.Minute
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Client{opts: o}
}

func (c *Client) Get(ctx context.Context, refresh bool) (Catalog, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.opts.Now()
	if !refresh && c.cached != nil && now.Sub(c.fetchedAt) < c.opts.TTL {
		return *c.cached, nil
	}
	raw, err := c.fetch(ctx)
	if err == nil {
		var cat Catalog
		if cat, err = parse(raw); err == nil {
			c.cached, c.fetchedAt = &cat, now
			c.writeCache(raw)
			return cat, nil
		}
	}
	if c.cached != nil {
		return *c.cached, nil
	}
	if data, rerr := os.ReadFile(c.opts.CachePath); rerr == nil {
		if cat, perr := parse(data); perr == nil {
			c.cached, c.fetchedAt = &cat, now
			return cat, nil
		}
	}
	return Catalog{}, err
}

func parse(data []byte) (Catalog, error) {
	var cat Catalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return Catalog{}, fmt.Errorf("parse catalog.json: %w", err)
	}
	if cat.SchemaVersion != SchemaVersion {
		return Catalog{}, fmt.Errorf("catalog schema_version %d is not supported (supported: %d)", cat.SchemaVersion, SchemaVersion)
	}
	return cat, nil
}

func (c *Client) writeCache(raw []byte) {
	if c.opts.CachePath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.opts.CachePath), 0o700); err != nil {
		return
	}
	tmp := c.opts.CachePath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, c.opts.CachePath)
}

func (c *Client) fetch(ctx context.Context) ([]byte, error) {
	if o := c.opts.Override; o != "" {
		if strings.HasPrefix(o, "http://") || strings.HasPrefix(o, "https://") {
			return c.get(ctx, o, maxDocumentBytes)
		}
		return os.ReadFile(o)
	}
	var errs []error
	for _, base := range c.opts.Registries {
		raw, err := c.fetchFrom(ctx, strings.TrimRight(base, "/"))
		if err == nil {
			return raw, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", base, err))
		if ctx.Err() != nil {
			break
		}
	}
	return nil, errors.Join(errs...)
}

type packument struct {
	Versions map[string]struct {
		Dist struct {
			Tarball   string `json:"tarball"`
			Integrity string `json:"integrity"`
		} `json:"dist"`
	} `json:"versions"`
}

func (c *Client) fetchFrom(ctx context.Context, base string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s?t=%d", base, strings.Replace(PackageName, "/", "%2F", 1), c.opts.Now().UnixNano())
	body, err := c.get(ctx, url, maxDocumentBytes)
	if err != nil {
		return nil, err
	}
	var doc packument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse package metadata: %w", err)
	}
	version, ok := latestRelease(doc, SchemaVersion)
	if !ok {
		return nil, fmt.Errorf("%s has no %d.x release", PackageName, SchemaVersion)
	}
	dist := doc.Versions[version].Dist
	tgz, err := c.get(ctx, dist.Tarball, maxTarballBytes)
	if err != nil {
		return nil, err
	}
	if !verifyIntegrity(tgz, dist.Integrity) {
		return nil, fmt.Errorf("tarball of %s@%s does not match its integrity", PackageName, version)
	}
	return readCatalog(tgz)
}

func latestRelease(doc packument, major int) (string, bool) {
	best, bestParts := "", [3]int{}
	for v := range doc.Versions {
		parts, ok := parseVersion(v)
		if !ok || parts[0] != major {
			continue
		}
		if best == "" || compareParts(parts, bestParts) > 0 {
			best, bestParts = v, parts
		}
	}
	return best, best != ""
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	fields := strings.Split(v, ".")
	if len(fields) != 3 {
		return out, false
	}
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 || (len(f) > 1 && f[0] == '0') {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func compareParts(a, b [3]int) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] > b[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

func CompareVersions(a, b string) (int, bool) {
	pa, ok := parseVersion(a)
	if !ok {
		return 0, false
	}
	pb, ok := parseVersion(b)
	if !ok {
		return 0, false
	}
	return compareParts(pa, pb), true
}

func verifyIntegrity(data []byte, integrity string) bool {
	want, ok := strings.CutPrefix(integrity, "sha512-")
	if !ok {
		return false
	}
	sum := sha512.Sum512(data)
	return base64.StdEncoding.EncodeToString(sum[:]) == want
}

func readCatalog(tgz []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, fmt.Errorf("read tarball: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("catalog.json is missing from the package")
		}
		if err != nil {
			return nil, fmt.Errorf("read tarball: %w", err)
		}
		_, rest, _ := strings.Cut(h.Name, "/")
		if rest != "catalog.json" || h.Typeflag != tar.TypeReg {
			continue
		}
		return io.ReadAll(io.LimitReader(tr, maxDocumentBytes))
	}
}

func (c *Client) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.opts.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%s is not published (HTTP 404 from %s)", PackageName, url)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: response larger than %d bytes", url, limit)
	}
	return data, nil
}
