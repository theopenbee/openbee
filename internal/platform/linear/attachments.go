package linear

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

// assetMatch is one extracted markdown image or link pointing at uploads.linear.app.
type assetMatch struct {
	span      [2]int // byte offsets [start, end) in the original text
	url       string
	altOrName string
	isImage   bool
}

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

	fence := []byte("```")
	for i := 0; i+3 <= len(b); {
		if !bytes.Equal(b[i:i+3], fence) {
			i++
			continue
		}
		j := i + 3
		for j+3 <= len(b) && !bytes.Equal(b[j:j+3], fence) {
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
	isImage := strings.HasPrefix(mime, "image/") || isImageExt(filepath.Ext(name))

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

	if isImage {
		return fmt.Sprintf("![%s](%s)", name, ticket.AssetURL), nil
	}
	return fmt.Sprintf("[%s](%s)", name, ticket.AssetURL), nil
}

func isImageExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}
