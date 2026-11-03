package model

import (
	"crypto/rand"
	"encoding/hex"
	"math"
	"slices"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

type Message struct {
	api.ChatMessage
	Pending bool
}

type List struct {
	Msgs    []Message
	HasMore bool
}

func NewMessageID() string {
	var b [16]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (l List) Latest(page api.MessagesPage) List {
	latest := make([]Message, len(page.Messages))
	latestIDs := make(map[string]struct{}, len(page.Messages))
	for i, m := range page.Messages {
		latest[i] = Message{ChatMessage: m}
		latestIDs[m.ID] = struct{}{}
	}
	firstTS := int64(math.MinInt64)
	if len(latest) > 0 {
		firstTS = latest[0].TS
	}
	contiguous := slices.ContainsFunc(l.Msgs, func(m Message) bool {
		_, ok := latestIDs[m.ID]
		return !m.Pending && ok
	})
	var older, pending []Message
	for _, m := range l.Msgs {
		if _, ok := latestIDs[m.ID]; ok {
			continue
		}
		if m.Pending {
			if contiguous || m.TS >= firstTS {
				pending = append(pending, m)
			}
		} else if contiguous && m.TS <= firstTS {
			older = append(older, m)
		}
	}
	msgs := slices.Concat(older, latest, pending)
	slices.SortStableFunc(msgs, func(a, b Message) int {
		switch {
		case a.TS < b.TS:
			return -1
		case a.TS > b.TS:
			return 1
		}
		return 0
	})
	hasMore := page.HasMore
	if len(older) > 0 {
		hasMore = l.HasMore
	}
	return List{Msgs: msgs, HasMore: hasMore}
}

func (l List) Older(page api.MessagesPage) List {
	ids := make(map[string]struct{}, len(l.Msgs))
	for _, m := range l.Msgs {
		ids[m.ID] = struct{}{}
	}
	var older []Message
	for _, m := range page.Messages {
		if _, ok := ids[m.ID]; !ok {
			older = append(older, Message{ChatMessage: m})
		}
	}
	msgs := l.Msgs
	if len(older) > 0 {
		msgs = slices.Concat(older, l.Msgs)
	}
	return List{Msgs: msgs, HasMore: page.HasMore}
}

func (l List) Add(m Message) List {
	if slices.ContainsFunc(l.Msgs, func(x Message) bool { return x.ID == m.ID }) {
		return l
	}
	return List{Msgs: slices.Concat(l.Msgs, []Message{m}), HasMore: l.HasMore}
}

func (l List) Sent(id string, ts int64) List {
	msgs := slices.Clone(l.Msgs)
	for i := range msgs {
		if msgs[i].ID == id {
			msgs[i].TS = ts
		}
	}
	return List{Msgs: msgs, HasMore: l.HasMore}
}

func (l List) Remove(id string) List {
	msgs := slices.DeleteFunc(slices.Clone(l.Msgs), func(m Message) bool { return m.ID == id })
	return List{Msgs: msgs, HasMore: l.HasMore}
}
