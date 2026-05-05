package linear

import (
	"bytes"
	"regexp"
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
