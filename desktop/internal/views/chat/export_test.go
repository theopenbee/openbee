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
