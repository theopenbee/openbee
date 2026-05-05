package linear

import (
	"reflect"
	"testing"
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
