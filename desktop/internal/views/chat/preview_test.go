package chat_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/views/chat"
	"github.com/theopenbee/openbee/desktop/locales"
)

func TestPreviewKind(t *testing.T) {
	for name, want := range map[string]string{
		"abc_README.md":   "markdown",
		"notes.MARKDOWN":  "markdown",
		"a.txt":           "text",
		"server.log":      "text",
		"rows.csv":        "text",
		"main.go":         "code",
		"config.yaml":     "code",
		"data.json":       "code",
		"uploads/x.py":    "code",
		"photo.png":       "",
		"report.pdf":      "",
		"bundle.zip":      "",
		"README":          "",
		"folder.md/file":  "",
		"C:\\dir\\a.bat":  "",
		"x.md\\notes.bin": "",
	} {
		if got := chat.PreviewKind(name); got != want {
			t.Errorf("previewKind(%q) = %q, want %q", name, got, want)
		}
	}
	if got := chat.CodeLanguage("App.TSX"); got != "tsx" {
		t.Errorf("codeLanguage(App.TSX) = %q, want tsx", got)
	}
	if got := chat.CodeLanguage("lib.h"); got != "c" {
		t.Errorf("codeLanguage(lib.h) = %q, want c", got)
	}
}

func TestDecodeText(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want string
		ok   bool
	}{
		{"utf-8 with a byte order mark", []byte("\xef\xbb\xbf你好 world"), "你好 world", true},
		{"gb18030", []byte{0xc4, 0xe3, 0xba, 0xc3}, "你好", true},
		{"utf-16le", []byte{0xff, 0xfe, 0x60, 0x4f, 0x7d, 0x59}, "你好", true},
		{"utf-16be", []byte{0xfe, 0xff, 0x4f, 0x60, 0x59, 0x7d}, "你好", true},
		{"line endings", []byte("a\r\nb\rc\n"), "a\nb\nc\n", true},
		{"binary", []byte{0x89, 'P', 'N', 'G', 0x00, 0x01}, "", false},
		{"no encoding fits", []byte{'a', 0xff}, "", false},
		{"empty", nil, "", true},
	} {
		got, ok := chat.DecodeText(tc.data)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s: decodeText = %q, %v; want %q, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestFenceCode(t *testing.T) {
	if got := chat.FenceCode("let a = 1\n", "ts"); got != "```ts\nlet a = 1\n```" {
		t.Errorf("fenceCode = %q", got)
	}
	if got := chat.FenceCode("a ```` b", "md"); got != "`````md\na ```` b\n`````" {
		t.Errorf("fenceCode with backticks = %q", got)
	}
}

func previewChat(t *testing.T, media map[string][]byte, names ...string) *fixture {
	t.Helper()
	m := withAttachments(message("m1", "worker", "w1", "Files.", time.Now().Add(-time.Hour)), names...)
	return newFixture(t, wideW, wideH, "w1", attached(media, m))
}

func (f *fixture) openPreview(name string) {
	f.t.Helper()
	f.click(locales.ChatPreviewFile(name))
	f.wantShown(locales.ChatClosePreview())
	f.Fix.Await(f.t, func() bool {
		f.T.Frame()
		return !f.shows(locales.ChatPreviewLoading())
	})
	f.settleMotion()
}

func (f *fixture) showsPrefix(prefix string) bool {
	for _, s := range f.T.Texts() {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func TestMarkdownPreviewRenders(t *testing.T) {
	const name = "m1_notes.md"
	f := previewChat(t, map[string][]byte{name: []byte("# Release notes\r\n\r\n- Preview **text** files\r\n")}, "uploads/"+name)
	o := &opener{}
	f.view.SetOpener(o.open)
	f.view.SetRevealer(o.open)
	f.openPreview(name)
	f.wantShown("Release notes", locales.ChatDownload())
	if f.showsPrefix("# Release") {
		t.Fatal("the markdown source is shown, want it rendered")
	}
	if n := len(o.paths()); n != 0 {
		t.Fatalf("previewing launched %q, want nothing opened", o.paths())
	}
	if entries, _ := os.ReadDir(f.Fix.DownloadDir); len(entries) != 0 {
		t.Fatalf("downloads entries = %v, want a preview to leave Downloads alone", entries)
	}

	f.press(ui.KeyEscape)
	f.wantHidden(locales.ChatClosePreview())
	f.openPreview(name)
	f.wantShown("Release notes")
	if n := f.mediaGets(name); n != 1 {
		t.Fatalf("%d downloads of %s, want reopening to reuse the first read", n, name)
	}
}

func TestTextPreviewDecodesAndKeepsLines(t *testing.T) {
	gbk := []byte{0xc4, 0xe3, 0xba, 0xc3, '\r', '\n', 'o', 'k'}
	f := previewChat(t, map[string][]byte{"gbk.txt": gbk}, "gbk.txt")
	f.openPreview("gbk.txt")
	f.wantShown("你好\nok")
}

func TestCodePreviewHighlights(t *testing.T) {
	src := "package main\n\nfunc main() {}\n"
	f := previewChat(t, map[string][]byte{"main.go": []byte(src)}, "main.go")
	f.openPreview("main.go")
	f.wantShown(locales.ArcCodeBlockRegion("go"))
}

func TestLargeCodePreviewIsPlainText(t *testing.T) {
	line := "export const value = 1\n"
	src := strings.Repeat(line, chat.RenderLimit/len(line)+1)
	f := previewChat(t, map[string][]byte{"big.ts": []byte(src)}, "big.ts")
	f.openPreview("big.ts")
	f.wantHidden(locales.ArcCodeBlockRegion("ts"))
	if !f.showsPrefix(line) {
		t.Fatalf("plain text of big.ts is not shown; texts %q", f.T.Texts())
	}
}

func TestLargeMarkdownPreviewIsPlainText(t *testing.T) {
	line := "# Heading\n"
	src := strings.Repeat(line, chat.RenderLimit/len(line)+1)
	f := previewChat(t, map[string][]byte{"big.md": []byte(src)}, "big.md")
	f.openPreview("big.md")
	if !f.showsPrefix(line) {
		t.Fatalf("the source of big.md is not shown as plain text; texts %q", f.T.Texts())
	}
}

func TestPreviewLinkAndTableOpenAbove(t *testing.T) {
	url := "https://example.com/guide"
	doc := "Read [the guide](" + url + ").\n\n| h1 | h2 |\n| --- | --- |\n| a | b |\n"
	f := previewChat(t, map[string][]byte{"doc.md": []byte(doc)}, "doc.md")
	f.openPreview("doc.md")
	f.click("the guide")
	f.wantShown(locales.LinkDialogTitle())
	f.click(locales.LinkOpen())
	if opened := f.T.OpenedURLs(); len(opened) != 1 || opened[0] != url {
		t.Fatalf("opened %q, want %q", opened, url)
	}
	f.wantHidden(locales.LinkDialogTitle())
	f.wantShown(locales.ChatClosePreview())

	f.click(locales.TableFullscreen())
	if n := f.count("h1"); n != 2 {
		t.Fatalf("%d header cells with the table in full screen, want 2; texts %q", n, f.T.Texts())
	}
	f.click(locales.ArcDialogClose())
	if n := f.count("h1"); n != 1 {
		t.Fatalf("%d header cells after closing the full-screen table, want 1", n)
	}
	f.wantShown(locales.ChatClosePreview())
}

func TestPreviewTooLargeOffersDownload(t *testing.T) {
	data := []byte(strings.Repeat("a", chat.PreviewLimit+1))
	f := previewChat(t, map[string][]byte{"big.log": data, "a.txt": []byte("hello")}, "big.log", "a.txt")
	o := &opener{}
	f.view.SetOpener(o.open)
	f.view.SetRevealer(o.open)
	f.openPreview("a.txt")
	bar := f.countText(locales.ChatDownload())
	f.press(ui.KeyEscape)
	f.openPreview("big.log")
	f.wantShown(locales.ChatPreviewTooLargeTitle(), locales.ChatPreviewTooLarge())
	if n := f.countText(locales.ChatDownload()); bar == 0 || n != 2*bar {
		t.Fatalf("download shown %d times, want the bar's %d twice over with the notice's button", n, bar)
	}
	f.click(locales.ChatDownload())
	want := filepath.Join(f.Fix.DownloadDir, "big.log")
	f.Fix.Await(t, func() bool {
		info, err := os.Stat(want)
		return err == nil && info.Size() == int64(len(data))
	})
	f.settleMotion()
	f.wantShown(locales.FileSavedToDownloads(), locales.ChatClosePreview())
	if n := len(o.paths()); n != 0 {
		t.Fatalf("downloading launched %q, want the file only saved", o.paths())
	}
}

func TestPreviewUnsupportedAndEmpty(t *testing.T) {
	f := previewChat(t, map[string][]byte{
		"binary.txt": {0x89, 'P', 'N', 'G', 0x00, 0x01},
		"empty.txt":  {},
	}, "binary.txt", "empty.txt")
	f.openPreview("binary.txt")
	f.wantShown(locales.ChatPreviewUnsupportedTitle())
	f.press(ui.KeyEscape)
	f.openPreview("empty.txt")
	f.wantShown(locales.ChatPreviewEmpty())
}

func TestPreviewFailureRetries(t *testing.T) {
	f := previewChat(t, nil, "late.md")
	f.openPreview("late.md")
	f.wantShown(locales.ChatPreviewFailedTitle())
	f.Fake.Update(func(st *fakeapi.State) { st.Media["late.md"] = []byte("# Arrived") })
	f.click(locales.CommonRetry())
	f.Fix.Await(t, func() bool {
		f.T.Frame()
		return f.shows("Arrived")
	})
	f.wantHidden(locales.ChatPreviewFailedTitle())
}

func TestPreviewDownloadFailureToasts(t *testing.T) {
	f := previewChat(t, nil, "gone.md")
	f.openPreview("gone.md")
	f.click(locales.ChatDownload())
	f.Fix.Await(t, func() bool { return f.mediaGets("gone.md") == 2 })
	f.settleMotion()
	f.wantShown(locales.FileSaveFailed())
	f.wantHidden(locales.FileSavedToDownloads(), locales.FileOpenFailed())
}

func TestPreviewClosesFromButtonAndBackdrop(t *testing.T) {
	f := previewChat(t, map[string][]byte{"a.txt": []byte("hello")}, "a.txt")
	f.openPreview("a.txt")
	f.click(locales.ChatClosePreview())
	f.wantHidden(locales.ChatClosePreview())

	f.openPreview("a.txt")
	r, ok := f.T.Find("hello")
	if !ok {
		t.Fatalf("preview text not shown; texts %q", f.T.Texts())
	}
	f.T.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	f.settleMotion()
	f.wantShown(locales.ChatClosePreview())

	f.T.ClickAt(lightboxEdge, wideH/2)
	f.settleMotion()
	f.wantHidden(locales.ChatClosePreview())
}
