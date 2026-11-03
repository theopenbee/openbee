package chat

import (
	"image"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
)

type Sender struct {
	Kind, Name, WorkerID string
	ViaBee               bool
}

func SenderOf(m api.ChatMessage, cid string, workers map[string]api.Worker) Sender {
	d := senderDisplay(m, cid, workers)
	return Sender{Kind: d.kind, Name: d.name, WorkerID: d.workerID, ViaBee: d.viaBee}
}

var TaskTypeLabel = taskTypeLabel

func (v *View) List(cid string) *ui.ListState {
	return &v.state(cid).list
}

var (
	HardBreaks      = hardBreaks
	SnippetName     = snippetName
	CollapsedPrefix = collapsedPrefix
)

func (v *View) SetSaver(save func(name string, data []byte) (string, error)) {
	v.save = save
}

type ImageCache = imageCache

var ErrImageTooLarge = errImageTooLarge

func (v *View) OnMediaLoad(fn func(name string)) {
	load := v.images.media
	v.images.media = func(name string) ([]byte, error) {
		fn(name)
		return load(name)
	}
}

func (v *View) ImageLoadFailed() bool {
	v.loads.mu.Lock()
	defer v.loads.mu.Unlock()
	return v.loads.failed
}

func NewImageCache(media func(name string) ([]byte, error)) *ImageCache {
	return newImageCache(media, nil)
}

func (ic *imageCache) Len() int {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	return ic.order.Len()
}

func (ic *imageCache) Bytes() int64 {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	return ic.size
}

func TableExports(src string) (markdown, csv, tsv string) {
	t := parseTable(src)
	return t.markdown(), t.csv(), t.tsv()
}

var (
	FileCategory = fileCategory
	IsImage      = isImage
	SafeToOpen   = safeToOpen
)

func (v *View) SetOpener(open func(path string) error) {
	v.openPath = open
}

func (v *View) SetRevealer(reveal func(path string) error) {
	v.reveal = reveal
}

func (v *View) CachedImageBytes() int64 {
	return v.images.Bytes()
}

func (ic *imageCache) LoadPhoto(name string) (ui.ImageSource, error) {
	return ic.loadPhoto(name, fullSize)
}

func (ic *imageCache) LoadThumbnail(name string) (ui.ImageSource, error) {
	return ic.loadPhoto(name, thumbSize)
}

func ScaledPhoto(data []byte) (*image.RGBA, error) {
	return scaleImage(data, photoEdge)
}

func (ic *imageCache) SetDecoder(decode func(data []byte, svg bool, edge int) (ui.ImageSource, int64, error)) {
	ic.decode = decode
}

const (
	ThumbEdge    = thumbEdge
	InlineEdge   = inlineEdge
	MaxMediaFile = maxMediaFile
)

var ReadMediaFile = readMediaFile

var (
	MentionQuery      = mentionQuery
	MentionCandidates = mentionCandidates
	ApplyMention      = applyMention
)

func (v *View) SetFilePicker(pick func() ([]string, error)) {
	v.pick = pick
}

func (v *View) Draft(cid string) string {
	return v.state(cid).composer.draft
}

func (v *View) Attachments(cid string) []string {
	return slices.Clone(v.state(cid).composer.files)
}

type Hint = hint

const (
	HintNone    = hintNone
	HintPrompt  = hintPrompt
	HintBlocked = hintBlocked
)

var HintState = hintState

func (v *View) SetImageBudget(bytes int64) {
	v.images.mu.Lock()
	defer v.images.mu.Unlock()
	v.images.limit = bytes
}
