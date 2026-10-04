package linear

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.Equal(t, want, got)
}

func TestExtractAssetURLs_Link(t *testing.T) {
	in := "doc: [spec.pdf](https://uploads.linear.app/a/b/c.pdf)"
	got := extractAssetURLs(in)
	require.Len(t, got, 1)
	assert.False(t, got[0].isImage, "link form should have isImage=false")
	assert.Equal(t, "https://uploads.linear.app/a/b/c.pdf", got[0].url)
	assert.Equal(t, "spec.pdf", got[0].altOrName)
}

func TestExtractAssetURLs_MultipleMixed(t *testing.T) {
	in := "a ![one](https://uploads.linear.app/x.png) b [two](https://uploads.linear.app/y.pdf) c"
	got := extractAssetURLs(in)
	require.Len(t, got, 2)
	assert.True(t, got[0].isImage, "expected first match to be an image")
	assert.False(t, got[1].isImage, "expected second match to be a link")
}

func TestExtractAssetURLs_SkipsForeignHost(t *testing.T) {
	in := "![x](https://example.com/foo.png)"
	got := extractAssetURLs(in)
	assert.Empty(t, got)
}

func TestExtractAssetURLs_SkipsFencedCode(t *testing.T) {
	in := "before\n```\n![inside](https://uploads.linear.app/x.png)\n```\nafter"
	got := extractAssetURLs(in)
	assert.Empty(t, got)
}

func TestExtractAssetURLs_SkipsInlineCode(t *testing.T) {
	in := "type `![x](https://uploads.linear.app/x.png)` to test"
	got := extractAssetURLs(in)
	assert.Empty(t, got)
}

func TestExtractAssetURLs_AltWithUnicodeAndSpaces(t *testing.T) {
	in := "![中文 alt](https://uploads.linear.app/u.png)"
	got := extractAssetURLs(in)
	require.Len(t, got, 1)
	assert.Equal(t, "中文 alt", got[0].altOrName)
}

func TestExtractAssetURLs_AltCanBeEmpty(t *testing.T) {
	in := "![](https://uploads.linear.app/u.png)"
	got := extractAssetURLs(in)
	require.Len(t, got, 1)
	assert.Empty(t, got[0].altOrName)
}

// fakeAssetClient is a Client double for resolver tests; it overrides
// DownloadAsset while delegating all other interface methods to the
// embedded fakeClient (see handler_test.go).
type fakeAssetClient struct {
	*fakeClient
	download func(url string) ([]byte, string, error)
}

func (f *fakeAssetClient) DownloadAsset(ctx context.Context, url string, maxBytes int) ([]byte, string, error) {
	data, contentType, err := f.download(url)
	if err != nil {
		return nil, "", err
	}
	if maxBytes > 0 && len(data) > maxBytes {
		return nil, "", errors.New("asset exceeds max size")
	}
	return data, contentType, nil
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
	got := r.Resolve(context.Background(), in)
	assert.Equal(t, in, got)
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
	assert.True(t, strings.HasPrefix(out, "see <media:image "), "expected placeholder prefix, got %q", out)
	assert.True(t, strings.HasSuffix(out, "!"), "expected suffix preserved, got %q", out)
	assert.Contains(t, out, `name="diagram"`)
	assert.Contains(t, out, `path="`)
}

// TestResolver_Resolve_Fallback covers the cases where Resolve cannot inline
// the asset (HTTP failure, over size limit) and must fall back to a
// placeholder that retains the original URL, as well as the link-vs-image
// placeholder type selection.
func TestResolver_Resolve_Fallback(t *testing.T) {
	cases := []struct {
		name            string
		download        func(url string) ([]byte, string, error)
		maxSize         int
		in              string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "HTTPFailureFallsBackToOriginalURL",
			download: func(url string) ([]byte, string, error) {
				return nil, "", errors.New("403 forbidden")
			},
			maxSize:         10 * 1024 * 1024,
			in:              "look ![pic](https://uploads.linear.app/x.png)",
			wantContains:    []string{"<media:image", "Original: https://uploads.linear.app/x.png"},
			wantNotContains: []string{`path="`},
		},
		{
			name: "LinkFallbackUsesDocumentType",
			download: func(url string) ([]byte, string, error) {
				return nil, "", errors.New("timeout")
			},
			maxSize:      10 * 1024 * 1024,
			in:           "ref [spec.pdf](https://uploads.linear.app/x.pdf)",
			wantContains: []string{"<media:document"},
		},
		{
			name: "SizeLimitExceededFallsBack",
			download: func(url string) ([]byte, string, error) {
				return make([]byte, 11), "image/png", nil
			},
			maxSize:         10,
			in:              "x ![big](https://uploads.linear.app/big.png)",
			wantContains:    []string{"Original: https://uploads.linear.app/big.png"},
			wantNotContains: []string{`path="`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &resolver{
				client:  newFakeResolverClient(tc.download),
				media:   media.NewService(),
				maxSize: tc.maxSize,
			}
			out := r.Resolve(context.Background(), tc.in)
			for _, s := range tc.wantContains {
				assert.Contains(t, out, s)
			}
			for _, s := range tc.wantNotContains {
				assert.NotContains(t, out, s)
			}
		})
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
	assert.Contains(t, out, `path="`)
	assert.Contains(t, out, "Original: https://uploads.linear.app/bad.png")
	assert.True(t, strings.HasPrefix(out, "a "), "surrounding text not preserved: %q", out)
	assert.True(t, strings.HasSuffix(out, " c"), "surrounding text not preserved: %q", out)
}

// fakeUploaderClient lets the test inject a FileUpload implementation.
type fakeUploaderClient struct {
	*fakeClient
	upload func(name, mime string, size int) (FileUploadTicket, error)
}

func (f *fakeUploaderClient) FileUpload(ctx context.Context, name, mime string, size int) (FileUploadTicket, error) {
	return f.upload(name, mime, size)
}

func TestUploader_UploadImage_ReturnsImageMarkdown(t *testing.T) {
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
	require.NoError(t, os.WriteFile(imgPath, []byte("PNGBYTES"), 0o644))

	u := &uploader{
		client: &fakeUploaderClient{fakeClient: &fakeClient{}, upload: func(name, mime string, size int) (FileUploadTicket, error) {
			assert.Equal(t, "foo.png", name)
			assert.NotEmpty(t, mime)
			assert.Equal(t, len("PNGBYTES"), size)
			return FileUploadTicket{
				AssetURL:  "https://uploads.linear.app/asset.png",
				UploadURL: s3.URL + "/sig",
				Headers:   map[string]string{"X-Test": "yes"},
			}, nil
		}},
		media:   media.NewService(),
		maxSize: 10 * 1024 * 1024,
		http:    http.DefaultClient,
	}

	md, err := u.Upload(context.Background(), imgPath)
	require.NoError(t, err)
	assert.Equal(t, "![foo.png](https://uploads.linear.app/asset.png)", md)
	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "PNGBYTES", string(gotBody))
	assert.Equal(t, "yes", gotHeaders["X-Test"])
}

func TestUploader_UploadDocument_ReturnsLinkMarkdown(t *testing.T) {
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer s3.Close()

	tmp := t.TempDir()
	pdf := tmp + "/spec.pdf"
	require.NoError(t, os.WriteFile(pdf, []byte("%PDF-1.4 ..."), 0o644))

	u := &uploader{
		client: &fakeUploaderClient{fakeClient: &fakeClient{}, upload: func(string, string, int) (FileUploadTicket, error) {
			return FileUploadTicket{
				AssetURL:  "https://uploads.linear.app/spec.pdf",
				UploadURL: s3.URL + "/sig",
			}, nil
		}},
		media:   media.NewService(),
		maxSize: 10 * 1024 * 1024,
		http:    http.DefaultClient,
	}

	md, err := u.Upload(context.Background(), pdf)
	require.NoError(t, err)
	assert.Equal(t, "[spec.pdf](https://uploads.linear.app/spec.pdf)", md)
}

func TestUploader_FileMissing_ReturnsError(t *testing.T) {
	u := &uploader{
		client:  &fakeUploaderClient{fakeClient: &fakeClient{}, upload: nil},
		media:   media.NewService(),
		maxSize: 10 * 1024 * 1024,
		http:    http.DefaultClient,
	}
	_, err := u.Upload(context.Background(), "/no/such/file.png")
	require.Error(t, err)
}

func TestUploader_FileTooLarge_RejectsBeforeMutation(t *testing.T) {
	tmp := t.TempDir()
	p := tmp + "/big.png"
	require.NoError(t, os.WriteFile(p, make([]byte, 11), 0o644))
	called := false
	u := &uploader{
		client: &fakeUploaderClient{fakeClient: &fakeClient{}, upload: func(string, string, int) (FileUploadTicket, error) {
			called = true
			return FileUploadTicket{}, nil
		}},
		media:   media.NewService(),
		maxSize: 10,
		http:    http.DefaultClient,
	}
	_, err := u.Upload(context.Background(), p)
	require.Error(t, err)
	assert.False(t, called, "FileUpload should not be invoked when over limit")
}

func TestUploader_PUTNon2xx_ReturnsError(t *testing.T) {
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer s3.Close()

	tmp := t.TempDir()
	p := tmp + "/foo.png"
	require.NoError(t, os.WriteFile(p, []byte("PNG"), 0o644))

	u := &uploader{
		client: &fakeUploaderClient{fakeClient: &fakeClient{}, upload: func(string, string, int) (FileUploadTicket, error) {
			return FileUploadTicket{
				AssetURL:  "https://uploads.linear.app/foo.png",
				UploadURL: s3.URL + "/sig",
			}, nil
		}},
		media:   media.NewService(),
		maxSize: 10 * 1024 * 1024,
		http:    http.DefaultClient,
	}
	_, err := u.Upload(context.Background(), p)
	require.Error(t, err)
}

func TestUploader_FileUploadMutationFails_ReturnsError(t *testing.T) {
	tmp := t.TempDir()
	p := tmp + "/foo.png"
	require.NoError(t, os.WriteFile(p, []byte("PNG"), 0o644))
	u := &uploader{
		client: &fakeUploaderClient{fakeClient: &fakeClient{}, upload: func(string, string, int) (FileUploadTicket, error) {
			return FileUploadTicket{}, errors.New("graphql denied")
		}},
		media:   media.NewService(),
		maxSize: 10 * 1024 * 1024,
		http:    http.DefaultClient,
	}
	_, err := u.Upload(context.Background(), p)
	require.Error(t, err)
}
