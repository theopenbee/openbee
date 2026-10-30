package model

import (
	"path"
	"slices"
	"strings"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

type PreviewKind int

const (
	PreviewPlain PreviewKind = iota
	PreviewYou
	PreviewHandoff
	PreviewDescription
	PreviewEmpty
)

type PreviewMedia int

const (
	MediaNone PreviewMedia = iota
	MediaImage
	MediaFile
)

const ImageExtensions = "jpg jpeg png gif webp svg bmp"

type Preview struct {
	Kind          PreviewKind
	Text          string
	Media         PreviewMedia
	WorkerName    string
	WorkerDeleted bool
}

func isImageName(name string) bool {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	return slices.Contains(strings.Fields(ImageExtensions), ext)
}

func previewMedia(last *api.LastMessage) PreviewMedia {
	switch {
	case last.Preview != "" || len(last.MediaPaths) == 0:
		return MediaNone
	case slices.ContainsFunc(last.MediaPaths, func(p string) bool { return !isImageName(p) }):
		return MediaFile
	}
	return MediaImage
}

func compareNames(a, b string) int {
	if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
		return c
	}
	return strings.Compare(b, a)
}

func workerName(c api.ConversationSummary, workers map[string]api.Worker) string {
	return workers[c.WorkerID].Name
}

func SortConversations(cs []api.ConversationSummary, workers map[string]api.Worker) []api.ConversationSummary {
	out := slices.Clone(cs)
	slices.SortStableFunc(out, func(a, b api.ConversationSummary) int {
		aBee, bBee := a.ID == api.BeeConversation, b.ID == api.BeeConversation
		switch {
		case aBee && bBee:
			return 0
		case aBee:
			return -1
		case bBee:
			return 1
		}
		switch {
		case a.LastMessage != nil && b.LastMessage != nil:
			switch {
			case b.LastMessage.TS > a.LastMessage.TS:
				return 1
			case b.LastMessage.TS < a.LastMessage.TS:
				return -1
			}
			return 0
		case a.LastMessage != nil:
			return -1
		case b.LastMessage != nil:
			return 1
		}
		return compareNames(workerName(a, workers), workerName(b, workers))
	})
	return out
}

func VisibleConversations(cs []api.ConversationSummary, workers map[string]api.Worker) []api.ConversationSummary {
	out := make([]api.ConversationSummary, 0, len(cs))
	for _, c := range cs {
		if c.ID == api.BeeConversation {
			out = append(out, c)
			continue
		}
		if _, ok := workers[c.WorkerID]; ok {
			out = append(out, c)
		}
	}
	return out
}

func FilterConversations(cs []api.ConversationSummary, workers map[string]api.Worker, query string) []api.ConversationSummary {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return cs
	}
	out := make([]api.ConversationSummary, 0, len(cs))
	for _, c := range cs {
		if c.ID == api.BeeConversation {
			out = append(out, c)
			continue
		}
		w, ok := workers[c.WorkerID]
		if !ok {
			continue
		}
		if strings.Contains(strings.ToLower(w.Name), needle) || strings.Contains(strings.ToLower(w.Description), needle) {
			out = append(out, c)
		}
	}
	return out
}

func ConversationPreview(cs api.ConversationSummary, workers map[string]api.Worker) Preview {
	last := cs.LastMessage
	if last == nil {
		if w, ok := workers[cs.WorkerID]; ok && w.Description != "" {
			return Preview{Kind: PreviewDescription, Text: w.Description}
		}
		return Preview{Kind: PreviewEmpty}
	}
	if last.Kind == "handoff" && last.HandoffWorkerID != "" {
		w, ok := workers[last.HandoffWorkerID]
		if !ok {
			return Preview{Kind: PreviewHandoff, WorkerDeleted: true}
		}
		return Preview{Kind: PreviewHandoff, WorkerName: w.Name}
	}
	if last.Sender.Type == "user" {
		return Preview{Kind: PreviewYou, Text: last.Preview, Media: previewMedia(last)}
	}
	return Preview{Kind: PreviewPlain, Text: last.Preview, Media: previewMedia(last)}
}
