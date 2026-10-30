package model

import (
	"reflect"
	"strings"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func eventMessage(content string, mutate ...func(*api.ChatMessage)) api.ChatMessage {
	m := api.ChatMessage{ID: "m1", Sender: api.ChatSender{Type: "worker", ID: "w1"}, Kind: "message", Content: content, TS: 100}
	for _, f := range mutate {
		f(&m)
	}
	return m
}

func bySender(sender api.ChatSender) func(*api.ChatMessage) {
	return func(m *api.ChatMessage) { m.Sender = sender }
}

func byKind(kind string) func(*api.ChatMessage) {
	return func(m *api.ChatMessage) { m.Kind = kind }
}

func byTS(ts int64) func(*api.ChatMessage) {
	return func(m *api.ChatMessage) { m.TS = ts }
}

func byHandoff(workerID string) func(*api.ChatMessage) {
	return func(m *api.ChatMessage) {
		m.Handoff = &api.HandoffInfo{WorkerID: workerID, TaskID: "t1", TaskType: "immediate", ConversationID: "worker:" + workerID}
	}
}

func messageEvent(cid string, m api.ChatMessage, seq uint64) api.ChatEvent {
	return api.ChatEvent{Type: "message", Seq: seq, ConversationID: cid, Message: &m}
}

func taskEvent(state string, queuedAhead int) api.ChatEvent {
	return api.ChatEvent{Type: "task", Seq: 1, ConversationID: "worker:w1", TaskID: "t1", WorkerID: "w1", State: state, QueuedAhead: queuedAhead}
}

func statusEvent(workerID, status string) api.ChatEvent {
	return api.ChatEvent{Type: "worker_status", Seq: 1, WorkerID: workerID, Status: status}
}

func summaryWith(id string, mutate func(*api.ConversationSummary)) api.ConversationSummary {
	s := summary(id)
	mutate(&s)
	return s
}

func withWorkers(workers ...api.Worker) map[string]api.Worker {
	out := make(map[string]api.Worker, len(workers))
	for _, w := range workers {
		out[w.ID] = w
	}
	return out
}

func statusWorker(id, status string) api.Worker {
	w := worker(id, id, "")
	w.Status = status
	return w
}

func sameSlice(a, b []api.ConversationSummary) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

func TestApplyEventMessage(t *testing.T) {
	t.Run("updates preview and unread for an inactive conversation", func(t *testing.T) {
		list := []api.ConversationSummary{summary("bee"), summaryWith("worker:w1", func(s *api.ConversationSummary) { s.Unread = 2 })}

		eff := ApplyEvent(messageEvent("worker:w1", eventMessage("  hello\n there ", byTS(150)), 1), list, nil, "", true)

		want := &api.LastMessage{Sender: api.ChatSender{Type: "worker", ID: "w1"}, Kind: "message", Preview: "hello there", TS: 150}
		if !eff.SummariesChanged {
			t.Fatal("SummariesChanged = false")
		}
		if !reflect.DeepEqual(eff.Summaries[1].LastMessage, want) {
			t.Fatalf("last message = %+v, want %+v", eff.Summaries[1].LastMessage, want)
		}
		if eff.Summaries[1].Unread != 3 {
			t.Fatalf("unread = %d, want 3", eff.Summaries[1].Unread)
		}
		if !reflect.DeepEqual(eff.Summaries[0], list[0]) {
			t.Fatal("untouched conversation changed")
		}
		if list[1].Unread != 2 || list[1].LastMessage != nil {
			t.Fatalf("input changed: %+v", list[1])
		}
	})

	t.Run("does not bump unread for the active visible conversation", func(t *testing.T) {
		list := []api.ConversationSummary{summary("bee"), summary("worker:w1")}

		eff := ApplyEvent(messageEvent("worker:w1", eventMessage("hi"), 1), list, nil, "worker:w1", true)

		if eff.Summaries[1].Unread != 0 {
			t.Fatalf("unread = %d, want 0", eff.Summaries[1].Unread)
		}
		if eff.Summaries[1].LastMessage == nil || eff.Summaries[1].LastMessage.Preview != "hi" {
			t.Fatalf("last message = %+v", eff.Summaries[1].LastMessage)
		}
	})

	t.Run("bumps unread for the active conversation while the window is hidden", func(t *testing.T) {
		eff := ApplyEvent(messageEvent("worker:w1", eventMessage("hi"), 1), []api.ConversationSummary{summary("worker:w1")}, nil, "worker:w1", false)

		if eff.Summaries[0].Unread != 1 {
			t.Fatalf("unread = %d, want 1", eff.Summaries[0].Unread)
		}
	})

	t.Run("bumps unread when another conversation is active", func(t *testing.T) {
		list := []api.ConversationSummary{summary("bee"), summary("worker:w1")}

		eff := ApplyEvent(messageEvent("worker:w1", eventMessage("hi"), 1), list, nil, "bee", true)

		if eff.Summaries[1].Unread != 1 {
			t.Fatalf("unread = %d, want 1", eff.Summaries[1].Unread)
		}
	})

	t.Run("an own message does not bump unread", func(t *testing.T) {
		list := []api.ConversationSummary{summaryWith("worker:w1", func(s *api.ConversationSummary) { s.Unread = 1 })}

		eff := ApplyEvent(messageEvent("worker:w1", eventMessage("mine", bySender(api.ChatSender{Type: "user"})), 1), list, nil, "", true)

		got := eff.Summaries[0]
		if got.Unread != 1 {
			t.Fatalf("unread = %d, want 1", got.Unread)
		}
		if got.LastMessage.Sender != (api.ChatSender{Type: "user"}) || got.LastMessage.Preview != "mine" {
			t.Fatalf("last message = %+v", got.LastMessage)
		}
	})

	t.Run("keeps the message kind in the last message", func(t *testing.T) {
		list := []api.ConversationSummary{summary("bee")}

		eff := ApplyEvent(messageEvent("bee", eventMessage("delegated", byKind("handoff"), bySender(api.ChatSender{Type: "bee"})), 1), list, nil, "", true)

		if eff.Summaries[0].LastMessage.Kind != "handoff" || eff.Summaries[0].Unread != 1 {
			t.Fatalf("got %+v", eff.Summaries[0])
		}
	})

	t.Run("keeps the media paths of an attachment", func(t *testing.T) {
		list := []api.ConversationSummary{summary("worker:w1")}
		attachment := eventMessage("", func(m *api.ChatMessage) { m.MediaPaths = []string{"m1_chart.png"} })

		eff := ApplyEvent(messageEvent("worker:w1", attachment, 1), list, nil, "", true)

		want := &api.LastMessage{Sender: api.ChatSender{Type: "worker", ID: "w1"}, Kind: "message", MediaPaths: []string{"m1_chart.png"}, TS: 100}
		if !reflect.DeepEqual(eff.Summaries[0].LastMessage, want) {
			t.Fatalf("last message = %+v, want %+v", eff.Summaries[0].LastMessage, want)
		}
	})

	t.Run("records the handoff worker and drops it on the next plain message", func(t *testing.T) {
		list := []api.ConversationSummary{summary("bee")}
		bee := bySender(api.ChatSender{Type: "bee"})
		handoff := eventMessage("write the report", byKind("handoff"), bee, byTS(200), byHandoff("w2"))

		handedOff := ApplyEvent(messageEvent("bee", handoff, 1), list, nil, "", true)
		replied := ApplyEvent(messageEvent("bee", eventMessage("on it", bee, byTS(300)), 2), handedOff.Summaries, nil, "", true)

		want := &api.LastMessage{Sender: api.ChatSender{Type: "bee"}, Kind: "handoff", Preview: "write the report", HandoffWorkerID: "w2", TS: 200}
		if !reflect.DeepEqual(handedOff.Summaries[0].LastMessage, want) {
			t.Fatalf("handed off = %+v, want %+v", handedOff.Summaries[0].LastMessage, want)
		}
		if replied.Summaries[0].LastMessage.HandoffWorkerID != "" || replied.Summaries[0].LastMessage.Preview != "on it" {
			t.Fatalf("replied = %+v", replied.Summaries[0].LastMessage)
		}
	})

	t.Run("leaves the preview and unread alone for a new conversation divider", func(t *testing.T) {
		list := []api.ConversationSummary{summary("bee"), summaryWith("worker:w1", func(s *api.ConversationSummary) { s.Unread = 1 })}
		divider := eventMessage("", bySender(api.ChatSender{Type: "system"}), byKind("reset"))

		eff := ApplyEvent(messageEvent("worker:w1", divider, 1), list, nil, "", true)

		if eff.SummariesChanged || eff.RefetchConversations || !sameSlice(eff.Summaries, list) {
			t.Fatalf("divider changed the list: %+v", eff)
		}
	})

	t.Run("refetches the list for an unknown conversation", func(t *testing.T) {
		list := []api.ConversationSummary{summary("bee")}

		eff := ApplyEvent(messageEvent("worker:gone", eventMessage("hi"), 1), list, nil, "", true)

		if !eff.RefetchConversations || eff.SummariesChanged || !sameSlice(eff.Summaries, list) {
			t.Fatalf("got %+v", eff)
		}
	})

	t.Run("a message event without a message changes nothing", func(t *testing.T) {
		list := []api.ConversationSummary{summary("bee")}

		eff := ApplyEvent(api.ChatEvent{Type: "message", Seq: 1, ConversationID: "bee"}, list, nil, "", true)

		if eff.SummariesChanged || eff.RefetchConversations || eff.RefetchWorkers || len(eff.WorkerStatus) != 0 {
			t.Fatalf("got %+v", eff)
		}
	})
}

func TestApplyEventTask(t *testing.T) {
	t.Run("queued then running then completed toggles activity", func(t *testing.T) {
		list := []api.ConversationSummary{summary("bee"), summary("worker:w1")}

		queued := ApplyEvent(taskEvent("queued", 2), list, nil, "", true)
		if queued.Summaries[1].Activity != (api.ConversationActivity{State: "queued", QueuedAhead: 2}) {
			t.Fatalf("queued activity = %+v", queued.Summaries[1].Activity)
		}

		running := ApplyEvent(taskEvent("running", 0), queued.Summaries, nil, "", true)
		if running.Summaries[1].Activity != (api.ConversationActivity{State: "running"}) {
			t.Fatalf("running activity = %+v", running.Summaries[1].Activity)
		}

		completed := ApplyEvent(taskEvent("completed", 0), running.Summaries, nil, "", true)
		if completed.Summaries[1].Activity != (api.ConversationActivity{State: "idle"}) {
			t.Fatalf("completed activity = %+v", completed.Summaries[1].Activity)
		}
		if !completed.SummariesChanged || !reflect.DeepEqual(completed.Summaries[0], list[0]) {
			t.Fatalf("got %+v", completed)
		}
	})

	for _, state := range []string{"failed", "cancelled"} {
		t.Run("task "+state+" sets activity to idle", func(t *testing.T) {
			list := []api.ConversationSummary{summaryWith("worker:w1", func(s *api.ConversationSummary) { s.Activity.State = "running" })}

			eff := ApplyEvent(taskEvent(state, 0), list, nil, "", true)

			if eff.Summaries[0].Activity != (api.ConversationActivity{State: "idle"}) {
				t.Fatalf("activity = %+v", eff.Summaries[0].Activity)
			}
		})
	}

	t.Run("an unknown conversation refetches the list", func(t *testing.T) {
		list := []api.ConversationSummary{summary("bee")}

		eff := ApplyEvent(taskEvent("running", 0), list, nil, "", true)

		if !eff.RefetchConversations || eff.SummariesChanged || !sameSlice(eff.Summaries, list) {
			t.Fatalf("got %+v", eff)
		}
	})
}

func TestApplyEventWorkerStatus(t *testing.T) {
	t.Run("leaves the list unchanged", func(t *testing.T) {
		list := []api.ConversationSummary{summary("worker:w1")}

		eff := ApplyEvent(statusEvent("w1", "working"), list, withWorkers(statusWorker("w1", "idle")), "", true)

		if eff.SummariesChanged || eff.RefetchConversations || !sameSlice(eff.Summaries, list) {
			t.Fatalf("got %+v", eff)
		}
	})

	t.Run("patches only the matching worker", func(t *testing.T) {
		workers := withWorkers(statusWorker("w1", "idle"), statusWorker("w2", "idle"))

		eff := ApplyEvent(statusEvent("w2", "working"), nil, workers, "", true)

		want := map[string]string{"w2": "working"}
		if !reflect.DeepEqual(eff.WorkerStatus, want) {
			t.Fatalf("got %v, want %v", eff.WorkerStatus, want)
		}
		if workers["w2"].Status != "idle" {
			t.Fatal("input workers changed")
		}
	})

	t.Run("reports nothing when the status is unchanged", func(t *testing.T) {
		eff := ApplyEvent(statusEvent("w1", "working"), nil, withWorkers(statusWorker("w1", "working")), "", true)

		if len(eff.WorkerStatus) != 0 || eff.RefetchWorkers {
			t.Fatalf("got %+v", eff)
		}
	})

	t.Run("reports nothing for an unknown worker but refetches the workers", func(t *testing.T) {
		eff := ApplyEvent(statusEvent("gone", "error"), nil, withWorkers(statusWorker("w1", "idle")), "", true)

		if len(eff.WorkerStatus) != 0 || !eff.RefetchWorkers {
			t.Fatalf("got %+v", eff)
		}
	})

	t.Run("reports nothing when no workers are loaded", func(t *testing.T) {
		eff := ApplyEvent(statusEvent("w1", "error"), nil, nil, "", true)

		if len(eff.WorkerStatus) != 0 || eff.RefetchWorkers || eff.RefetchConversations || eff.SummariesChanged {
			t.Fatalf("got %+v", eff)
		}
	})

	t.Run("other event types carry no worker status", func(t *testing.T) {
		list := []api.ConversationSummary{summary("worker:w1")}

		eff := ApplyEvent(taskEvent("running", 0), list, withWorkers(statusWorker("w1", "idle")), "", true)

		if len(eff.WorkerStatus) != 0 {
			t.Fatalf("got %v", eff.WorkerStatus)
		}
	})
}

func TestApplyEventRefetchWorkers(t *testing.T) {
	known := withWorkers(statusWorker("w1", "working"), statusWorker("w2", "idle"))
	list := []api.ConversationSummary{summary("bee"), summary("worker:w1"), summary("worker:new")}
	bee := bySender(api.ChatSender{Type: "bee"})
	handoffTo := func(id string) api.ChatEvent {
		return messageEvent("bee", eventMessage("do it", byKind("handoff"), bee, byHandoff(id)), 1)
	}
	newTask := taskEvent("queued", 0)
	newTask.WorkerID = "new"
	newTask.ConversationID = "worker:new"

	refetching := []struct {
		name string
		ev   api.ChatEvent
	}{
		{"a handoff to a new worker", handoffTo("new")},
		{"a message in a new worker chat", messageEvent("worker:new", eventMessage("hi"), 1)},
		{"a task of a new worker", newTask},
		{"the status of a new worker", statusEvent("new", "working")},
	}
	for _, tc := range refetching {
		t.Run("refetches the workers for "+tc.name, func(t *testing.T) {
			if eff := ApplyEvent(tc.ev, list, known, "", true); !eff.RefetchWorkers {
				t.Fatalf("got %+v", eff)
			}
		})
	}

	t.Run("leaves workers that have not loaded yet alone", func(t *testing.T) {
		if eff := ApplyEvent(statusEvent("new", "working"), list, nil, "", true); eff.RefetchWorkers {
			t.Fatalf("got %+v", eff)
		}
	})

	t.Run("refetches against a loaded but empty worker list", func(t *testing.T) {
		if eff := ApplyEvent(statusEvent("new", "working"), list, map[string]api.Worker{}, "", true); !eff.RefetchWorkers {
			t.Fatalf("got %+v", eff)
		}
	})

	t.Run("keeps the workers fresh when every named worker is cached", func(t *testing.T) {
		events := []api.ChatEvent{
			handoffTo("w2"),
			messageEvent("worker:w1", eventMessage("hi"), 1),
			taskEvent("running", 0),
			statusEvent("w2", "working"),
			messageEvent("bee", eventMessage("x", bee), 1),
		}
		for _, ev := range events {
			if eff := ApplyEvent(ev, list, known, "", true); eff.RefetchWorkers {
				t.Fatalf("%s event refetched the workers", ev.Type)
			}
		}
	})

	t.Run("a message refetches the workers and updates the list together", func(t *testing.T) {
		eff := ApplyEvent(messageEvent("worker:new", eventMessage("hi"), 1), list, known, "", true)

		if !eff.RefetchWorkers || !eff.SummariesChanged || eff.Summaries[2].Unread != 1 {
			t.Fatalf("got %+v", eff)
		}
	})
}

func TestApplyEventListNotLoaded(t *testing.T) {
	events := []api.ChatEvent{
		messageEvent("worker:w1", eventMessage("hi"), 1),
		taskEvent("running", 0),
	}
	for _, ev := range events {
		eff := ApplyEvent(ev, nil, withWorkers(statusWorker("w1", "idle")), "", true)

		if eff.SummariesChanged || eff.RefetchConversations || eff.Summaries != nil {
			t.Fatalf("%s event with no list loaded produced %+v", ev.Type, eff)
		}
	}
}

func TestApplyEventDoesNotMutateTheInput(t *testing.T) {
	build := func() []api.ConversationSummary {
		return []api.ConversationSummary{summary("bee"), summaryWith("worker:w1", func(s *api.ConversationSummary) { s.Unread = 4 })}
	}
	list := build()

	ApplyEvent(messageEvent("worker:w1", eventMessage("hi"), 1), list, nil, "", true)
	ApplyEvent(taskEvent("running", 0), list, nil, "", true)

	if !reflect.DeepEqual(list, build()) {
		t.Fatalf("input changed to %+v", list)
	}
}

func TestSeqGap(t *testing.T) {
	seq := func(n uint64) *uint64 { return &n }

	tests := []struct {
		name string
		last *uint64
		seq  uint64
		want bool
	}{
		{"first event", nil, 1, false},
		{"first event after a late connect", nil, 40, false},
		{"next event", seq(1), 2, false},
		{"skipped event", seq(5), 7, true},
		{"repeated event", seq(5), 5, true},
		{"seq reset after a server restart", seq(5), 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SeqGap(tc.last, tc.seq); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPreviewTextStripsFileMarkers(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"one marker", "\x00[file] a.png\nhello   world", "hello world"},
		{"nul markers and whitespace runs", "\x00[file] a.png\n\x00[file] b.pdf\nlook\n\n  at   this\tpic", "look at this pic"},
		{"plain markers", "[file] a.png\n[file] b.pdf\ntwo files", "two files"},
		{"mixed markers", "\x00[file] a.png\n[file] b.pdf\nmixed", "mixed"},
		{"surrounding whitespace", "  hello\n there ", "hello there"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PreviewText(tc.content); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPreviewTextCutsAt120CodePoints(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"cjk", strings.Repeat("你", 130), strings.Repeat("你", 120)},
		{"han", strings.Repeat("字", 130), strings.Repeat("字", 120)},
		{"emoji", strings.Repeat("😀", 130), strings.Repeat("😀", 120)},
		{"exactly 120", strings.Repeat("a", 120), strings.Repeat("a", 120)},
		{"121", strings.Repeat("a", 121), strings.Repeat("a", 120)},
		{"short", "short", "short"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PreviewText(tc.content)
			if got != tc.want {
				t.Fatalf("got %d runes, want %d", len([]rune(got)), len([]rune(tc.want)))
			}
		})
	}
}

func TestPreviewTextKeepsMarkerLookalikes(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"marker in the middle of a line", "see [file] a.png\nnow", "see [file] a.png now"},
		{"unterminated marker line", "\x00[file] unterminated", "\x00[file] unterminated"},
		{"marker without a space", "[file]nospace\nhello", "[file]nospace hello"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PreviewText(tc.content); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPreviewTextBlank(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"empty", "", ""},
		{"media only", "\x00[file] a.png\n ", ""},
		{"unicode spaces only", " \n\t 　  ", ""},
		{"unicode spaces collapse", "a 　b\u0085c", "a b c"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PreviewText(tc.content); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWorkerIDsOf(t *testing.T) {
	bee := bySender(api.ChatSender{Type: "bee"})
	handoff := messageEvent("bee", eventMessage("x", bee, byKind("handoff"), byHandoff("w2")), 1)

	tests := []struct {
		name string
		ev   api.ChatEvent
		want []string
	}{
		{"handoff target", handoff, []string{"w2"}},
		{"worker chat", messageEvent("worker:w1", eventMessage("x"), 1), []string{"w1"}},
		{"handoff target and chat worker", messageEvent("worker:w1", eventMessage("x", byHandoff("w2")), 1), []string{"w2", "w1"}},
		{"task", taskEvent("running", 0), []string{"w1"}},
		{"worker status", statusEvent("w3", "idle"), []string{"w3"}},
		{"bee message without handoff", messageEvent("bee", eventMessage("x", bee), 1), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := workerIDsOf(tc.ev)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHasUnknownWorker(t *testing.T) {
	known := withWorkers(statusWorker("w1", "idle"))
	bee := bySender(api.ChatSender{Type: "bee"})

	if hasUnknownWorker(messageEvent("worker:w1", eventMessage("x"), 1), known) {
		t.Error("a known chat worker was reported unknown")
	}
	if !hasUnknownWorker(statusEvent("w2", "idle"), known) {
		t.Error("an unknown status worker was reported known")
	}
	if hasUnknownWorker(messageEvent("bee", eventMessage("x", bee), 1), map[string]api.Worker{}) {
		t.Error("an event naming no worker was reported unknown")
	}
}
