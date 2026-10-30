package model

import (
	"slices"
	"strings"
	"unicode"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

const previewMaxCodePoints = 120

var mediaMarkers = []string{"\x00[file] ", "[file] "}

type EventEffect struct {
	Summaries            []api.ConversationSummary
	SummariesChanged     bool
	RefetchConversations bool
	RefetchWorkers       bool
	WorkerStatus         map[string]string
}

func hasMediaMarker(s string) bool {
	return slices.ContainsFunc(mediaMarkers, func(marker string) bool {
		return strings.HasPrefix(s, marker)
	})
}

func stripMediaMarkers(content string) string {
	rest := content
	for hasMediaMarker(rest) {
		end := strings.IndexByte(rest, '\n')
		if end == -1 {
			break
		}
		rest = rest[end+1:]
	}
	return rest
}

func PreviewText(content string) string {
	text := strings.Join(strings.FieldsFunc(stripMediaMarkers(content), unicode.IsSpace), " ")
	runes := []rune(text)
	if len(runes) > previewMaxCodePoints {
		runes = runes[:previewMaxCodePoints]
	}
	return string(runes)
}

func SeqGap(last *uint64, seq uint64) bool {
	return last != nil && seq != *last+1
}

func workerIDsOf(ev api.ChatEvent) []string {
	chatWorker, _ := api.ConversationWorkerID(ev.ConversationID)
	var ids []string
	switch ev.Type {
	case "worker_status":
		ids = []string{ev.WorkerID}
	case "task":
		ids = []string{ev.WorkerID, chatWorker}
	case "message":
		handoff := ""
		if ev.Message != nil && ev.Message.Handoff != nil {
			handoff = ev.Message.Handoff.WorkerID
		}
		ids = []string{handoff, chatWorker}
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func hasUnknownWorker(ev api.ChatEvent, workers map[string]api.Worker) bool {
	return slices.ContainsFunc(workerIDsOf(ev), func(id string) bool {
		_, ok := workers[id]
		return !ok
	})
}

func activityOf(ev api.ChatEvent) api.ConversationActivity {
	if ev.State != "queued" && ev.State != "running" {
		return api.ConversationActivity{State: "idle"}
	}
	return api.ConversationActivity{State: ev.State, QueuedAhead: ev.QueuedAhead}
}

func withMessage(c api.ConversationSummary, m api.ChatMessage, seen bool) api.ConversationSummary {
	last := &api.LastMessage{
		Sender:     m.Sender,
		Kind:       m.Kind,
		Preview:    PreviewText(m.Content),
		MediaPaths: m.MediaPaths,
		TS:         m.TS,
	}
	if m.Handoff != nil {
		last.HandoffWorkerID = m.Handoff.WorkerID
	}
	c.LastMessage = last
	if !seen {
		c.Unread++
	}
	return c
}

func ApplyEvent(ev api.ChatEvent, summaries []api.ConversationSummary, workers map[string]api.Worker, active string, visible bool) EventEffect {
	eff := EventEffect{Summaries: summaries}
	eff.RefetchWorkers = workers != nil && hasUnknownWorker(ev, workers)
	switch ev.Type {
	case "message", "task":
		if summaries == nil || (ev.Type == "message" && ev.Message == nil) {
			return eff
		}
		index := slices.IndexFunc(summaries, func(c api.ConversationSummary) bool { return c.ID == ev.ConversationID })
		if index == -1 {
			eff.RefetchConversations = true
			return eff
		}
		updated := summaries[index]
		if ev.Type == "task" {
			updated.Activity = activityOf(ev)
		} else {
			if ev.Message.Kind == "reset" {
				return eff
			}
			seen := ev.Message.Sender.Type == "user" || (active != "" && active == ev.ConversationID && visible)
			updated = withMessage(updated, *ev.Message, seen)
		}
		eff.Summaries = slices.Clone(summaries)
		eff.Summaries[index] = updated
		eff.SummariesChanged = true
	case "worker_status":
		if w, ok := workers[ev.WorkerID]; ok && w.Status != ev.Status {
			eff.WorkerStatus = map[string]string{ev.WorkerID: ev.Status}
		}
	}
	return eff
}
