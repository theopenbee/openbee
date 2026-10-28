package chat

import (
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
	HardBreaks  = hardBreaks
	SnippetName = snippetName
)

func (v *View) SetSaver(save func(name string, data []byte) (string, error)) {
	v.save = save
}

type ImageCache = imageCache

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
