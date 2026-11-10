package chat

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

const (
	previewLimit    = 1 << 20
	highlightLimit  = 128 << 10
	previewMaxWidth = 896
	previewPad      = 24
	previewSpinner  = 24
	previewWaitPad  = 48
	previewGlyph    = "file-exclamation-point"

	previewMarkdown = "markdown"
	previewText     = "text"
	previewCode     = "code"
)

type previewStatus int

const (
	previewLoading previewStatus = iota
	previewReady
	previewTooLarge
	previewUnsupported
	previewFailed
)

var previewKinds = map[string]string{
	"md":       previewMarkdown,
	"markdown": previewMarkdown,
	"txt":      previewText,
	"log":      previewText,
	"csv":      previewText,
}

var languageAliases = map[string]string{"h": "c"}

var lineEndings = strings.NewReplacer("\r\n", "\n", "\r", "\n")

func previewKind(name string) string {
	ext := extension(name)
	if kind, ok := previewKinds[ext]; ok {
		return kind
	}
	if ext != "" && fileCategory(name) == categoryCode {
		return previewCode
	}
	return ""
}

func codeLanguage(name string) string {
	ext := extension(name)
	if alias, ok := languageAliases[ext]; ok {
		return alias
	}
	return ext
}

func decodeText(data []byte) (string, bool) {
	var text string
	ok := true
	switch {
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}):
		text, ok = decodeWith(unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM), data)
	case bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		text, ok = decodeWith(unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM), data)
	case bytes.IndexByte(data, 0) >= 0:
		return "", false
	case utf8.Valid(data):
		text = string(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	default:
		text, ok = decodeWith(simplifiedchinese.GB18030, data)
	}
	if !ok {
		return "", false
	}
	return lineEndings.Replace(text), true
}

func decodeWith(enc encoding.Encoding, data []byte) (string, bool) {
	out, err := enc.NewDecoder().Bytes(data)
	if err != nil || bytes.ContainsRune(out, utf8.RuneError) {
		return "", false
	}
	return string(out), true
}

func fenceCode(code, language string) string {
	longest, run := 2, 0
	for i := 0; i < len(code); i++ {
		if code[i] != '`' {
			run = 0
			continue
		}
		run++
		longest = max(longest, run)
	}
	fence := strings.Repeat("`", longest+1)
	return fence + language + "\n" + strings.TrimSuffix(code, "\n") + "\n" + fence
}

type previewState struct {
	open bool
	name string
}

type previewLoad struct {
	status   previewStatus
	text     string
	markdown string
}

func newPreviewLoad(name string, data []byte, err error) *previewLoad {
	switch {
	case errors.Is(err, api.ErrMediaTooLarge):
		return &previewLoad{status: previewTooLarge}
	case err != nil:
		log.Printf("chat: preview %s: %v", name, err)
		return &previewLoad{status: previewFailed}
	}
	text, ok := decodeText(data)
	if !ok {
		return &previewLoad{status: previewUnsupported}
	}
	p := &previewLoad{status: previewReady, text: strings.TrimSuffix(text, "\n")}
	switch kind := previewKind(name); {
	case kind == previewMarkdown:
		p.markdown = text
	case kind == previewCode && len(text) <= highlightLimit:
		p.markdown = fenceCode(text, codeLanguage(name))
	}
	return p
}

func (v *View) openPreview(name string) {
	v.preview = previewState{open: true, name: name}
	if p, ok := v.previews[name]; ok && p.status != previewFailed {
		return
	}
	v.loadPreview(name)
}

func (v *View) loadPreview(name string) {
	v.previews[name] = &previewLoad{status: previewLoading}
	gen := v.mediaGen
	v.s.ReadMedia(name, previewLimit, func(data []byte, err error) {
		if v.mediaGen != gen {
			return
		}
		v.previews[name] = newPreviewLoad(name, data, err)
	})
}

func (v *View) textPreview(c *ui.Context, tk theme.Tokens) {
	p := &v.preview
	if !p.open {
		return
	}
	w, h := c.Size()
	name := p.name
	ui.DialogBase(c, &p.open, func(backdrop, panel ui.Element) {
		v.viewerFrame(tk, backdrop, panel, w, h, locales.ChatPreviewFile(name))
		if panel.Clicked() {
			p.open = false
		}
		v.viewerBar(c, tk, name, w, viewerAction{
			text:  locales.ChatDownload(),
			icon:  "download",
			busy:  v.opening[name],
			click: func() { v.downloadMedia(name) },
		}, locales.ChatClosePreview(), func() { p.open = false })
		v.previewStage(c, tk, name, w)
	})
}

func (v *View) previewStage(c *ui.Context, tk theme.Tokens, name string, w float32) {
	pad := viewerPad(w)
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).AlignItems(ui.Center).Padding(0, pad, lightboxBottom, pad).Children(func() {
			box := ui.Column(c).Width(min(previewMaxWidth, w-2*pad)).MinWidth(0).Padding(previewPad).
				Radius(theme.RadiusSurface).Border(1, tk.Border).Background(tk.Surface).Label(name)
			theme.WithShadow(keepOpen(box), tk.ShadowFloating).Children(func() {
				v.previewBody(c, tk, name)
			})
		})
	})
}

func (v *View) previewBody(c *ui.Context, tk theme.Tokens, name string) {
	p := v.previews[name]
	if p == nil {
		return
	}
	download := func() {
		if kit.Button(c, tk, locales.ChatDownload(), kit.ButtonOpts{Variant: kit.Secondary, Icon: "download", Loading: v.opening[name]}).Clicked() {
			v.downloadMedia(name)
		}
	}
	switch p.status {
	case previewLoading:
		ui.Column(c).AlignItems(ui.Center).Padding(previewWaitPad).Label(locales.ChatPreviewLoading()).Children(func() {
			kit.Spinner(c, tk, previewSpinner)
		})
	case previewFailed:
		kit.EmptyState(c, tk, previewGlyph, locales.ChatPreviewFailedTitle(), locales.ChatPreviewFailed(), func() {
			if kit.Button(c, tk, locales.CommonRetry(), kit.ButtonOpts{Variant: kit.Secondary, Icon: "rotate-cw"}).Clicked() {
				v.loadPreview(name)
			}
		})
	case previewTooLarge:
		kit.EmptyState(c, tk, previewGlyph, locales.ChatPreviewTooLargeTitle(), locales.ChatPreviewTooLarge(), download)
	case previewUnsupported:
		kit.EmptyState(c, tk, previewGlyph, locales.ChatPreviewUnsupportedTitle(), locales.ChatPreviewUnsupported(), download)
	default:
		v.previewContent(c, tk, p)
	}
}

func (v *View) previewContent(c *ui.Context, tk theme.Tokens, p *previewLoad) {
	switch {
	case strings.TrimSpace(p.text) == "":
		ui.Text(c, locales.ChatPreviewEmpty()).TextColor(tk.TextSecondary)
	case p.markdown != "":
		ui.Column(c).MinWidth(0).TextColor(tk.Foreground).FontSize(bodySize).LineHeight(bodyLineHeight).Children(func() {
			markdown.View(c, p.markdown, v.md)
		})
	default:
		ui.Column(c).MinWidth(0).Selectable().Children(func() {
			ui.Text(c, p.text).Font(theme.FontMono).FontSize(bodySize * codeSizeEm).LineHeight(codeLineHeight).
				TextColor(tk.Foreground).FontFeatures(noLigatures...)
		})
	}
}
