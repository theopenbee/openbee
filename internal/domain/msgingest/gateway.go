package msgingest

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/theopenbee/openbee/internal/infra/logger"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
	"go.uber.org/zap"
)

var log = logger.With(zap.String("component", "msgingest"))

const mergedSeparator = "\n\n---\n\n"

// IngestedMessage is a deduplicated, debounced, normalized message ready for routing.
type IngestedMessage struct {
	MsgID      string
	SessionKey string
	Platform   string
	Content    string
	ReplyTo    platform.InboundMessage
}

// MessageStore is the subset of store.MessageStore used by msgingest.
type MessageStore interface {
	CreateBatch(ctx context.Context, msgs []store.BatchMsg) (int64, error)
}

type debounceState struct {
	timer      *time.Timer
	generation int
	msgs       []platform.InboundMessage // full message bodies, arrival order
	content    string                    // merged content string
}

// Gateway receives raw platform messages, deduplicates, debounces, and emits IngestedMessages.
type Gateway struct {
	msgStore       MessageStore
	debounce       time.Duration
	sessions       map[string]*debounceState
	seen           map[string]struct{} // in-memory dedup set keyed by platform_msg_id
	mu             sync.Mutex
	out            chan IngestedMessage
	commandHandler CommandHandler // optional; intercepts slash commands before DB write
}

// Option configures a Gateway.
type Option func(*Gateway)

// WithCommandHandler sets an optional slash-command handler.
// When set, each debounced message is offered to the handler before DB write.
// If the handler returns true, the message is consumed and not stored.
func WithCommandHandler(h CommandHandler) Option {
	return func(g *Gateway) { g.commandHandler = h }
}

// New constructs a Gateway.
func New(msgStore MessageStore, debounce time.Duration, opts ...Option) *Gateway {
	g := &Gateway{
		msgStore: msgStore,
		debounce: debounce,
		sessions: make(map[string]*debounceState),
		seen:     make(map[string]struct{}),
		out:      make(chan IngestedMessage, 64),
	}
	for _, o := range opts {
		o(g)
	}
	return g
}

// Out returns the channel of outgoing IngestedMessages.
func (g *Gateway) Out() <-chan IngestedMessage { return g.out }

// Run blocks until ctx is cancelled, then closes Out().
func (g *Gateway) Run(ctx context.Context) {
	<-ctx.Done()
	close(g.out)
}

// emit sends msg to the output channel non-blocking; drops and logs if the channel is full.
func (g *Gateway) emit(msg IngestedMessage) {
	select {
	case g.out <- msg:
	default:
		log.Warn("output channel full, dropping message", zap.String("sessionKey", msg.SessionKey))
	}
}

// Dispatch is called by a platform receiver for each inbound message.
// All seen-map and debounce-state mutations are protected by g.mu.
func (g *Gateway) Dispatch(msg platform.InboundMessage) {
	g.mu.Lock()

	// In-memory dedup: drop if platform_msg_id already seen this process lifetime.
	if msg.PlatformMessageID != "" {
		if _, dup := g.seen[msg.PlatformMessageID]; dup {
			g.mu.Unlock()
			log.Info("duplicate dropped", zap.String("platformMsgID", msg.PlatformMessageID))
			return
		}
		g.seen[msg.PlatformMessageID] = struct{}{}
	}

	// Accumulate into debounce state.
	state, ok := g.sessions[msg.SessionKey]
	if !ok {
		state = &debounceState{}
		g.sessions[msg.SessionKey] = state
	}

	if state.content == "" {
		state.content = msg.Content
	} else {
		state.content = state.content + mergedSeparator + msg.Content
	}
	state.msgs = append(state.msgs, msg)

	if state.timer != nil {
		state.timer.Stop()
	}
	state.generation++
	gen := state.generation
	sessionKey := msg.SessionKey
	state.timer = time.AfterFunc(g.debounce, func() { g.onDebounce(sessionKey, gen) })

	g.mu.Unlock()
}

func (g *Gateway) onDebounce(sessionKey string, generation int) {
	g.mu.Lock()
	state, ok := g.sessions[sessionKey]
	if !ok || len(state.msgs) == 0 {
		g.mu.Unlock()
		return
	}
	if state.generation != generation {
		g.mu.Unlock()
		return
	}
	msgs := state.msgs
	content := state.content
	delete(g.sessions, sessionKey)
	g.mu.Unlock()

	n := len(msgs)
	ids := make([]string, n)
	for i := range msgs {
		ids[i] = uuid.New().String()
	}
	primaryID := ids[n-1]

	batch := make([]store.BatchMsg, n)
	for i, m := range msgs {
		mt := m.MessageTime
		if mt == 0 {
			mt = time.Now().UnixMilli()
		}
		bm := store.BatchMsg{
			ID:            ids[i],
			SessionKey:    m.SessionKey,
			Platform:      m.Platform,
			Content:       m.Content,
			Raw:           m.Raw,
			PlatformMsgID: m.PlatformMessageID,
			MessageTime:   mt,
			MergedInto:    "",
		}
		if i < n-1 {
			bm.Status = "merged"
			bm.MergedInto = primaryID
		} else {
			bm.Status = "received"
		}
		batch[i] = bm
	}

	if g.commandHandler != nil {
		if g.commandHandler.HandleCommand(context.Background(), content, msgs[n-1]) {
			return
		}
	}

	inserted, err := g.msgStore.CreateBatch(context.Background(), batch)
	if err != nil {
		log.Error("CreateBatch error", zap.String("sessionKey", sessionKey), zap.Error(err))
		return
	}
	if inserted != int64(n) {
		log.Warn("CreateBatch partial insert, suppressing emit", zap.String("sessionKey", sessionKey), zap.Int("expected", n), zap.Int64("got", inserted))
		return
	}

	g.emit(IngestedMessage{
		MsgID:      primaryID,
		SessionKey: sessionKey,
		Platform:   msgs[n-1].Platform,
		Content:    content,
		ReplyTo:    msgs[n-1],
	})
}
