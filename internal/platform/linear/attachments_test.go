package linear

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/media"
)

func TestExtractAssetURLs_Image(t *testing.T) {
	in := "see ![diagram](https://uploads.linear.app/a/b/c.png) attached"
	got := extractAssetURLs(in)
	want := []assetMatch{{
		span:      [2]int{4, 52},
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

// fakeAssetClient is a Client double for resolver tests; it overrides
// DownloadAsset while delegating all other interface methods to the
// embedded fakeClient (see handler_test.go).
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
