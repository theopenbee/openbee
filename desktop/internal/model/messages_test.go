package model

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func chatMessage(id string, ts int64) api.ChatMessage {
	return api.ChatMessage{ID: id, Sender: api.ChatSender{Type: "user"}, Kind: "message", Content: id, TS: ts}
}

func msg(id string, ts int64) Message {
	return Message{ChatMessage: chatMessage(id, ts)}
}

func live(id string, ts int64) Message {
	m := msg(id, ts)
	m.Pending = true
	return m
}

func page(hasMore bool, messages ...Message) api.MessagesPage {
	out := make([]api.ChatMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, m.ChatMessage)
	}
	return api.MessagesPage{Messages: out, HasMore: hasMore}
}

func listOf(hasMore bool, messages ...Message) List {
	return List{Msgs: messages, HasMore: hasMore}
}

func msgIDs(l List) []string {
	out := make([]string, 0, len(l.Msgs))
	for _, m := range l.Msgs {
		out = append(out, m.ID)
	}
	return out
}

func TestMessagesLatestMerge(t *testing.T) {
	t.Run("takes the first page as is", func(t *testing.T) {
		got := List{}.Latest(page(true, msg("a", 10)))

		want := listOf(true, msg("a", 10))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("keeps older history when the refreshed page overlaps it", func(t *testing.T) {
		prev := listOf(true, msg("a", 1), msg("b", 2), msg("c", 10), msg("d", 11))

		got := prev.Latest(page(true, msg("c", 10), msg("d", 11), msg("e", 12)))

		want := []string{"a", "b", "c", "d", "e"}
		if ids := msgIDs(got); !reflect.DeepEqual(ids, want) {
			t.Fatalf("got %v, want %v", ids, want)
		}
	})

	t.Run("replaces an optimistic message with its stored copy despite clock skew", func(t *testing.T) {
		bee := msg("r", 1010)
		bee.Sender = api.ChatSender{Type: "bee"}
		prev := listOf(false, msg("x", 1), msg("y", 2), live("u", 1000))

		got := prev.Latest(page(true, msg("u", 1005), bee))

		want := []Message{msg("u", 1005), bee}
		if !reflect.DeepEqual(got.Msgs, want) {
			t.Fatalf("got %+v, want %+v", got.Msgs, want)
		}
	})

	t.Run("drops disconnected older history instead of leaving a hidden gap", func(t *testing.T) {
		prev := listOf(false, msg("m1", 1), msg("m2", 2))

		got := prev.Latest(page(true, msg("m101", 101), msg("m150", 150)))

		want := listOf(true, msg("m101", 101), msg("m150", 150))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("keeps live messages the snapshot has not stored yet", func(t *testing.T) {
		prev := listOf(false, msg("a", 10), msg("b", 11), live("reply", 12), live("sent", 13))

		got := prev.Latest(page(false, msg("a", 10), msg("b", 11)))

		want := []string{"a", "b", "reply", "sent"}
		if ids := msgIDs(got); !reflect.DeepEqual(ids, want) {
			t.Fatalf("got %v, want %v", ids, want)
		}
	})

	t.Run("keeps live messages when the server has nothing stored yet", func(t *testing.T) {
		got := listOf(false, live("first", 5)).Latest(api.MessagesPage{})

		want := []string{"first"}
		if ids := msgIDs(got); !reflect.DeepEqual(ids, want) {
			t.Fatalf("got %v, want %v", ids, want)
		}
	})

	t.Run("drops stale live messages that precede a disconnected page", func(t *testing.T) {
		prev := listOf(false, msg("a", 1), live("lost", 2))

		got := prev.Latest(page(true, msg("z", 50)))

		want := []string{"z"}
		if ids := msgIDs(got); !reflect.DeepEqual(ids, want) {
			t.Fatalf("got %v, want %v", ids, want)
		}
	})

	t.Run("preserves hasMore from earlier pages when older history is kept", func(t *testing.T) {
		prev := listOf(false, msg("a", 1), msg("b", 2))

		got := prev.Latest(page(true, msg("b", 2), msg("c", 3)))

		if got.HasMore {
			t.Fatal("HasMore = true, want false")
		}
	})

	t.Run("adopts the server hasMore when the page starts the list", func(t *testing.T) {
		prev := listOf(false, msg("a", 1), msg("b", 2))

		got := prev.Latest(page(true, msg("a", 1), msg("b", 2)))

		if !got.HasMore {
			t.Fatal("HasMore = false, want true")
		}
	})

	t.Run("sorts the result by timestamp", func(t *testing.T) {
		prev := listOf(false, msg("a", 10), live("late", 30))

		got := prev.Latest(page(false, msg("a", 10), msg("b", 20)))

		want := []string{"a", "b", "late"}
		if ids := msgIDs(got); !reflect.DeepEqual(ids, want) {
			t.Fatalf("got %v, want %v", ids, want)
		}
	})

	t.Run("does not mutate the receiver", func(t *testing.T) {
		prev := listOf(true, msg("c", 10), live("u", 12))
		before := listOf(true, msg("c", 10), live("u", 12))

		prev.Latest(page(false, msg("c", 10), msg("d", 11)))

		if !reflect.DeepEqual(prev, before) {
			t.Fatalf("receiver changed to %+v", prev)
		}
	})
}

func TestMessagesOlder(t *testing.T) {
	t.Run("prepends unseen messages and records hasMore", func(t *testing.T) {
		got := listOf(true, msg("c", 3)).Older(page(false, msg("a", 1), msg("b", 2), msg("c", 3)))

		want := listOf(false, msg("a", 1), msg("b", 2), msg("c", 3))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("keeps the list when nothing is new", func(t *testing.T) {
		got := listOf(true, msg("c", 3)).Older(page(false, msg("c", 3)))

		want := listOf(false, msg("c", 3))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("does not mutate the receiver", func(t *testing.T) {
		prev := listOf(true, msg("c", 3))

		prev.Older(page(false, msg("a", 1)))

		if !reflect.DeepEqual(prev, listOf(true, msg("c", 3))) {
			t.Fatalf("receiver changed to %+v", prev)
		}
	})
}

func TestMessagesLiveUpdates(t *testing.T) {
	t.Run("ignores a live message that is already shown", func(t *testing.T) {
		prev := listOf(false, msg("r", 1))

		got := prev.Add(live("r", 1))

		if !reflect.DeepEqual(got, prev) {
			t.Fatalf("got %+v, want %+v", got, prev)
		}
	})

	t.Run("appends a new message without sorting", func(t *testing.T) {
		got := listOf(false, msg("b", 5)).Add(live("a", 1))

		want := []string{"b", "a"}
		if ids := msgIDs(got); !reflect.DeepEqual(ids, want) {
			t.Fatalf("got %v, want %v", ids, want)
		}
	})

	t.Run("adopts the server timestamp once a send is accepted", func(t *testing.T) {
		got := listOf(false, live("u", 1000)).Sent("u", 1005)

		want := []Message{live("u", 1005)}
		if !reflect.DeepEqual(got.Msgs, want) {
			t.Fatalf("got %+v, want %+v", got.Msgs, want)
		}
	})

	t.Run("removes a message whose send failed", func(t *testing.T) {
		got := listOf(false, msg("a", 1), live("u", 2)).Remove("u")

		want := []string{"a"}
		if ids := msgIDs(got); !reflect.DeepEqual(ids, want) {
			t.Fatalf("got %v, want %v", ids, want)
		}
	})

	t.Run("leave the receiver untouched", func(t *testing.T) {
		prev := listOf(true, msg("a", 1), live("u", 2))
		before := listOf(true, msg("a", 1), live("u", 2))

		prev.Add(live("n", 3))
		prev.Sent("u", 9)
		prev.Remove("a")

		if !reflect.DeepEqual(prev, before) {
			t.Fatalf("receiver changed to %+v", prev)
		}
	})
}

func TestNewMessageID(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9a-f]{32}$`)
	a := NewMessageID()

	if !pattern.MatchString(a) {
		t.Fatalf("id %q is not 32 lowercase hex characters", a)
	}
	if b := NewMessageID(); b == a {
		t.Fatalf("two ids are equal: %q", a)
	}
}
