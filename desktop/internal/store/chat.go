package store

import (
	"context"
	"math"
	"slices"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/model"
)

const (
	messagesPageSize = 50
	senderUser       = "user"
	kindMessage      = "message"
	kindReset        = "reset"
)

type Chat struct {
	ID           string
	List         model.List
	Loaded       bool
	Loading      bool
	LoadingOlder bool
	Err          error
	OlderErr     error
	BeeBusy      bool
	Stopping     bool
	Clearing     bool

	s         *Store
	seq       uint64
	stale     bool
	busySince int64
}

func (s *Store) Chat(cid string) *Chat {
	if ch, ok := s.chats[cid]; ok {
		return ch
	}
	ch := &Chat{ID: cid, s: s}
	s.chats[cid] = ch
	return ch
}

func (s *Store) resetChats() {
	for _, ch := range s.chats {
		ch.reset()
	}
}

func (ch *Chat) Ensure() {
	if ch.Loading || (ch.Loaded || ch.Err != nil) && !ch.stale {
		return
	}
	ch.Refresh()
}

func (ch *Chat) Refresh() {
	ch.seq++
	seq := ch.seq
	ch.Loading = true
	ch.stale = false
	cid := ch.ID
	ch.s.async(func(ctx context.Context, c *api.Client) func() {
		page, err := c.Messages(ctx, cid, 0, messagesPageSize)
		return func() {
			if ch.seq != seq {
				return
			}
			ch.Loading = false
			if err != nil {
				ch.Err = err
			} else {
				ch.List = ch.List.Latest(page)
				ch.Loaded = true
				ch.Err = nil
				ch.settleBusy(page.Messages)
			}
		}
	}, nil)
}

func (ch *Chat) settleBusy(msgs []api.ChatMessage) {
	if ch.BeeBusy && slices.ContainsFunc(msgs, func(m api.ChatMessage) bool { return isReply(m) && m.TS >= ch.busySince }) {
		ch.BeeBusy = false
	}
}

func isReply(m api.ChatMessage) bool {
	return m.Sender.Type != senderUser && m.Kind != kindReset
}

func (ch *Chat) LoadOlder() {
	if ch.LoadingOlder {
		return
	}
	before := ch.s.o.Now().UnixMilli()
	anchor := ""
	if len(ch.List.Msgs) > 0 {
		before = ch.List.Msgs[0].TS
		anchor = ch.List.Msgs[0].ID
	}
	ch.LoadingOlder = true
	ch.OlderErr = nil
	cid := ch.ID
	ch.s.async(func(ctx context.Context, c *api.Client) func() {
		page, err := c.Messages(ctx, cid, before, messagesPageSize)
		return func() {
			ch.LoadingOlder = false
			if anchor != "" && (len(ch.List.Msgs) == 0 || ch.List.Msgs[0].ID != anchor) {
				return
			}
			if err != nil {
				ch.OlderErr = err
				return
			}
			ch.List = ch.List.Older(page)
		}
	}, nil)
}

func (ch *Chat) Send(text string, media []string, done func(error)) {
	done = orNop(done)
	if len(media) == 0 {
		media = nil
	} else {
		media = slices.Clone(media)
	}
	id := model.NewMessageID()
	ch.List = ch.List.Add(model.Message{
		ChatMessage: api.ChatMessage{
			ID:         id,
			Sender:     api.ChatSender{Type: senderUser},
			Kind:       kindMessage,
			Content:    text,
			MediaPaths: media,
			TS:         ch.s.o.Now().UnixMilli(),
		},
		Pending: true,
	})
	bee := ch.isBee()
	if bee {
		ch.BeeBusy = true
		ch.busySince = math.MaxInt64
	}
	cid := ch.ID
	ch.s.async(func(ctx context.Context, c *api.Client) func() {
		sent, err := c.SendMessage(ctx, cid, api.SendMessageRequest{ID: id, Content: text, MediaPaths: media})
		return func() {
			if err != nil {
				ch.List = ch.List.Remove(id)
				if bee {
					ch.BeeBusy = false
				}
				done(err)
				return
			}
			ch.List = ch.List.Sent(id, sent.TS)
			if bee {
				ch.busySince = sent.TS
			}
			ch.s.noteSent(cid, api.ChatMessage{
				ID:         id,
				Sender:     api.ChatSender{Type: senderUser},
				Kind:       kindMessage,
				Content:    text,
				MediaPaths: media,
				TS:         sent.TS,
			})
			done(nil)
		}
	}, canceledBy(done))
}

func (ch *Chat) MarkRead(ts int64) {
	s := ch.s
	if s.active != ch.ID || !s.foreground {
		return
	}
	cid := ch.ID
	s.async(func(ctx context.Context, c *api.Client) func() {
		err := c.MarkRead(ctx, cid, ts)
		return func() {
			if err == nil {
				s.Conversations.Invalidate()
			}
		}
	}, nil)
}

func (ch *Chat) Clear(force bool, done func(active []api.ActiveTask, err error)) {
	if done == nil {
		done = func([]api.ActiveTask, error) {}
	}
	ch.Clearing = true
	cid := ch.ID
	ch.s.async(func(ctx context.Context, c *api.Client) func() {
		err := c.ClearConversation(ctx, cid, force)
		return func() {
			ch.Clearing = false
			if err != nil {
				done(api.ActiveTasks(err), err)
				return
			}
			ch.s.Conversations.Invalidate()
			ch.Refresh()
			done(nil, nil)
		}
	}, func() { done(nil, context.Canceled) })
}

func (ch *Chat) Stop(done func(error)) {
	done = orNop(done)
	ch.Stopping = true
	cid := ch.ID
	ch.s.async(func(ctx context.Context, c *api.Client) func() {
		err := c.StopConversation(ctx, cid)
		return func() {
			ch.Stopping = false
			if err == nil {
				ch.s.Conversations.Invalidate()
			}
			done(err)
		}
	}, canceledBy(done))
}

func (ch *Chat) receive(m api.ChatMessage) {
	ch.List = ch.List.Add(model.Message{ChatMessage: m, Pending: true})
	if !isReply(m) {
		return
	}
	if ch.isBee() {
		ch.BeeBusy = false
	}
	ch.MarkRead(m.TS)
}

func (ch *Chat) resync() {
	switch {
	case !ch.Loaded && !ch.Loading && ch.Err == nil:
	case ch.ID == ch.s.active:
		ch.Refresh()
		ch.MarkRead(0)
	default:
		ch.stale = true
	}
}

func (ch *Chat) isBee() bool {
	return ch.ID == api.BeeConversation
}

func (ch *Chat) reset() {
	ch.seq++
	ch.stale = false
	ch.busySince = 0
	ch.List = model.List{}
	ch.Loaded = false
	ch.Loading = false
	ch.LoadingOlder = false
	ch.Err = nil
	ch.OlderErr = nil
	ch.BeeBusy = false
	ch.Stopping = false
	ch.Clearing = false
}
