package chat

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	headingOneSize   = 18
	headingTwoSize   = 16
	headingTop       = 20
	headingBelow     = 8
	blockGap         = 12
	itemGap          = 4
	listIndent       = 24
	quoteBarWidth    = 2
	codeSizeEm       = 0.875
	codeLineHeight   = 1.7
	codePadX         = 16
	codePadY         = 12
	codeChunkPad     = 12
	codeChunkTop     = 0.25 * bodySize
	codeHeaderHeight = 40
	codeHeaderStart  = 12
	codeHeaderEnd    = 4
	codeLabelSize    = 12
	toolBox          = 32
	toolGlyph        = 16
	hardBreak        = "  "
	snippetBase      = "snippet"
	plainExtension   = "txt"
	savedFileMode    = 0o644
)

var snippetExtensions = map[string]string{
	"bash": "sh", "c": "c", "c++": "cpp", "cpp": "cpp", "cs": "cs", "csharp": "cs", "css": "css",
	"csv": "csv", "diff": "diff", "dockerfile": "dockerfile", "go": "go", "golang": "go", "graphql": "graphql",
	"html": "html", "ini": "ini", "java": "java", "javascript": "js", "js": "js", "json": "json", "jsx": "jsx",
	"kotlin": "kt", "kt": "kt", "less": "less", "lua": "lua", "makefile": "mak", "markdown": "md", "md": "md",
	"objc": "m", "perl": "pl", "php": "php", "powershell": "ps1", "proto": "proto", "protobuf": "proto",
	"py": "py", "python": "py", "r": "r", "rb": "rb", "ruby": "rb", "rs": "rs", "rust": "rs", "scala": "scala",
	"scss": "scss", "sh": "sh", "shell": "sh", "sql": "sql", "svelte": "svelte", "swift": "swift", "toml": "toml",
	"ts": "ts", "tsx": "tsx", "typescript": "ts", "vue": "vue", "xml": "xml", "yaml": "yaml", "yml": "yml",
	"zig": "zig", "zsh": "zsh",
}

var noLigatures = []string{"liga=0", "calt=0"}

type bodyKey struct{}

type bodySource struct {
	content, markdown, preview string
	cut                        bool
}

func newMarkdown(v *View, loads *imageLoads) *markdown.Options {
	return &markdown.Options{
		Extensions: parser.GFM | parser.ExtFootnotes,
		OnLink:     v.showLink,
		LoadImage:  loads.track(v.images),
		CodeBlock:  v.codeBlock,
		Block:      v.block,
	}
}

func ems(px float32) float32 {
	return px / bodySize
}

func markdownTheme(tk theme.Tokens) *markdown.Theme {
	heading := func(size float32) markdown.HeadingStyle {
		return markdown.HeadingStyle{Size: ems(size), Weight: mediumWeight, Color: tk.Foreground, Top: ems(headingTop), Bottom: ems(headingBelow)}
	}
	th := &markdown.Theme{
		FontSize:            bodySize,
		CodeFont:            theme.FontMono,
		LineHeight:          bodyLineHeight,
		CodeLineHeight:      codeLineHeight,
		Text:                tk.Foreground,
		Muted:               tk.TextMuted,
		Link:                linkColor(tk),
		Strong:              tk.Foreground,
		StrongWeight:        mediumWeight,
		Code:                tk.Foreground,
		CodeBackground:      codeChip(tk),
		CodeSize:            codeSizeEm,
		BlockSpacing:        ems(blockGap),
		ItemSpacing:         ems(itemGap),
		ListIndent:          ems(listIndent),
		QuoteBar:            tk.BorderStrong,
		QuoteBarWidth:       quoteBarWidth,
		QuoteText:           tk.TextSecondary,
		Rule:                tk.BorderSubtle,
		CodeBlockBackground: tk.Surface,
		CodeBlockBorder:     tk.Border,
		CodeBlockRadius:     theme.RadiusInset,
		CodeBlockPadding:    ems(codeChunkPad),
		TableBorder:         tk.BorderSubtle,
		TableHeader:         tk.SurfaceMuted,
		Checkbox:            tk.ControlOn,
	}
	th.Headings[0] = heading(headingOneSize)
	th.Headings[1] = heading(headingTwoSize)
	for i := 2; i < len(th.Headings); i++ {
		th.Headings[i] = heading(bodySize)
	}
	return th
}

func codeChip(tk theme.Tokens) ui.Color {
	if tk.Dark {
		return tk.Surface
	}
	return tk.Border
}

func linkColor(tk theme.Tokens) ui.Color {
	if tk.Dark {
		return tk.Accent
	}
	return tk.AccentStrong
}

func (v *View) markdownTheme(tk theme.Tokens) *markdown.Theme {
	k := 0
	if tk.Dark {
		k = 1
	}
	if v.themes[k] == nil {
		v.themes[k] = markdownTheme(tk)
	}
	return v.themes[k]
}

func (v *View) renderBody(c *ui.Context, m model.Message, collapsed bool) bool {
	box := ui.Column(c).MinWidth(0)
	src := ui.Local(box, bodyKey{}, func() bodySource { return bodySource{} })
	if src.content != m.Content {
		md := hardBreaks(m.Content)
		preview, cut := collapsedPrefix(md)
		*src = bodySource{content: m.Content, markdown: md, preview: preview, cut: cut}
	}
	text, rows := src.markdown, 0
	if collapsed {
		text, rows = src.preview, collapsedTableRows
	}
	box.Children(func() {
		v.rowLimit = rows
		markdown.View(c, text, v.md)
		v.rowLimit = 0
	})
	return collapsed && src.cut
}

func hardBreaks(src string) string {
	if !strings.Contains(src, "\n") {
		return src
	}
	lines := strings.SplitAfter(src, "\n")
	var b strings.Builder
	b.Grow(len(src) + len(lines)*len(hardBreak))
	fence := ""
	for k, line := range lines {
		text := strings.TrimRight(line, "\r\n")
		switch {
		case fence != "":
			if closesFence(text, fence) {
				fence = ""
			}
		case openingFence(text) != "":
			fence = openingFence(text)
		case k+1 < len(lines) && breaksBefore(text, lines[k+1]):
			b.WriteString(text)
			b.WriteString(hardBreak)
			b.WriteString(line[len(text):])
			continue
		}
		b.WriteString(line)
	}
	return b.String()
}

func breaksBefore(text, next string) bool {
	next = strings.TrimRight(next, "\r\n")
	return strings.TrimSpace(text) != "" && strings.TrimSpace(next) != "" &&
		!strings.HasSuffix(text, hardBreak) && !strings.HasSuffix(text, `\`) && openingFence(next) == ""
}

func fenceMarker(text string) (marker, rest string) {
	s := stripContainers(text)
	if len(s) < 3 || s[0] != '`' && s[0] != '~' {
		return "", ""
	}
	n := 0
	for n < len(s) && s[n] == s[0] {
		n++
	}
	if n < 3 {
		return "", ""
	}
	return s[:n], s[n:]
}

func stripContainers(s string) string {
	for {
		t := strings.TrimLeft(s, " \t>")
		if len(t) > 1 && strings.ContainsRune("-*+", rune(t[0])) && t[1] == ' ' {
			t = t[2:]
		} else if n := orderedMarker(t); n > 0 {
			t = t[n:]
		}
		if t == s {
			return s
		}
		s = t
	}
}

func orderedMarker(s string) int {
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	if n == 0 || n+1 >= len(s) || s[n] != '.' && s[n] != ')' || s[n+1] != ' ' {
		return 0
	}
	return n + 2
}

func openingFence(text string) string {
	marker, info := fenceMarker(text)
	if marker == "" || marker[0] == '`' && strings.Contains(info, "`") {
		return ""
	}
	return marker
}

func closesFence(text, fence string) bool {
	marker, rest := fenceMarker(text)
	return marker != "" && marker[0] == fence[0] && len(marker) >= len(fence) && strings.TrimSpace(rest) == ""
}

func (v *View) codeBlock(c *ui.Context, b *markdown.CodeBlockView) {
	tk := v.tk
	code := b.Code()
	label := locales.ArcCodeBlockSourceCode()
	if b.Language != "" {
		label = locales.ArcCodeBlockRegion(b.Language)
	}
	ui.Column(c).AlignSelf(ui.Stretch).MinWidth(0).Direction(ui.LeftToRight).Background(tk.Surface).
		Border(1, tk.Border).Radius(theme.RadiusInset).Clip().Role(ui.RoleGroup).Label(label).Children(func() {
		ui.Row(c).Height(codeHeaderHeight).AlignItems(ui.Center).Padding(0, codeHeaderEnd, 0, codeHeaderStart).
			BorderWidth(0, 0, 1, 0).BorderColor(tk.BorderSubtle).Children(func() {
			ui.Text(c, strings.ToLower(b.Language)).Font(theme.FontMono).FontSize(codeLabelSize).TextColor(tk.TextMuted).
				SingleLine().Unselectable()
			ui.Spacer(c)
			copyTool(c, tk, strings.TrimSuffix(code, "\n"), locales.ArcCodeBlockCopy())
			if toolButton(c, tk, "download", locales.CodeDownload()).Clicked() {
				v.download(snippetName(b.Language), []byte(code))
			}
		})
		b.Body(c).Padding(codePadY-codeChunkTop, codePadX-codeChunkPad, 0, codePadX-codeChunkPad).FontFeatures(noLigatures...)
	})
}

func snippetName(lang string) string {
	ext, ok := snippetExtensions[strings.ToLower(lang)]
	if !ok {
		ext = plainExtension
	}
	return snippetBase + "." + ext
}

func toolButton(c *ui.Context, tk theme.Tokens, icon, label string) ui.Element {
	b := ui.ButtonBase(c).Label(label).Size(toolBox, toolBox).Shrink(0).Radius(theme.RadiusControl)
	bg, fg := tk.SurfaceMuted.Alpha(0), tk.TextSecondary
	if b.Hovered() || b.Pressed() {
		bg, fg = tk.SurfaceMuted, tk.Foreground
	}
	b.Background(b.AnimateColor("bg", bg, colorFade)).TextColor(b.AnimateColor("fg", fg, colorFade)).Children(func() {
		ui.Icon(c, icons.Get(icon)).Size(toolGlyph, toolGlyph)
	})
	return kit.Tooltip(c, tk, b, label)
}

func copyTool(c *ui.Context, tk theme.Tokens, text, label string) ui.Element {
	return kit.Tooltip(c, tk, kit.CopyIconButton(c, tk, text, label).Size(toolBox, toolBox), label)
}

func (v *View) download(name string, data []byte) {
	save := v.save
	v.s.Go(func(context.Context) {
		path, err := save(name, data)
		v.s.Post(func() { v.saved(name, path, err) })
	})
}

func (v *View) saved(name, path string, err error) {
	switch {
	case err != nil:
		log.Printf("chat: save %s: %v", name, err)
	case path != "":
		v.s.Toast(kit.Toast{Tone: kit.ToneSuccess, Title: locales.DownloadSaved(filepath.Base(path))})
	}
}

func saveWithDialog(name string, data []byte) (string, error) {
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{DefaultPath: name, CreateDirectories: true})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, data, savedFileMode); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}
