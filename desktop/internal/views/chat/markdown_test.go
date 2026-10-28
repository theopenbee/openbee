package chat_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/views/chat"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	bodyLine     = 14 * 1.65
	imageTimeout = 3 * time.Second
	smallImages  = 70
	cachedImages = 64
	bigImageW    = 2000
	bigImageH    = 1500
	bigImages    = 12
	cacheBytes   = 128 << 20
	imageShown   = 200
)

type savedFile struct {
	name string
	data string
}

type fakeSaver struct {
	mu    sync.Mutex
	saved []savedFile
}

func (s *fakeSaver) save(name string, data []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, savedFile{name: name, data: string(data)})
	return "/Users/someone/Downloads/" + name, nil
}

func (s *fakeSaver) files() []savedFile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.saved)
}

func workerSays(content string) func(st *fakeapi.State) {
	return seedChat(team(), workerCID, message("w", "worker", "w1", content, time.Now().Add(-time.Hour)))
}

func withMedia(name string, data []byte) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		if st.Media == nil {
			st.Media = map[string][]byte{}
		}
		st.Media[name] = data
	}
}

func pngImage(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 160, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (f *fixture) awaitImage(alt string, w, h float32) {
	f.t.Helper()
	deadline := time.Now().Add(imageTimeout)
	for {
		f.Settle()
		f.T.Frame()
		if r, ok := f.T.Find(alt); ok && r.W == w && r.H == h {
			return
		}
		if time.Now().After(deadline) {
			r, ok := f.T.Find(alt)
			f.t.Fatalf("image %q did not load: box %v (shown %v); texts %q", alt, r, ok, f.T.Texts())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (f *fixture) chooseMenu(button, item string) {
	f.t.Helper()
	f.click(button)
	if err := f.T.ChooseMenuItem(item); err != nil {
		f.t.Fatalf("choose %q from %q: %v (menu %q)", item, button, err, f.T.Menu())
	}
	f.settleMotion()
}

func (f *fixture) count(text string) int {
	n := 0
	for _, s := range f.T.Texts() {
		if s == text {
			n++
		}
	}
	return n
}

func TestHardBreaksPreprocessor(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"single newline", "a\nb", "a  \nb"},
		{"paragraph break", "a\n\nb", "a\n\nb"},
		{"existing hard break", "a  \nb", "a  \nb"},
		{"backslash break", "a\\\nb", "a\\\nb"},
		{"carriage returns", "a\r\nb", "a  \r\nb"},
		{"quote lines", "> a\n> b", "> a  \n> b"},
		{"fenced code untouched", "```\nx\ny\n```\nc\nd", "```\nx\ny\n```\nc  \nd"},
		{"tilde fence untouched", "~~~py\nx\ny\n~~~", "~~~py\nx\ny\n~~~"},
		{"shorter fence does not close", "````\n```\nx\n````\nz\nw", "````\n```\nx\n````\nz  \nw"},
		{"line before a fence", "text\n```go\ncode\nmore\n```", "text\n```go\ncode\nmore\n```"},
		{"fence in a list", "- a\n  ```\n  x\n  y\n  ```\n- b\n  c", "- a\n  ```\n  x\n  y\n  ```\n- b  \n  c"},
		{"unclosed fence", "```\nx\ny", "```\nx\ny"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := chat.HardBreaks(tc.in); got != tc.want {
				t.Fatalf("hardBreaks(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSingleNewlineBreaks(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", workerSays("a\nb"))
	r, ok := f.T.Find("a\nb")
	if !ok {
		t.Fatalf("no paragraph with a line break; texts %q", f.T.Texts())
	}
	if r.H < 2*bodyLine-1 {
		t.Fatalf("paragraph is %v tall, want two lines of %v", r.H, bodyLine)
	}
	f.wantHidden("a b")
}

func TestCodeFenceKeepsNewlines(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", workerSays("Run this:\n```go\nfunc main() {\n\tprintln(\"hi\")\n}\n```\nthen\nreport"))
	f.wantShown("func main() {\n\tprintln(\"hi\")\n}", "go", "then\nreport")
}

func TestCodeBlockCopy(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", workerSays("```sh\nmake dist\nmake publish\n```"))
	f.click(locales.ArcCodeBlockCopy())
	if got := f.T.Clipboard(); got != "make dist\nmake publish" {
		t.Fatalf("clipboard = %q", got)
	}
}

func TestCodeBlockDownload(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", workerSays("```go\nfunc main() {}\n```"))
	saver := &fakeSaver{}
	f.view.SetSaver(saver.save)
	f.click(locales.CodeDownload())
	f.Settle()
	want := []savedFile{{name: "snippet.go", data: "func main() {}\n"}}
	if got := saver.files(); !slices.Equal(got, want) {
		t.Fatalf("saved %q, want %q", got, want)
	}
	f.settleMotion()
	f.wantShown(locales.DownloadSaved("snippet.go"))
}

func TestCodeDownloadNames(t *testing.T) {
	for lang, want := range map[string]string{
		"go":         "snippet.go",
		"python":     "snippet.py",
		"TypeScript": "snippet.ts",
		"bash":       "snippet.sh",
		"":           "snippet.txt",
		"../evil":    "snippet.txt",
	} {
		if got := chat.SnippetName(lang); got != want {
			t.Errorf("snippetName(%q) = %q, want %q", lang, got, want)
		}
	}
}

const twoByTwo = "| h1 | h2 |\n| --- | --- |\n| 1 | 2 |"

func TestTableToolbarCopyCSV(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", workerSays(twoByTwo))
	f.click(locales.TableCopy())
	want := []string{locales.TableCopyMarkdown(), locales.TableCopyCsv(), locales.TableCopyTsv()}
	if got := f.T.Menu(); !slices.Equal(got, want) {
		t.Fatalf("copy menu = %q, want %q", got, want)
	}
	if err := f.T.ChooseMenuItem(locales.TableCopyCsv()); err != nil {
		t.Fatal(err)
	}
	f.settleMotion()
	if got := f.T.Clipboard(); got != "h1,h2\n1,2" {
		t.Fatalf("clipboard = %q", got)
	}
}

func TestTableDownloadMarkdown(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", workerSays(twoByTwo))
	saver := &fakeSaver{}
	f.view.SetSaver(saver.save)
	f.chooseMenu(locales.TableDownload(), locales.TableCopyMarkdown())
	f.Settle()
	want := []savedFile{{name: "table.md", data: "| h1 | h2 |\n| --- | --- |\n| 1 | 2 |"}}
	if got := saver.files(); !slices.Equal(got, want) {
		t.Fatalf("saved %q, want %q", got, want)
	}
}

func TestTableExports(t *testing.T) {
	src := "| name | note |\n| --- | --- |\n| **a** | x, \"y\" |\n| b | p \\| q |\n| c |"
	md, csv, tsv := chat.TableExports(src)
	if want := "| name | note |\n| --- | --- |\n| a | x, \"y\" |\n| b | p \\| q |\n| c |  |"; md != want {
		t.Errorf("markdown = %q, want %q", md, want)
	}
	if want := "name,note\na,\"x, \"\"y\"\"\"\nb,p | q\nc,"; csv != want {
		t.Errorf("csv = %q, want %q", csv, want)
	}
	if want := "name\tnote\na\tx, \"y\"\nb\tp | q\nc\t"; tsv != want {
		t.Errorf("tsv = %q, want %q", tsv, want)
	}
}

func TestTableFullscreen(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", workerSays(twoByTwo))
	if n := f.count("h1"); n != 1 {
		t.Fatalf("%d header cells before full screen, want 1", n)
	}
	f.click(locales.TableFullscreen())
	if n := f.count("h1"); n != 2 {
		t.Fatalf("%d header cells in full screen, want 2; texts %q", n, f.T.Texts())
	}
	f.wantShown(locales.ArcDialogClose())
	f.T.Key(0, ui.KeyEscape)
	f.settleMotion()
	if n := f.count("h1"); n != 1 {
		t.Fatalf("%d header cells after Escape, want 1", n)
	}
}

func TestLinkOpensDialogNotBrowser(t *testing.T) {
	url := "https://example.com/guide"
	f := newFixture(t, wideW, wideH, "w1", workerSays("Read [the guide]("+url+") first."))
	f.click("the guide")
	f.wantShown(locales.LinkDialogTitle(), locales.LinkDialogDescription(), url)
	if opened := f.T.OpenedURLs(); len(opened) != 0 {
		t.Fatalf("opened %q before confirming", opened)
	}
	f.click(locales.LinkOpen())
	if opened := f.T.OpenedURLs(); !slices.Equal(opened, []string{url}) {
		t.Fatalf("opened %q, want %q", opened, url)
	}
	f.wantHidden(locales.LinkDialogTitle())
}

func TestLinkDialogCopiesLink(t *testing.T) {
	url := "https://example.com/a?b=c"
	f := newFixture(t, wideW, wideH, "w1", workerSays("See <"+url+">."))
	f.click(url)
	f.click(locales.LinkCopy())
	if got := f.T.Clipboard(); got != url {
		t.Fatalf("clipboard = %q, want %q", got, url)
	}
	if opened := f.T.OpenedURLs(); len(opened) != 0 {
		t.Fatalf("copying opened %q", opened)
	}
}

func TestUnsafeLinkDoesNotOpen(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", workerSays("[run me](file:///etc/passwd)"))
	f.click("run me")
	f.wantShown(locales.LinkDialogTitle(), "file:///etc/passwd")
	f.click(locales.LinkOpen())
	if opened := f.T.OpenedURLs(); len(opened) != 0 {
		t.Fatalf("opened %q", opened)
	}
}

func TestMarkdownImageLoadsMedia(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", both(workerSays("![chart](/api/local/media/chart.png?token=stale)"),
		withMedia("chart.png", pngImage(t, 120, 80))))
	f.awaitImage("chart", 120, 80)
	calls := f.Fake.Calls(http.MethodGet, "/api/local/media/")
	if len(calls) != 1 || calls[0].Path != "/api/local/media/chart.png" || strings.Contains(calls[0].Query, "token") {
		t.Fatalf("media requests = %+v, want one GET of chart.png without a token", calls)
	}
}

func TestMarkdownImageBasename(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", both(workerSays("![diagram](1f2e_diagram.png)"),
		withMedia("1f2e_diagram.png", pngImage(t, 64, 48))))
	f.awaitImage("diagram", 64, 48)
}

func TestTallImageCollapses(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", both(workerSays("Here is the chart."), withMedia("tall.png", pngImage(t, 40, 1200))))
	f.Fake.Reply(workerCID, api.ChatMessage{Content: "![tall](/api/local/media/tall.png)"})
	deadline := time.Now().Add(imageTimeout)
	for {
		f.Fix.Settle(t)
		f.T.Frame()
		if r, ok := f.T.Find("tall"); ok && r.H > imageShown {
			break
		}
		if f.shows(locales.CommonShowMore()) {
			t.Fatalf("the message collapsed before its image loaded; texts %q", f.T.Texts())
		}
		if time.Now().After(deadline) {
			t.Fatalf("the tall image did not load; texts %q", f.T.Texts())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !f.shows(locales.CommonShowMore()) {
		t.Fatalf("the frame that showed the tall image did not collapse the message; texts %q", f.T.Texts())
	}
}

func TestImageCacheEvicts(t *testing.T) {
	small := pngImage(t, 1, 1)
	big := pngImage(t, bigImageW, bigImageH)
	ic := chat.NewImageCache(func(name string) ([]byte, error) {
		if strings.HasPrefix(name, "big") {
			return big, nil
		}
		return small, nil
	})
	for i := range smallImages {
		if _, err := ic.Load(fmt.Sprintf("/api/local/media/small-%02d.png", i)); err != nil {
			t.Fatal(err)
		}
	}
	if n, size := ic.Len(), ic.Bytes(); n != cachedImages || size != cachedImages*4 {
		t.Fatalf("after %d images: %d kept, %d bytes; want %d kept, %d bytes", smallImages, n, size, cachedImages, cachedImages*4)
	}
	if _, err := ic.Load("/api/local/media/big-00.png"); err != nil {
		t.Fatal(err)
	}
	if n, size := ic.Len(), ic.Bytes(); n != cachedImages || size != (cachedImages-1)*4+bigImageW*bigImageH*4 {
		t.Fatalf("after a %d×%d image: %d kept, %d bytes", bigImageW, bigImageH, n, size)
	}
	for i := 1; i < bigImages; i++ {
		if _, err := ic.Load(fmt.Sprintf("big-%02d.png", i)); err != nil {
			t.Fatal(err)
		}
	}
	if size := ic.Bytes(); size > cacheBytes {
		t.Fatalf("%d bytes kept, want at most %d", size, cacheBytes)
	}
	if n, want := ic.Len(), bigImages-1; n != want {
		t.Fatalf("%d images kept after %d large ones, want the %d newest", n, bigImages, want)
	}
}
