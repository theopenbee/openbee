package chat

import (
	"log"
	"path"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	categoryImage    = "image"
	categoryAudio    = "audio"
	categoryVideo    = "video"
	categoryDocument = "document"
	categoryCode     = "code"
	categoryArchive  = "archive"
	categoryOther    = "other"

	attachGap         = 8
	attachBelow       = 8
	attachUserShare   = 85
	attachUserMax     = 512
	attachWorkerMax   = 448
	thumbMaxHeight    = 240
	thumbHoverOpacity = 0.9
	thumbPlaceW       = 160
	thumbPlaceH       = 120
	chipMinHeight     = 44
	chipPadX          = 12
	chipGap           = 8
	chipGlyph         = 16
)

var categoryExtensions = []struct{ category, extensions string }{
	{categoryImage, model.ImageExtensions},
	{categoryAudio, "mp3 wav ogg aac flac m4a wma"},
	{categoryVideo, "mp4 mov avi mkv webm flv wmv"},
	{categoryDocument, "pdf doc docx txt md csv xls xlsx ppt pptx rtf"},
	{categoryCode, "js ts jsx tsx py go java c cpp h cs rb php html css json xml yaml yml sh sql"},
	{categoryArchive, "zip tar gz rar 7z bz2"},
}

var categoryIcons = map[string]string{
	categoryImage:    "file-image",
	categoryAudio:    "file-audio",
	categoryVideo:    "file-video",
	categoryDocument: "file-text",
	categoryCode:     "file-code",
	categoryArchive:  "file-archive",
	categoryOther:    "file",
}

var openableCategories = map[string]bool{
	categoryImage:    true,
	categoryAudio:    true,
	categoryVideo:    true,
	categoryDocument: true,
	categoryArchive:  true,
}

var revealOnly = map[string]bool{"svg": true}

var extensionCategories = func() map[string]string {
	m := map[string]string{}
	for _, c := range categoryExtensions {
		for _, ext := range strings.Fields(c.extensions) {
			m[ext] = c.category
		}
	}
	return m
}()

func fileCategory(name string) string {
	if category, ok := extensionCategories[extension(name)]; ok {
		return category
	}
	return categoryOther
}

func extension(name string) string {
	return strings.ToLower(strings.TrimPrefix(path.Ext(mediaBase(name)), "."))
}

func safeToOpen(name string) bool {
	base := strings.TrimRight(mediaBase(name), ". ")
	return openableCategories[fileCategory(base)] && !revealOnly[extension(base)]
}

func isImage(name string) bool {
	return fileCategory(name) == categoryImage
}

func mediaBase(mediaPath string) string {
	if i := strings.LastIndexAny(mediaPath, `/\`); i >= 0 {
		return mediaPath[i+1:]
	}
	return mediaPath
}

type photoLoads struct {
	mu      sync.Mutex
	loading map[string]bool
	failed  map[string]bool
}

func newPhotoLoads() *photoLoads {
	return &photoLoads{loading: map[string]bool{}, failed: map[string]bool{}}
}

func (l *photoLoads) begin(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.loading[key] || l.failed[key] {
		return false
	}
	l.loading[key] = true
	return true
}

func (l *photoLoads) finish(key string, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.loading, key)
	if err != nil {
		l.failed[key] = true
		log.Printf("chat: load attachment %s: %v", key, err)
	}
}

func (l *photoLoads) hasFailed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.failed[key]
}

func (l *photoLoads) retryFailed() {
	l.mu.Lock()
	defer l.mu.Unlock()
	clear(l.failed)
}

func (v *View) photo(c *ui.Context, name string, size photoSize) (ui.ImageSource, bool) {
	key := size.key(name)
	if src, _, ok := v.images.get(key); ok {
		return src, true
	}
	if v.photos.begin(key) {
		images, loads, services := v.images, v.photos, c.Services()
		go func() {
			_, err := images.loadPhoto(name, size)
			loads.finish(key, err)
			services.Invalidate()
		}()
	}
	return nil, false
}

func sourceSize(src ui.ImageSource) (w, h float32) {
	switch s := src.(type) {
	case *ui.Bitmap:
		iw, ih := s.Size()
		return float32(iw), float32(ih)
	case *ui.SVG:
		return s.Size()
	}
	return 0, 0
}

func (v *View) attachments(c *ui.Context, tk theme.Tokens, m model.Message, mine bool) {
	names := attachmentNames(m.MediaPaths)
	if len(names) == 0 {
		return
	}
	align, share, limit := ui.Start, float32(100), float32(attachWorkerMax)
	if mine {
		align, share, limit = ui.End, attachUserShare, attachUserMax
	}
	below := float32(0)
	if hasText(m.Content) {
		below = attachBelow
	}
	list := func() {
		ui.Column(c).AlignItems(align).Gap(attachGap).MinWidth(0).MaxWidthPercent(100).Children(func() {
			for _, name := range names {
				v.attachment(c, tk, name)
			}
		})
	}
	outer := ui.Column(c).MinWidth(0).Margin(0, 0, below, 0)
	if w, _ := c.Size(); w < theme.BreakComposer {
		outer.MaxWidthPercent(100).Children(list)
		return
	}
	outer.MaxWidthPercent(share).Children(func() {
		ui.Column(c).MinWidth(0).MaxWidth(limit).AlignItems(align).Children(list)
	})
}

func attachmentNames(mediaPaths []string) []string {
	names := make([]string, 0, len(mediaPaths))
	for _, p := range mediaPaths {
		if name := mediaBase(p); name != "" && name != "." && name != ".." {
			names = append(names, name)
		}
	}
	return names
}

func (v *View) attachment(c *ui.Context, tk theme.Tokens, name string) {
	if !isImage(name) || v.photos.hasFailed(thumbSize.key(name)) {
		v.fileChip(c, tk, name)
		return
	}
	src, ok := v.photo(c, name, thumbSize)
	if !ok {
		kit.SkeletonBar(c, tk, thumbPlaceW, thumbPlaceH).Radius(theme.RadiusInset)
		return
	}
	v.thumbnail(c, tk, name, src)
}

func (v *View) thumbnail(c *ui.Context, tk theme.Tokens, name string, src ui.ImageSource) {
	iw, ih := sourceSize(src)
	b := ui.ButtonBase(c).Label(locales.ChatViewImage(name)).Radius(theme.RadiusInset).Border(1, tk.Border).
		Background(tk.Surface).Clip().MinWidth(0).MaxWidthPercent(100).Cursor(ui.CursorPointer)
	shown := float32(1)
	if b.Hovered() {
		shown = thumbHoverOpacity
	}
	b.Opacity(b.AnimateTo("hover", shown, colorFade)).OnClick(func() { v.openLightbox(name) }).Children(func() {
		img := ui.Image(c, src).Fit(ui.Contain)
		if iw > 0 && ih > 0 {
			h := min(ih, thumbMaxHeight)
			img.Width(iw * h / ih).MaxWidthPercent(100).AspectRatio(iw / ih)
		}
	})
}

func (v *View) fileChip(c *ui.Context, tk theme.Tokens, name string) {
	busy := v.opening[name]
	b := ui.ButtonBase(c).Label(name).MinHeight(chipMinHeight).Padding(0, chipPadX).Gap(chipGap).AlignItems(ui.Center).
		Radius(theme.RadiusControl).Border(1, tk.Border).MinWidth(0).MaxWidthPercent(100).Cursor(ui.CursorPointer)
	bg := tk.Surface
	if b.Hovered() || b.Pressed() {
		bg = tk.SurfaceMuted
	}
	b.Background(b.AnimateColor("bg", bg, colorFade)).TextColor(tk.Foreground).FontSize(bodySize).
		OnClick(func() { v.openMedia(name) }).Children(func() {
		ui.Icon(c, icons.Get(categoryIcons[fileCategory(name)])).Size(chipGlyph, chipGlyph).Shrink(0).TextColor(tk.TextSecondary)
		ui.Text(c, name).SingleLine().MinWidth(0)
		if busy {
			kit.Spinner(c, tk, chipGlyph)
			return
		}
		ui.Icon(c, icons.Get("external-link")).Size(chipGlyph, chipGlyph).Shrink(0).TextColor(tk.TextMuted)
	})
}

func (v *View) openMedia(name string) {
	if v.opening[name] {
		return
	}
	v.opening[name] = true
	gen := v.mediaGen
	v.s.MediaFile(name, func(localPath string, err error) {
		if v.mediaGen != gen {
			return
		}
		delete(v.opening, name)
		if err == nil {
			err = v.launch(localPath)
		}
		if err != nil {
			log.Printf("chat: open attachment %s: %v", name, err)
			v.s.Toast(kit.Toast{Tone: kit.ToneDanger, Title: locales.FileOpenFailed()})
		}
	})
}

func (v *View) launch(localPath string) error {
	if safeToOpen(localPath) {
		return v.openPath(localPath)
	}
	return v.reveal(localPath)
}
