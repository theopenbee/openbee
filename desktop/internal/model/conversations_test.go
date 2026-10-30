package model

import (
	"reflect"
	"strings"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func summary(id string) api.ConversationSummary {
	s := api.ConversationSummary{ID: id, Kind: "worker", Activity: api.ConversationActivity{State: "idle"}}
	if id == "bee" {
		s.Kind = "bee"
	} else {
		s.WorkerID = strings.TrimPrefix(id, "worker:")
	}
	return s
}

func summaryAt(id string, ts int64) api.ConversationSummary {
	s := summary(id)
	s.LastMessage = &api.LastMessage{Sender: api.ChatSender{Type: "user"}, Kind: "message", Preview: "hi", TS: ts}
	return s
}

func worker(id, name, description string) api.Worker {
	return api.Worker{ID: id, Name: name, Description: description, Engine: "claude", Status: "idle"}
}

func convIDs(cs []api.ConversationSummary) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}

func TestSortConversations(t *testing.T) {
	t.Run("puts bee first then recent then by name", func(t *testing.T) {
		list := []api.ConversationSummary{
			summary("worker:w-zed"),
			summaryAt("worker:w-old", 10),
			summary("bee"),
			summary("worker:w-alpha"),
			summaryAt("worker:w-new", 20),
		}
		workers := map[string]api.Worker{
			"w-zed":   worker("w-zed", "Zed", ""),
			"w-old":   worker("w-old", "Old", ""),
			"w-alpha": worker("w-alpha", "Alpha", ""),
			"w-new":   worker("w-new", "New", ""),
		}

		got := convIDs(SortConversations(list, workers))

		want := []string{"bee", "worker:w-new", "worker:w-old", "worker:w-alpha", "worker:w-zed"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("puts bee first even when a worker has a newer message", func(t *testing.T) {
		got := convIDs(SortConversations([]api.ConversationSummary{summaryAt("worker:w1", 99), summaryAt("bee", 1)}, nil))

		want := []string{"bee", "worker:w1"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("does not mutate the input list", func(t *testing.T) {
		list := []api.ConversationSummary{summary("worker:w1"), summary("bee")}

		SortConversations(list, nil)

		want := []string{"worker:w1", "bee"}
		if got := convIDs(list); !reflect.DeepEqual(got, want) {
			t.Fatalf("input became %v, want %v", got, want)
		}
	})

	t.Run("orders names without regard to case", func(t *testing.T) {
		list := []api.ConversationSummary{summary("worker:b"), summary("worker:a"), summary("worker:c")}
		workers := map[string]api.Worker{
			"a": worker("a", "alpha", ""),
			"b": worker("b", "Bravo", ""),
			"c": worker("c", "charlie", ""),
		}

		got := convIDs(SortConversations(list, workers))

		want := []string{"worker:a", "worker:b", "worker:c"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("keeps the input order for equal keys", func(t *testing.T) {
		list := []api.ConversationSummary{summaryAt("worker:b", 5), summaryAt("worker:a", 5)}

		got := convIDs(SortConversations(list, nil))

		want := []string{"worker:b", "worker:a"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
}

func TestVisibleConversations(t *testing.T) {
	workers := map[string]api.Worker{"w1": worker("w1", "Researcher", "")}
	list := []api.ConversationSummary{
		summary("worker:w1"),
		summary("worker:gone"),
		summary("bee"),
		{ID: "worker:", Kind: "worker"},
	}

	got := convIDs(VisibleConversations(list, workers))

	want := []string{"worker:w1", "bee"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFilterConversations(t *testing.T) {
	workers := map[string]api.Worker{
		"w1": worker("w1", "Researcher", "Finds papers"),
		"w2": worker("w2", "Coder", "Writes Go services"),
	}
	list := []api.ConversationSummary{summary("bee"), summary("worker:w1"), summary("worker:w2")}

	t.Run("keeps bee and matches description", func(t *testing.T) {
		got := convIDs(FilterConversations(list, workers, "papers"))

		want := []string{"bee", "worker:w1"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("matches the name case-insensitively", func(t *testing.T) {
		got := convIDs(FilterConversations(list, workers, "CODER"))

		want := []string{"bee", "worker:w2"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("returns the list unchanged for an empty query", func(t *testing.T) {
		for _, query := range []string{"", "   "} {
			got := FilterConversations(list, workers, query)
			if len(got) != len(list) || &got[0] != &list[0] {
				t.Fatalf("query %q returned a different list", query)
			}
		}
	})

	t.Run("drops workers that are not loaded", func(t *testing.T) {
		got := convIDs(FilterConversations(list, nil, "papers"))

		want := []string{"bee"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("trims the query", func(t *testing.T) {
		got := convIDs(FilterConversations(list, workers, "  coder "))

		want := []string{"bee", "worker:w2"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
}

func TestConversationPreview(t *testing.T) {
	workers := map[string]api.Worker{"w1": worker("w1", "Researcher", "Finds papers"), "w3": worker("w3", "Quiet", "")}
	conv := func(id string, mutate func(*api.LastMessage)) api.ConversationSummary {
		s := summary(id)
		last := &api.LastMessage{Sender: api.ChatSender{Type: "bee"}, Kind: "message", Preview: "write the report", TS: 1}
		if mutate != nil {
			mutate(last)
		}
		s.LastMessage = last
		return s
	}

	tests := []struct {
		name string
		cs   api.ConversationSummary
		want Preview
	}{
		{
			name: "prefixes the user's own messages",
			cs:   conv("bee", func(l *api.LastMessage) { l.Sender = api.ChatSender{Type: "user"} }),
			want: Preview{Kind: PreviewYou, Text: "write the report"},
		},
		{
			name: "shows other senders' preview as is",
			cs:   conv("worker:w1", func(l *api.LastMessage) { l.Sender = api.ChatSender{Type: "worker", ID: "w1"} }),
			want: Preview{Kind: PreviewPlain, Text: "write the report"},
		},
		{
			name: "names the handoff target",
			cs:   conv("bee", func(l *api.LastMessage) { l.Kind = "handoff"; l.HandoffWorkerID = "w1" }),
			want: Preview{Kind: PreviewHandoff, WorkerName: "Researcher"},
		},
		{
			name: "marks a handoff to a deleted worker",
			cs:   conv("bee", func(l *api.LastMessage) { l.Kind = "handoff"; l.HandoffWorkerID = "w9" }),
			want: Preview{Kind: PreviewHandoff, WorkerDeleted: true},
		},
		{
			name: "falls back to the preview when the handoff target is missing",
			cs:   conv("bee", func(l *api.LastMessage) { l.Kind = "handoff" }),
			want: Preview{Kind: PreviewPlain, Text: "write the report"},
		},
		{
			name: "labels a media-only message with images",
			cs:   conv("worker:w1", func(l *api.LastMessage) { l.Preview = ""; l.MediaPaths = []string{"a.png", "b.JPG"} }),
			want: Preview{Kind: PreviewPlain, Media: MediaImage},
		},
		{
			name: "labels a media-only message with any other file as a file",
			cs:   conv("worker:w1", func(l *api.LastMessage) { l.Preview = ""; l.MediaPaths = []string{"a.png", "report.pdf"} }),
			want: Preview{Kind: PreviewPlain, Media: MediaFile},
		},
		{
			name: "labels the user's own media-only message",
			cs: conv("bee", func(l *api.LastMessage) {
				l.Sender = api.ChatSender{Type: "user"}
				l.Preview = ""
				l.MediaPaths = []string{"photo.webp"}
			}),
			want: Preview{Kind: PreviewYou, Media: MediaImage},
		},
		{
			name: "keeps the text of a message with text and media",
			cs:   conv("worker:w1", func(l *api.LastMessage) { l.MediaPaths = []string{"a.png"} }),
			want: Preview{Kind: PreviewPlain, Text: "write the report"},
		},
		{
			name: "shows the worker description without a last message",
			cs:   summary("worker:w1"),
			want: Preview{Kind: PreviewDescription, Text: "Finds papers"},
		},
		{
			name: "is empty without a last message and a description",
			cs:   summary("worker:w3"),
			want: Preview{Kind: PreviewEmpty},
		},
		{
			name: "is empty for bee without a last message",
			cs:   summary("bee"),
			want: Preview{Kind: PreviewEmpty},
		},
		{
			name: "is empty for a deleted worker without a last message",
			cs:   summary("worker:gone"),
			want: Preview{Kind: PreviewEmpty},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ConversationPreview(tc.cs, workers); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
