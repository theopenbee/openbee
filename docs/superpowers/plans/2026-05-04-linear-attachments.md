# Linear Attachments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Linear platform adapter download `uploads.linear.app` assets into `media.Service` placeholders on the inbound side, and upload `OutboundMessage.MediaPath` files via Linear's `fileUpload` mutation on the outbound side, mirroring the feishu/telegram media pipeline.

**Architecture:** A new `internal/platform/linear/attachments.go` houses three units — a markdown extractor (`extractAssetURLs`), a `resolver` that downloads and rewrites text into `<media:*>` placeholders, and an `uploader` that fileUpload+S3-PUT a local path and returns the markdown to append. The `Client` interface gains `DownloadAsset` and `FileUpload`. `handler.go` wires the resolver into `tickOnce` and the uploader into `Sender.Send`.

**Tech Stack:** Go 1.x, Linear GraphQL API, existing `internal/infra/media` service, `golang.org/x/sync/errgroup`, standard `net/http` (httptest in tests).

**Spec:** `docs/superpowers/specs/2026-05-04-linear-attachments-design.md`

---

## File Map

| File | Status | Responsibility |
|------|--------|---------------|
| `internal/infra/config/config.go` | modify | Add `LinearConfig.MaxMediaSize` and default |
| `internal/infra/config/config_bee_test.go` | modify | Cover new default + yaml override |
| `internal/platform/linear/client.go` | modify | Extend `Client` interface with `DownloadAsset` and `FileUpload`; add `FileUploadTicket` type and httpClient impls |
| `internal/platform/linear/client_test.go` | modify | Cover `DownloadAsset` and `FileUpload` |
| `internal/platform/linear/attachments.go` | create | `extractAssetURLs`, `resolver`, `uploader` |
| `internal/platform/linear/attachments_test.go` | create | Unit tests for the three units |
| `internal/platform/linear/handler.go` | modify | Wire resolver/uploader; `NewPlatform` takes `*media.Service`; rewrite text in `tickOnce`; integrate uploader in `Send` |
| `internal/platform/linear/handler_test.go` | modify | Update `fakeClient` to satisfy new interface; add resolver/uploader integration tests |
| `internal/platform/linear/sender_test.go` | modify | Replace `TestSender_RejectsMediaPath` with success path through uploader |
| `internal/app/app.go` | modify | Pass `mediaSvc` into `linear.NewPlatform` |
| `CHANGELOG.md` | modify | Note attachment support under `[Unreleased] / Added` |

---

## Task 1: Add `MaxMediaSize` to `LinearConfig`

**Files:**
- Modify: `internal/infra/config/config.go:230-237` (LinearConfig struct), `config.go` `applyDefaults`
- Test: `internal/infra/config/config_bee_test.go`

- [ ] **Step 1: Write the failing test (default)**

Add this test to `internal/infra/config/config_bee_test.go`:

```go
func TestBeeConfig_LinearDefaults(t *testing.T) {
	f, _ := os.CreateTemp("", "*.yaml")
	f.WriteString(`
server:
  port: 8080
`)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Bee.Platforms.Linear.MaxMediaSize != 50*1024*1024 {
		t.Errorf("Linear.MaxMediaSize default: want %d got %d",
			50*1024*1024, cfg.Bee.Platforms.Linear.MaxMediaSize)
	}
}
```

- [ ] **Step 2: Write the failing test (yaml override)**

Add immediately after the previous test:

```go
func TestBeeConfig_LinearLoad(t *testing.T) {
	f, _ := os.CreateTemp("", "*.yaml")
	f.WriteString(`
server:
  port: 8080
bee:
  platforms:
    linear:
      enabled: true
      api_key: "lin_test"
      max_media_size: 10485760
`)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	lc := cfg.Bee.Platforms.Linear
	if !lc.Enabled {
		t.Error("Linear.Enabled: want true")
	}
	if lc.APIKey != "lin_test" {
		t.Errorf("Linear.APIKey: want lin_test got %q", lc.APIKey)
	}
	if lc.MaxMediaSize != 10485760 {
		t.Errorf("Linear.MaxMediaSize: want 10485760 got %d", lc.MaxMediaSize)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/infra/config/ -run TestBeeConfig_Linear -v`
Expected: FAIL — both tests reference `Linear.MaxMediaSize` which does not exist yet.

- [ ] **Step 4: Add the field**

Edit `internal/infra/config/config.go` — locate `LinearConfig` (currently lines 230-237) and add the new field as the **last** field:

```go
type LinearConfig struct {
	Enabled      bool          `yaml:"enabled"`
	APIKey       string        `yaml:"api_key"`       // Linear personal API key (required when enabled)
	LabelName    string        `yaml:"label_name"`    // gating label; default "openbee"
	PollInterval time.Duration `yaml:"poll_interval"` // default 10s
	Projects     []string      `yaml:"projects"`      // project name allow-list; empty = process nothing
	States       []string      `yaml:"states"`        // workflow-state name allow-list; empty = skip
	MaxMediaSize int           `yaml:"max_media_size"` // bytes; default 50 MB
}
```

- [ ] **Step 5: Add the default**

Open `internal/infra/config/config.go` and locate `applyDefaults`. Find the existing block around `cfg.Bee.Platforms.Telegram.MaxMediaSize` and append a Linear stanza in the same style. Search for the line:

```
	if cfg.Bee.Platforms.Weixin.MaxMediaSize == 0 {
		cfg.Bee.Platforms.Weixin.MaxMediaSize = 100 * 1024 * 1024 // 100MB
	}
```

Insert immediately after that block:

```go
	if cfg.Bee.Platforms.Linear.MaxMediaSize == 0 {
		cfg.Bee.Platforms.Linear.MaxMediaSize = 50 * 1024 * 1024 // 50MB
	}
```

- [ ] **Step 6: Run tests to verify pass**

Run: `go test ./internal/infra/config/ -run TestBeeConfig_Linear -v`
Expected: PASS for both tests.

Also run: `go test ./internal/infra/config/ -v`
Expected: All existing config tests still PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/infra/config/config.go internal/infra/config/config_bee_test.go
git commit -m "feat(linear): add max_media_size config with 50MB default"
```

---

## Task 2: Add `FileUploadTicket` type and `Client.DownloadAsset`

**Files:**
- Modify: `internal/platform/linear/client.go`
- Test: `internal/platform/linear/client_test.go`

- [ ] **Step 1: Write the failing test**

Add to the end of `internal/platform/linear/client_test.go`:

```go
func TestClient_DownloadAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "test-key" {
			t.Errorf("Authorization = %q, want test-key", got)
		}
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer srv.Close()

	c := newHTTPClient("test-key")
	data, ct, err := c.DownloadAsset(context.Background(), srv.URL+"/some/path")
	if err != nil {
		t.Fatalf("DownloadAsset: %v", err)
	}
	if string(data) != "PNGDATA" {
		t.Errorf("data = %q, want PNGDATA", string(data))
	}
	if ct != "image/png" {
		t.Errorf("contentType = %q, want image/png", ct)
	}
}

func TestClient_DownloadAsset_NonOKReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := newHTTPClient("test-key")
	_, _, err := c.DownloadAsset(context.Background(), srv.URL+"/x")
	if err == nil {
		t.Fatal("expected error on non-2xx, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/platform/linear/ -run TestClient_DownloadAsset -v`
Expected: FAIL — `DownloadAsset` is undefined.

- [ ] **Step 3: Add the interface method and implementation**

In `internal/platform/linear/client.go`, locate the `Client` interface (around line 51) and add the method **before** the closing `}`:

```go
	// DownloadAsset fetches a uploads.linear.app asset using the workspace API
	// key in the Authorization header. Returns the body and the server-reported
	// Content-Type. A non-2xx response is returned as an error.
	DownloadAsset(ctx context.Context, url string) (data []byte, contentType string, err error)
```

Then append a new method on `*httpClient` after `CreateComment`:

```go
const downloadTimeout = 30 * time.Second

func (c *httpClient) DownloadAsset(ctx context.Context, url string) ([]byte, string, error) {
	dlCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("linear: build download request: %w", err)
	}
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("linear: download asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, "", fmt.Errorf("linear: download asset http %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("linear: read asset body: %w", err)
	}
	return data, resp.Header.Get("Content-Type"), nil
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/platform/linear/ -run TestClient_DownloadAsset -v`
Expected: PASS for both tests.

Run also: `go test ./internal/platform/linear/ -v`
Expected: existing tests fail because `fakeClient` (in `handler_test.go`) does not satisfy the extended interface. **This is expected — Task 4 fixes it.** Continue to Step 5 anyway; we will commit just the production code now and fix the test fake in a later task.

Actually — to keep the build green commit-by-commit, add the stub on `fakeClient` here so the package keeps compiling:

In `internal/platform/linear/handler_test.go`, add the following methods on `*fakeClient` (place them next to the other methods on `fakeClient`):

```go
func (f *fakeClient) DownloadAsset(ctx context.Context, url string) ([]byte, string, error) {
	return nil, "", nil
}
```

Re-run `go test ./internal/platform/linear/ -v` — all tests should PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/client.go internal/platform/linear/client_test.go internal/platform/linear/handler_test.go
git commit -m "feat(linear): add Client.DownloadAsset for authenticated asset fetch"
```

---

## Task 3: Add `Client.FileUpload` and `FileUploadTicket`

**Files:**
- Modify: `internal/platform/linear/client.go`
- Test: `internal/platform/linear/client_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/platform/linear/client_test.go`:

```go
func TestClient_FileUpload(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		if !strings.Contains(s, "fileUpload") {
			t.Errorf("query missing fileUpload: %s", s)
		}
		if !strings.Contains(s, `"filename":"foo.png"`) {
			t.Errorf("variables missing filename: %s", s)
		}
		if !strings.Contains(s, `"contentType":"image/png"`) {
			t.Errorf("variables missing contentType: %s", s)
		}
		if !strings.Contains(s, `"size":42`) {
			t.Errorf("variables missing size: %s", s)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"fileUpload": map[string]any{
					"success":   true,
					"uploadFile": map[string]any{
						"assetUrl":  "https://uploads.linear.app/abc.png",
						"uploadUrl": "https://s3.example/abc?sig=xyz",
						"headers": []map[string]string{
							{"key": "x-amz-acl", "value": "private"},
							{"key": "Content-Type", "value": "image/png"},
						},
					},
				},
			},
		})
	})

	got, err := c.FileUpload(context.Background(), "foo.png", "image/png", 42)
	if err != nil {
		t.Fatalf("FileUpload: %v", err)
	}
	if got.AssetURL != "https://uploads.linear.app/abc.png" {
		t.Errorf("AssetURL = %q", got.AssetURL)
	}
	if got.UploadURL != "https://s3.example/abc?sig=xyz" {
		t.Errorf("UploadURL = %q", got.UploadURL)
	}
	if got.Headers["x-amz-acl"] != "private" || got.Headers["Content-Type"] != "image/png" {
		t.Errorf("Headers = %v", got.Headers)
	}
}

func TestClient_FileUpload_GraphQLError(t *testing.T) {
	_, c := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]string{{"message": "denied"}},
		})
	})
	_, err := c.FileUpload(context.Background(), "foo.png", "image/png", 1)
	if err == nil {
		t.Fatal("expected error on graphql error response")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/platform/linear/ -run TestClient_FileUpload -v`
Expected: FAIL — `FileUpload` and `FileUploadTicket` are undefined.

- [ ] **Step 3: Add the type, interface method, and implementation**

In `internal/platform/linear/client.go`, after the `Comment` type (around line 34), add:

```go
// FileUploadTicket is the result of Linear's fileUpload mutation. AssetURL is
// embedded into a comment markdown after the bytes are PUT to UploadURL with
// the supplied Headers.
type FileUploadTicket struct {
	AssetURL  string
	UploadURL string
	Headers   map[string]string
}
```

Add to the `Client` interface, immediately after the `DownloadAsset` line:

```go
	// FileUpload runs Linear's fileUpload mutation and returns the presigned
	// upload target plus the asset URL to embed in a comment markdown.
	FileUpload(ctx context.Context, name, mime string, size int) (FileUploadTicket, error)
```

Then add the GraphQL constant and the method on `*httpClient`. Place these next to `createCommentMutation`:

```go
const fileUploadMutation = `
mutation FileUpload($filename: String!, $contentType: String!, $size: Int!) {
  fileUpload(filename: $filename, contentType: $contentType, size: $size) {
    success
    uploadFile {
      assetUrl
      uploadUrl
      headers { key value }
    }
  }
}`

func (c *httpClient) FileUpload(ctx context.Context, name, mime string, size int) (FileUploadTicket, error) {
	vars := map[string]any{
		"filename":    name,
		"contentType": mime,
		"size":        size,
	}
	var data struct {
		FileUpload struct {
			Success    bool `json:"success"`
			UploadFile struct {
				AssetURL  string `json:"assetUrl"`
				UploadURL string `json:"uploadUrl"`
				Headers   []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"uploadFile"`
		} `json:"fileUpload"`
	}
	if err := c.do(ctx, "fileUpload", fileUploadMutation, vars, &data); err != nil {
		return FileUploadTicket{}, err
	}
	if !data.FileUpload.Success {
		return FileUploadTicket{}, fmt.Errorf("linear: fileUpload not successful")
	}
	headers := make(map[string]string, len(data.FileUpload.UploadFile.Headers))
	for _, h := range data.FileUpload.UploadFile.Headers {
		headers[h.Key] = h.Value
	}
	return FileUploadTicket{
		AssetURL:  data.FileUpload.UploadFile.AssetURL,
		UploadURL: data.FileUpload.UploadFile.UploadURL,
		Headers:   headers,
	}, nil
}
```

Add the matching stub on `fakeClient` (in `internal/platform/linear/handler_test.go`):

```go
func (f *fakeClient) FileUpload(ctx context.Context, name, mime string, size int) (FileUploadTicket, error) {
	return FileUploadTicket{}, nil
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/platform/linear/ -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/client.go internal/platform/linear/client_test.go internal/platform/linear/handler_test.go
git commit -m "feat(linear): add Client.FileUpload mutation and ticket type"
```

---

## Task 4: Implement `extractAssetURLs`

**Files:**
- Create: `internal/platform/linear/attachments.go`
- Test: `internal/platform/linear/attachments_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/platform/linear/attachments_test.go`:

```go
package linear

import (
	"reflect"
	"testing"
)

func TestExtractAssetURLs_Image(t *testing.T) {
	in := "see ![diagram](https://uploads.linear.app/a/b/c.png) attached"
	got := extractAssetURLs(in)
	want := []assetMatch{{
		span:      [2]int{4, 60},
		url:       "https://uploads.linear.app/a/b/c.png",
		altOrName: "diagram",
		isImage:   true,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestExtractAssetURLs_Link(t *testing.T) {
	in := "doc: [spec.pdf](https://uploads.linear.app/a/b/c.pdf)"
	got := extractAssetURLs(in)
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1", len(got))
	}
	if got[0].isImage {
		t.Error("link form should have isImage=false")
	}
	if got[0].url != "https://uploads.linear.app/a/b/c.pdf" {
		t.Errorf("url = %q", got[0].url)
	}
	if got[0].altOrName != "spec.pdf" {
		t.Errorf("altOrName = %q", got[0].altOrName)
	}
}

func TestExtractAssetURLs_MultipleMixed(t *testing.T) {
	in := "a ![one](https://uploads.linear.app/x.png) b [two](https://uploads.linear.app/y.pdf) c"
	got := extractAssetURLs(in)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if !got[0].isImage || got[1].isImage {
		t.Errorf("expected first image, second link; got %+v", got)
	}
}

func TestExtractAssetURLs_SkipsForeignHost(t *testing.T) {
	in := "![x](https://example.com/foo.png)"
	if got := extractAssetURLs(in); len(got) != 0 {
		t.Errorf("expected 0 matches for foreign host, got %d", len(got))
	}
}

func TestExtractAssetURLs_SkipsFencedCode(t *testing.T) {
	in := "before\n```\n![inside](https://uploads.linear.app/x.png)\n```\nafter"
	if got := extractAssetURLs(in); len(got) != 0 {
		t.Errorf("expected 0 matches inside fenced block, got %+v", got)
	}
}

func TestExtractAssetURLs_SkipsInlineCode(t *testing.T) {
	in := "type `![x](https://uploads.linear.app/x.png)` to test"
	if got := extractAssetURLs(in); len(got) != 0 {
		t.Errorf("expected 0 matches inside inline code, got %+v", got)
	}
}

func TestExtractAssetURLs_AltWithUnicodeAndSpaces(t *testing.T) {
	in := "![中文 alt](https://uploads.linear.app/u.png)"
	got := extractAssetURLs(in)
	if len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
	if got[0].altOrName != "中文 alt" {
		t.Errorf("altOrName = %q", got[0].altOrName)
	}
}

func TestExtractAssetURLs_AltCanBeEmpty(t *testing.T) {
	in := "![](https://uploads.linear.app/u.png)"
	got := extractAssetURLs(in)
	if len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
	if got[0].altOrName != "" {
		t.Errorf("altOrName = %q, want empty", got[0].altOrName)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/platform/linear/ -run TestExtractAssetURLs -v`
Expected: FAIL — `extractAssetURLs` and `assetMatch` are undefined.

- [ ] **Step 3: Implement extractor**

Create `internal/platform/linear/attachments.go`:

```go
package linear

import (
	"regexp"
	"strings"
)

// assetMatch is one extracted markdown image or link pointing at uploads.linear.app.
type assetMatch struct {
	span      [2]int // byte offsets [start, end) in the original text
	url       string
	altOrName string
	isImage   bool
}

// linearAssetHost gates the extractor to only Linear-hosted uploads.
const linearAssetHost = "https://uploads.linear.app/"

// markdown image: ![alt](url)   ; alt may be empty, may contain unicode and spaces; URL ends at ) or whitespace.
var imageRE = regexp.MustCompile(`!\[([^\]]*)\]\((https://uploads\.linear\.app/[^)\s]+)\)`)

// markdown link: [text](url)
var linkRE = regexp.MustCompile(`\[([^\]]*)\]\((https://uploads\.linear\.app/[^)\s]+)\)`)

// extractAssetURLs returns the asset matches in text in their natural order.
// URLs inside fenced code blocks (```...```) and inline code (`...`) are
// ignored to avoid rewriting tutorial / example snippets.
func extractAssetURLs(text string) []assetMatch {
	masked := maskCodeRegions(text)
	var out []assetMatch

	for _, m := range imageRE.FindAllStringSubmatchIndex(masked, -1) {
		out = append(out, assetMatch{
			span:      [2]int{m[0], m[1]},
			altOrName: text[m[2]:m[3]],
			url:       text[m[4]:m[5]],
			isImage:   true,
		})
	}

	// linkRE also matches the leading [..](..) of an image (since the regex
	// has no leading-! constraint). Filter out spans already claimed by an
	// image match.
	for _, m := range linkRE.FindAllStringSubmatchIndex(masked, -1) {
		if isImageSpan(out, m[0], m[1]) {
			continue
		}
		out = append(out, assetMatch{
			span:      [2]int{m[0], m[1]},
			altOrName: text[m[2]:m[3]],
			url:       text[m[4]:m[5]],
			isImage:   false,
		})
	}

	// Re-sort by span start so caller can splice deterministically.
	sortMatchesBySpan(out)
	return out
}

func isImageSpan(images []assetMatch, start, end int) bool {
	for _, im := range images {
		if im.isImage && im.span[0] == start-1 && im.span[1] == end {
			return true // [..](..) sits exactly inside ![..](..)
		}
	}
	return false
}

func sortMatchesBySpan(in []assetMatch) {
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && in[j-1].span[0] > in[j].span[0]; j-- {
			in[j-1], in[j] = in[j], in[j-1]
		}
	}
}

// maskCodeRegions replaces all bytes inside ```fenced``` blocks and inline
// `...` spans with a non-matching filler ('.') so the extractor skips them.
// Length is preserved, so byte offsets in the masked string still line up
// with the original text.
func maskCodeRegions(text string) string {
	b := []byte(text)
	out := make([]byte, len(b))
	copy(out, b)

	// Fenced first.
	fence := []byte("```")
	for i := 0; i+3 <= len(b); {
		if !bytesEq(b[i:i+3], fence) {
			i++
			continue
		}
		// find closing fence after current
		j := i + 3
		for j+3 <= len(b) && !bytesEq(b[j:j+3], fence) {
			j++
		}
		end := j + 3
		if end > len(b) {
			end = len(b)
		}
		for k := i; k < end; k++ {
			out[k] = '.'
		}
		i = end
	}

	// Inline `...` (single backtick); ignore positions already masked.
	for i := 0; i < len(out); {
		if out[i] != '`' {
			i++
			continue
		}
		j := i + 1
		for j < len(out) && out[j] != '`' {
			j++
		}
		end := j + 1
		if end > len(out) {
			end = len(out)
		}
		for k := i; k < end; k++ {
			out[k] = '.'
		}
		i = end
	}
	return string(out)
}

func bytesEq(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// host check is implicit in the regex; this helper exists for tests.
func isLinearAssetURL(u string) bool { return strings.HasPrefix(u, linearAssetHost) }
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/platform/linear/ -run TestExtractAssetURLs -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/attachments.go internal/platform/linear/attachments_test.go
git commit -m "feat(linear): extract uploads.linear.app markdown image/link spans"
```

---

## Task 5: Implement `resolver.Resolve`

**Files:**
- Modify: `internal/platform/linear/attachments.go`
- Test: `internal/platform/linear/attachments_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/platform/linear/attachments_test.go`:

```go
import (
	// add to existing imports
	"context"
	"errors"
	"fmt"

	"github.com/theopenbee/openbee/internal/infra/media"
)

// fakeAssetClient is a Client double for resolver tests; it only implements
// the methods the resolver touches.
type fakeAssetClient struct {
	*fakeClient
	download func(url string) ([]byte, string, error)
}

func (f *fakeAssetClient) DownloadAsset(ctx context.Context, url string) ([]byte, string, error) {
	return f.download(url)
}

func newFakeResolverClient(dl func(url string) ([]byte, string, error)) *fakeAssetClient {
	return &fakeAssetClient{fakeClient: &fakeClient{}, download: dl}
}

func TestResolver_Resolve_NoMatchesReturnsOriginal(t *testing.T) {
	r := &resolver{
		client:  newFakeResolverClient(nil),
		media:   media.NewService(),
		maxSize: 10 * 1024 * 1024,
	}
	in := "plain text without any media"
	if got := r.Resolve(context.Background(), in); got != in {
		t.Errorf("got %q, want %q", got, in)
	}
}

func TestResolver_Resolve_ImageSuccessReplacesWithPlaceholder(t *testing.T) {
	r := &resolver{
		client: newFakeResolverClient(func(url string) ([]byte, string, error) {
			return []byte("PNGDATA"), "image/png", nil
		}),
		media:   media.NewService(),
		maxSize: 10 * 1024 * 1024,
	}
	in := "see ![diagram](https://uploads.linear.app/a/b.png)!"
	out := r.Resolve(context.Background(), in)
	if !strings.HasPrefix(out, "see <media:image ") {
		t.Errorf("expected placeholder prefix, got %q", out)
	}
	if !strings.HasSuffix(out, "!") {
		t.Errorf("expected suffix preserved, got %q", out)
	}
	if !strings.Contains(out, `name="diagram"`) {
		t.Errorf("placeholder missing name: %q", out)
	}
	if !strings.Contains(out, `path="`) {
		t.Errorf("placeholder missing path: %q", out)
	}
}

func TestResolver_Resolve_HTTPFailureFallsBackToOriginalURL(t *testing.T) {
	r := &resolver{
		client: newFakeResolverClient(func(url string) ([]byte, string, error) {
			return nil, "", errors.New("403 forbidden")
		}),
		media:   media.NewService(),
		maxSize: 10 * 1024 * 1024,
	}
	in := "look ![pic](https://uploads.linear.app/x.png)"
	out := r.Resolve(context.Background(), in)
	if !strings.Contains(out, "<media:image") {
		t.Errorf("expected fallback placeholder, got %q", out)
	}
	if strings.Contains(out, `path="`) {
		t.Errorf("fallback should not have path: %q", out)
	}
	if !strings.Contains(out, "Original: https://uploads.linear.app/x.png") {
		t.Errorf("fallback should retain original URL: %q", out)
	}
}

func TestResolver_Resolve_LinkFallbackUsesDocumentType(t *testing.T) {
	r := &resolver{
		client: newFakeResolverClient(func(url string) ([]byte, string, error) {
			return nil, "", errors.New("timeout")
		}),
		media:   media.NewService(),
		maxSize: 10 * 1024 * 1024,
	}
	in := "ref [spec.pdf](https://uploads.linear.app/x.pdf)"
	out := r.Resolve(context.Background(), in)
	if !strings.Contains(out, "<media:document") {
		t.Errorf("expected document fallback, got %q", out)
	}
}

func TestResolver_Resolve_SizeLimitExceededFallsBack(t *testing.T) {
	r := &resolver{
		client: newFakeResolverClient(func(url string) ([]byte, string, error) {
			return make([]byte, 11), "image/png", nil
		}),
		media:   media.NewService(),
		maxSize: 10,
	}
	in := "x ![big](https://uploads.linear.app/big.png)"
	out := r.Resolve(context.Background(), in)
	if strings.Contains(out, `path="`) {
		t.Errorf("oversize should not save, got %q", out)
	}
	if !strings.Contains(out, "Original: https://uploads.linear.app/big.png") {
		t.Errorf("oversize fallback missing Original: %q", out)
	}
}

func TestResolver_Resolve_MultipleMatchesPartialFailure(t *testing.T) {
	r := &resolver{
		client: newFakeResolverClient(func(url string) ([]byte, string, error) {
			if strings.Contains(url, "good") {
				return []byte("PNG"), "image/png", nil
			}
			return nil, "", fmt.Errorf("nope")
		}),
		media:   media.NewService(),
		maxSize: 10 * 1024 * 1024,
	}
	in := "a ![ok](https://uploads.linear.app/good.png) b ![bad](https://uploads.linear.app/bad.png) c"
	out := r.Resolve(context.Background(), in)
	if !strings.Contains(out, `path="`) {
		t.Errorf("expected one success placeholder with path: %q", out)
	}
	if !strings.Contains(out, "Original: https://uploads.linear.app/bad.png") {
		t.Errorf("expected fallback for bad URL: %q", out)
	}
	if !strings.HasPrefix(out, "a ") || !strings.HasSuffix(out, " c") {
		t.Errorf("surrounding text not preserved: %q", out)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/platform/linear/ -run TestResolver -v`
Expected: FAIL — `resolver` is undefined.

- [ ] **Step 3: Add resolver to `attachments.go`**

Replace the existing import block at the top of `internal/platform/linear/attachments.go` (currently only `"regexp"` and `"strings"`) with:

```go
import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/theopenbee/openbee/internal/infra/media"
)

const resolverConcurrency = 4

// resolver downloads uploads.linear.app assets cited in markdown text and
// rewrites the segments into <media:*> placeholders. Per-asset failures fall
// back to a placeholder that retains the original URL on a separate line.
type resolver struct {
	client  Client
	media   *media.Service
	maxSize int
}

// Resolve never returns an error; per-match failure falls back inline.
func (r *resolver) Resolve(ctx context.Context, text string) string {
	matches := extractAssetURLs(text)
	if len(matches) == 0 {
		return text
	}

	replacements := make([]string, len(matches))

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(resolverConcurrency)
	var mu sync.Mutex
	for i, m := range matches {
		g.Go(func() error {
			rep := r.resolveOne(gCtx, m)
			mu.Lock()
			replacements[i] = rep
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()

	return spliceReplacements(text, matches, replacements)
}

// resolveOne returns the replacement string for a single match. It never
// returns an error; failures collapse into the fallback placeholder.
func (r *resolver) resolveOne(ctx context.Context, m assetMatch) string {
	data, contentType, err := r.client.DownloadAsset(ctx, m.url)
	if err != nil {
		log.Warn("linear: download asset failed",
			zap.String("url", m.url), zap.Error(err))
		return r.fallback(m)
	}
	if len(data) > r.maxSize {
		log.Warn("linear: asset exceeds max size",
			zap.String("url", m.url),
			zap.Int("size", len(data)),
			zap.Int("max", r.maxSize),
		)
		return r.fallback(m)
	}

	mime := r.media.DetectMIME(data, m.altOrName)
	if contentType != "" {
		mime = contentType
	}
	ext := r.media.ExtensionFromMIME(mime)
	path, err := r.media.SaveInbound(ctx, data, ext)
	if err != nil {
		log.Warn("linear: save asset failed",
			zap.String("url", m.url), zap.Error(err))
		return r.fallback(m)
	}
	return r.media.BuildPlaceholder(media.MediaTypeFromMIME(mime), path, m.altOrName)
}

func (r *resolver) fallback(m assetMatch) string {
	mediaType := "document"
	if m.isImage {
		mediaType = "image"
	}
	placeholder := r.media.BuildPlaceholder(mediaType, "", m.altOrName)
	return placeholder + "\nOriginal: " + m.url
}

// spliceReplacements stitches text + replacements together. matches is sorted
// by span start (extractAssetURLs guarantees this).
func spliceReplacements(text string, matches []assetMatch, reps []string) string {
	if len(matches) == 0 {
		return text
	}
	var b []byte
	cursor := 0
	for i, m := range matches {
		b = append(b, text[cursor:m.span[0]]...)
		b = append(b, reps[i]...)
		cursor = m.span[1]
	}
	b = append(b, text[cursor:]...)
	return string(b)
}

```

Note: the `log` symbol is the package-level logger declared in `handler.go`. Don't redeclare it.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/platform/linear/ -run TestResolver -v`
Expected: all PASS.

Run also: `go test ./internal/platform/linear/ -v`
Expected: all existing tests still PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/attachments.go internal/platform/linear/attachments_test.go
git commit -m "feat(linear): add resolver to download and placeholder assets"
```

---

## Task 6: Implement `uploader.Upload`

**Files:**
- Modify: `internal/platform/linear/attachments.go`
- Test: `internal/platform/linear/attachments_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/platform/linear/attachments_test.go`:

```go
// fakeUploaderClient lets the test inject a FileUpload implementation.
type fakeUploaderClient struct {
	*fakeClient
	upload func(name, mime string, size int) (FileUploadTicket, error)
}

func (f *fakeUploaderClient) FileUpload(ctx context.Context, name, mime string, size int) (FileUploadTicket, error) {
	return f.upload(name, mime, size)
}

func TestUploader_UploadImage_ReturnsImageMarkdown(t *testing.T) {
	// Capture S3 PUT
	var (
		gotMethod  string
		gotHeaders map[string]string
		gotBody    []byte
	)
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotHeaders = map[string]string{}
		for k := range r.Header {
			gotHeaders[k] = r.Header.Get(k)
		}
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer s3.Close()

	tmp := t.TempDir()
	imgPath := tmp + "/foo.png"
	if err := os.WriteFile(imgPath, []byte("PNGBYTES"), 0o644); err != nil {
		t.Fatal(err)
	}

	u := &uploader{
		client: &fakeUploaderClient{fakeClient: &fakeClient{}, upload: func(name, mime string, size int) (FileUploadTicket, error) {
			if name != "foo.png" || mime == "" || size != len("PNGBYTES") {
				t.Errorf("FileUpload args: name=%q mime=%q size=%d", name, mime, size)
			}
			return FileUploadTicket{
				AssetURL:  "https://uploads.linear.app/asset.png",
				UploadURL: s3.URL + "/sig",
				Headers:   map[string]string{"X-Test": "yes"},
			}, nil
		}},
		maxSize: 10 * 1024 * 1024,
		http:    http.DefaultClient,
	}

	md, err := u.Upload(context.Background(), imgPath)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if md != "![foo.png](https://uploads.linear.app/asset.png)" {
		t.Errorf("md = %q", md)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("S3 method = %q, want PUT", gotMethod)
	}
	if string(gotBody) != "PNGBYTES" {
		t.Errorf("S3 body = %q", string(gotBody))
	}
	if gotHeaders["X-Test"] != "yes" {
		t.Errorf("S3 headers missing X-Test: %v", gotHeaders)
	}
}

func TestUploader_UploadDocument_ReturnsLinkMarkdown(t *testing.T) {
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer s3.Close()

	tmp := t.TempDir()
	pdf := tmp + "/spec.pdf"
	if err := os.WriteFile(pdf, []byte("%PDF-1.4 ..."), 0o644); err != nil {
		t.Fatal(err)
	}

	u := &uploader{
		client: &fakeUploaderClient{fakeClient: &fakeClient{}, upload: func(string, string, int) (FileUploadTicket, error) {
			return FileUploadTicket{
				AssetURL:  "https://uploads.linear.app/spec.pdf",
				UploadURL: s3.URL + "/sig",
			}, nil
		}},
		maxSize: 10 * 1024 * 1024,
		http:    http.DefaultClient,
	}

	md, err := u.Upload(context.Background(), pdf)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if md != "[spec.pdf](https://uploads.linear.app/spec.pdf)" {
		t.Errorf("md = %q", md)
	}
}

func TestUploader_FileMissing_ReturnsError(t *testing.T) {
	u := &uploader{
		client:  &fakeUploaderClient{fakeClient: &fakeClient{}, upload: nil},
		maxSize: 10 * 1024 * 1024,
		http:    http.DefaultClient,
	}
	_, err := u.Upload(context.Background(), "/no/such/file.png")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestUploader_FileTooLarge_RejectsBeforeMutation(t *testing.T) {
	tmp := t.TempDir()
	p := tmp + "/big.png"
	if err := os.WriteFile(p, make([]byte, 11), 0o644); err != nil {
		t.Fatal(err)
	}
	called := false
	u := &uploader{
		client: &fakeUploaderClient{fakeClient: &fakeClient{}, upload: func(string, string, int) (FileUploadTicket, error) {
			called = true
			return FileUploadTicket{}, nil
		}},
		maxSize: 10,
		http:    http.DefaultClient,
	}
	_, err := u.Upload(context.Background(), p)
	if err == nil {
		t.Fatal("expected error for oversized file")
	}
	if called {
		t.Error("FileUpload should not be invoked when over limit")
	}
}

func TestUploader_PUTNon2xx_ReturnsError(t *testing.T) {
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s3.Close()

	tmp := t.TempDir()
	p := tmp + "/foo.png"
	if err := os.WriteFile(p, []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}

	u := &uploader{
		client: &fakeUploaderClient{fakeClient: &fakeClient{}, upload: func(string, string, int) (FileUploadTicket, error) {
			return FileUploadTicket{
				AssetURL:  "https://uploads.linear.app/foo.png",
				UploadURL: s3.URL + "/sig",
			}, nil
		}},
		maxSize: 10 * 1024 * 1024,
		http:    http.DefaultClient,
	}
	_, err := u.Upload(context.Background(), p)
	if err == nil {
		t.Fatal("expected error on PUT 5xx")
	}
}
```

Make sure imports include:

```go
"io"
"net/http"
"net/http/httptest"
"os"
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/platform/linear/ -run TestUploader -v`
Expected: FAIL — `uploader` is undefined.

- [ ] **Step 3: Implement uploader**

First update the import block at the top of `internal/platform/linear/attachments.go` to include the new packages — replace whatever is there with:

```go
import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/theopenbee/openbee/internal/infra/media"
)

const uploadPutTimeout = 120 * time.Second

// uploader runs Linear's two-step upload (fileUpload mutation + presigned PUT)
// and returns markdown to embed in a comment body.
type uploader struct {
	client  Client
	maxSize int
	http    *http.Client
}

// Upload returns the markdown fragment for the given local file path.
// Image files render as "![name](assetUrl)"; everything else as "[name](assetUrl)".
func (u *uploader) Upload(ctx context.Context, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("linear: read upload file: %w", err)
	}
	if len(data) > u.maxSize {
		return "", fmt.Errorf("linear: upload file too large: %d bytes (max %d)", len(data), u.maxSize)
	}

	name := filepath.Base(path)
	mime := http.DetectContentType(data)

	ticket, err := u.client.FileUpload(ctx, name, mime, len(data))
	if err != nil {
		return "", fmt.Errorf("linear: fileUpload mutation: %w", err)
	}

	putCtx, cancel := context.WithTimeout(ctx, uploadPutTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(putCtx, http.MethodPut, ticket.UploadURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("linear: build upload PUT: %w", err)
	}
	for k, v := range ticket.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", mime)
	}

	resp, err := u.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("linear: upload PUT: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("linear: upload PUT http %d", resp.StatusCode)
	}

	if strings.HasPrefix(mime, "image/") {
		return fmt.Sprintf("![%s](%s)", name, ticket.AssetURL), nil
	}
	return fmt.Sprintf("[%s](%s)", name, ticket.AssetURL), nil
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/platform/linear/ -run TestUploader -v`
Expected: all PASS.

Run also: `go test ./internal/platform/linear/ -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/attachments.go internal/platform/linear/attachments_test.go
git commit -m "feat(linear): add uploader for fileUpload + S3 PUT flow"
```

---

## Task 7: Wire resolver into receiver

**Files:**
- Modify: `internal/platform/linear/handler.go`
- Modify: `internal/platform/linear/handler_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/platform/linear/handler_test.go` (next to other receiver tests):

```go
func TestReceiver_TickOnce_ResolvesAssetURLsInDescriptionAndComments(t *testing.T) {
	bot := User{ID: "BOT"}
	descURL := "https://uploads.linear.app/d/desc.png"
	commURL := "https://uploads.linear.app/d/comm.png"
	issue := Issue{
		ID: "I1", Identifier: "ENG-42",
		Title:       "Fix login",
		Description: "Snapshot ![desc](" + descURL + ")",
		Team:        Team{Key: "ENG"}, Creator: User{ID: "U2", Name: "Alice"},
		Comments: []Comment{
			{ID: "C1", Body: "Repro: ![c1](" + commURL + ")", User: User{ID: "U2", Name: "Alice"}},
		},
	}

	fc := &fakeClient{viewer: bot, issues: func() ([]Issue, error) { return []Issue{issue}, nil }}
	fc.downloads = map[string][]byte{
		descURL: []byte("PNG-D"),
		commURL: []byte("PNG-C"),
	}

	r := &LinearReceiver{
		client:       fc,
		seenIssues:   newFakeSeenSet(),
		seenComments: newFakeSeenSet(),
		labelName:    "openbee",
		pollInterval: time.Hour,
		projectStore: testProjectStore(),
		statesStore:  testStatesStore(),
		resolver: &resolver{
			client:  fc,
			media:   media.NewService(),
			maxSize: 10 * 1024 * 1024,
		},
	}

	var received []platform.InboundMessage
	r.tickOnce(context.Background(), func(m platform.InboundMessage) { received = append(received, m) })
	if len(received) != 1 {
		t.Fatalf("got %d", len(received))
	}
	body := received[0].Content
	if strings.Contains(body, descURL) {
		t.Errorf("description URL not replaced: %q", body)
	}
	if strings.Contains(body, commURL) {
		t.Errorf("comment URL not replaced: %q", body)
	}
	if !strings.Contains(body, "<media:image") {
		t.Errorf("expected placeholders in body: %q", body)
	}
}
```

Add to `fakeClient` in the same file (next to existing methods):

```go
// Override DownloadAsset for resolver tests using a per-URL map.
type downloadResult struct {
	data []byte
	ct   string
	err  error
}

func (f *fakeClient) downloads_setBy(url string, data []byte) {
	if f.downloads == nil {
		f.downloads = map[string][]byte{}
	}
	f.downloads[url] = data
}
```

Then update the existing `DownloadAsset` stub on `*fakeClient` to:

```go
func (f *fakeClient) DownloadAsset(ctx context.Context, url string) ([]byte, string, error) {
	if data, ok := f.downloads[url]; ok {
		return data, "image/png", nil
	}
	return nil, "", nil
}
```

And add the `downloads` field to `fakeClient`:

```go
type fakeClient struct {
	mu           sync.Mutex
	viewer       User
	calls        int
	lastStates   []string
	lastProjects []string
	issues       func() ([]Issue, error)
	created      []struct {
		IssueID, Body string
		ParentID      *string
	}
	downloads map[string][]byte // optional: used by resolver tests
}
```

Make sure `internal/platform/linear/handler_test.go` imports are updated to include `"strings"` and `"github.com/theopenbee/openbee/internal/infra/media"`.

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/platform/linear/ -run TestReceiver_TickOnce_Resolves -v`
Expected: FAIL — `LinearReceiver.resolver` field does not exist.

- [ ] **Step 3: Wire `resolver` into `LinearReceiver` and call it**

Existing receiver tests construct `LinearReceiver{...}` directly without a resolver. After this change `tickOnce` dereferences `r.resolver`, so they will nil-panic. Update each existing test (every `&LinearReceiver{...}` literal in `handler_test.go` apart from the one added in Step 1) to set:

```go
resolver: &resolver{
    client:  fc,
    media:   media.NewService(),
    maxSize: 10 * 1024 * 1024,
},
```

`extractAssetURLs` returns no matches for the existing test fixtures, so `Resolve` is a pass-through — assertions stay valid.

In `internal/platform/linear/handler.go`:

1. Add `resolver` field to `LinearReceiver`:

```go
type LinearReceiver struct {
	client       Client
	seenIssues   seenAPI
	seenComments seenAPI
	labelName    string
	pollInterval time.Duration
	projectStore *linearcfg.Store
	statesStore  *linearcfg.Store
	resolver     *resolver // new
}
```

2. In `tickOnce`, replace the body of the loop body so each issue and each comment runs its text through `r.resolver.Resolve`. Find the current `for _, issue := range issues {` block and rewrite the inner blocks. Replace **only the inner replacement logic** so it looks like:

```go
	for _, issue := range issues {
		if !r.seenIssues.Contains(issue.ID) {
			nonBot := nonBotComments(issue.Comments)
			resolvedIssue := issue
			resolvedIssue.Description = r.resolver.Resolve(ctx, issue.Description)
			resolvedComments := make([]Comment, len(nonBot))
			for i, c := range nonBot {
				rc := c
				rc.Body = r.resolver.Resolve(ctx, c.Body)
				resolvedComments[i] = rc
			}
			log.Debug("tick: dispatch initial merged",
				zap.String("identifier", issue.Identifier),
				zap.String("issue_id", issue.ID),
				zap.Int("non_bot_comment_count", len(nonBot)),
			)
			dispatch(buildInitialInbound(resolvedIssue, resolvedComments))
			newIssueIDs = append(newIssueIDs, issue.ID)
			for _, c := range nonBot {
				newCommentIDs = append(newCommentIDs, c.ID)
			}
			continue
		}
		for _, c := range issue.Comments {
			if r.seenComments.Contains(c.ID) {
				continue
			}
			if strings.HasPrefix(c.Body, botCommentPrefix) {
				continue
			}
			rc := c
			rc.Body = r.resolver.Resolve(ctx, c.Body)
			log.Debug("tick: dispatch comment",
				zap.String("identifier", issue.Identifier),
				zap.String("comment_id", c.ID),
				zap.String("user_id", c.User.ID),
			)
			dispatch(buildCommentInbound(issue, rc))
			newCommentIDs = append(newCommentIDs, c.ID)
		}
	}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/platform/linear/ -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/handler.go internal/platform/linear/handler_test.go
git commit -m "feat(linear): rewrite asset URLs into media placeholders on receive"
```

---

## Task 8: Wire uploader into sender

**Files:**
- Modify: `internal/platform/linear/handler.go`
- Modify: `internal/platform/linear/sender_test.go`

- [ ] **Step 1: Replace the rejection test with a success test**

Edit `internal/platform/linear/sender_test.go`. Delete `TestSender_RejectsMediaPath`. Replace it with:

```go
func TestSender_AppendsUploadedMarkdownToBody(t *testing.T) {
	rawBytes, _ := json.Marshal(replyTarget{IssueID: "I1"})

	tmp := t.TempDir()
	imgPath := tmp + "/snap.png"
	if err := os.WriteFile(imgPath, []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}

	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer s3.Close()

	fc := &fakeClient{viewer: User{ID: "BOT"}}
	fc.uploadImpl = func(name, mime string, size int) (FileUploadTicket, error) {
		return FileUploadTicket{
			AssetURL:  "https://uploads.linear.app/snap.png",
			UploadURL: s3.URL + "/sig",
		}, nil
	}

	s := &LinearSender{
		client:   fc,
		uploader: &uploader{client: fc, maxSize: 10 * 1024 * 1024, http: http.DefaultClient},
	}

	err := s.Send(context.Background(), platform.OutboundMessage{
		Content:   "see attached",
		MediaPath: imgPath,
		ReplyTo:   platform.InboundMessage{Raw: string(rawBytes)},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(fc.created) != 1 {
		t.Fatalf("expected 1 CreateComment, got %d", len(fc.created))
	}
	wantBody := "[openbee-bot]\n\nsee attached\n\n![snap.png](https://uploads.linear.app/snap.png)"
	if fc.created[0].Body != wantBody {
		t.Errorf("body = %q, want %q", fc.created[0].Body, wantBody)
	}
}
```

Add imports `"net/http"`, `"net/http/httptest"`, `"os"` to `sender_test.go` if not present.

Also add `uploadImpl` plumbing to `fakeClient` in `handler_test.go`. Replace the existing stub:

```go
func (f *fakeClient) FileUpload(ctx context.Context, name, mime string, size int) (FileUploadTicket, error) {
	if f.uploadImpl != nil {
		return f.uploadImpl(name, mime, size)
	}
	return FileUploadTicket{}, nil
}
```

And add to `fakeClient` struct:

```go
	uploadImpl func(name, mime string, size int) (FileUploadTicket, error)
```

- [ ] **Step 2: Add a regression test that text-only sends still work**

Confirm `TestSender_PostsCommentWithParentID` still exists and passes. No edits needed for it.

- [ ] **Step 3: Run tests to verify failure**

Run: `go test ./internal/platform/linear/ -run TestSender -v`
Expected: FAIL — `LinearSender.uploader` field does not exist; `Send` still rejects MediaPath.

- [ ] **Step 4: Add `uploader` field and integrate it**

In `internal/platform/linear/handler.go`:

1. Add `uploader` field to `LinearSender`:

```go
type LinearSender struct {
	client   Client
	uploader *uploader
}
```

2. Replace the body of `Send`:

```go
func (s *LinearSender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	var target replyTarget
	if err := json.Unmarshal([]byte(msg.ReplyTo.Raw), &target); err != nil {
		return fmt.Errorf("linear: parse reply target: %w", err)
	}
	if target.IssueID == "" {
		return errors.New("linear: reply target missing issue_id")
	}

	body := selfMarker + msg.Content
	if msg.MediaPath != "" {
		md, err := s.uploader.Upload(ctx, msg.MediaPath)
		if err != nil {
			return fmt.Errorf("linear: upload attachment: %w", err)
		}
		if msg.Content != "" {
			body = body + "\n\n" + md
		} else {
			body = selfMarker + md
		}
	}

	return utils.RetryWithBackoff(ctx, func() error {
		_, err := s.client.CreateComment(ctx, target.IssueID, body, target.ParentCommentID)
		return err
	}, utils.DefaultRetryCount, utils.DefaultRetryDelay)
}
```

Remove the now-unused `errors` import if it ends up unused; but `errors.New` is still called above, so keep it.

- [ ] **Step 5: Run tests to verify pass**

Run: `go test ./internal/platform/linear/ -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/platform/linear/handler.go internal/platform/linear/handler_test.go internal/platform/linear/sender_test.go
git commit -m "feat(linear): upload MediaPath via fileUpload and append to comment"
```

---

## Task 9: Update `NewPlatform` to accept `*media.Service`

**Files:**
- Modify: `internal/platform/linear/handler.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Modify `NewPlatform` signature**

Open `internal/platform/linear/handler.go`. Update `NewPlatform`:

```go
func NewPlatform(cfg config.LinearConfig, projectStore, statesStore *linearcfg.Store, mediaSvc *media.Service) (platform.Platform, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("linear: resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".openbee", ".linear")
	client := NewClient(cfg.APIKey)

	maxSize := cfg.MaxMediaSize
	if maxSize == 0 {
		maxSize = 50 * 1024 * 1024 // safety net if applyDefaults was bypassed
	}

	return &LinearPlatform{
		receiver: &LinearReceiver{
			client:       client,
			seenIssues:   NewSeenSet(dir, "seen_issues.ndjson"),
			seenComments: NewSeenSet(dir, "seen_comments.ndjson"),
			labelName:    cfg.LabelName,
			pollInterval: cfg.PollInterval,
			projectStore: projectStore,
			statesStore:  statesStore,
			resolver: &resolver{
				client:  client,
				media:   mediaSvc,
				maxSize: maxSize,
			},
		},
		sender: &LinearSender{
			client: client,
			uploader: &uploader{
				client:  client,
				maxSize: maxSize,
				http:    &http.Client{Timeout: uploadPutTimeout + 30*time.Second},
			},
		},
	}, nil
}
```

Add `"net/http"` and the `media` import:

```go
import (
	// add
	"net/http"

	"github.com/theopenbee/openbee/internal/infra/media"
)
```

- [ ] **Step 2: Update the call site in `app.go`**

Open `internal/app/app.go`. Locate the Linear branch (around line 354):

```go
		if lc.Enabled {
			p, err := linear.NewPlatform(lc, linearCfg, linearStates)
```

Replace with:

```go
		if lc.Enabled {
			p, err := linear.NewPlatform(lc, linearCfg, linearStates, mediaSvc)
```

- [ ] **Step 3: Run build & tests**

Run: `go build ./...`
Expected: success.

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/platform/linear/handler.go internal/app/app.go
git commit -m "feat(linear): construct platform with media.Service for attachments"
```

---

## Task 10: Changelog

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add entry**

In `CHANGELOG.md`, under `## [Unreleased]` → `### Added`, append below the existing Linear bullet:

```markdown
- Linear platform now downloads `uploads.linear.app` images and files cited in issue descriptions and comments into the standard `<media:*>` placeholders, and supports outbound attachments by uploading via Linear's `fileUpload` mutation and appending the resulting markdown to the comment body. New `bee.platforms.linear.max_media_size` config (default 50 MB).
```

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs(changelog): note Linear attachment support"
```

---

## Task 11: Final verification

- [ ] **Step 1: Full test suite**

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 2: Vet**

Run: `go vet ./...`
Expected: no warnings.

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Spot-check the spec**

Open `docs/superpowers/specs/2026-05-04-linear-attachments-design.md` and verify each section in §2-§5 has at least one task in this plan that implements it. Note any gaps as a follow-up commit if found.

- [ ] **Step 5: Push the branch (only if running interactively and the user confirms)**

```bash
git push
```

The branch is `feat/linear-platform`; do not push unless the user explicitly says so.
