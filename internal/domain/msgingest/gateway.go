package msgingest

import (
	"context"
	"regexp"
	"strings"
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

// seenMaxSize caps the dedup set at ~2x this value via a two-generation rotation.
// Once the active generation hits the cap, it rotates to cold and a fresh map
// takes over. Entries older than one full cap fall out naturally.
const seenMaxSize = 10000

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

type commandTask struct {
	content string
	msg     platform.InboundMessage
}

// Gateway receives raw platform messages, deduplicates, debounces, and emits IngestedMessages.
type Gateway struct {
	msgStore       MessageStore
	debounce       time.Duration
	sessions       map[string]*debounceState
	seen           map[string]struct{} // in-memory dedup set keyed by platform_msg_id
	seenPrev       map[string]struct{} // previous generation, checked on lookup only
	mu             sync.Mutex
	out            chan IngestedMessage
	cmdCh          chan commandTask           // serialized command dispatch queue
	commandHandler CommandHandler            // intercepts slash commands before DB write
	botNameREs     map[string]*regexp.Regexp // platform → compiled @mention regex
	emptyHandler   EmptyMessageHandler
}

// Option configures a Gateway.
type Option func(*Gateway)

// WithPlatformBotNames sets a per-platform bot display name whose @mention is stripped
// from message content before command matching, debounce accumulation, and DB storage.
func WithPlatformBotNames(names map[string]string) Option {
	res := make(map[string]*regexp.Regexp, len(names))
	for platform, name := range names {
		if name != "" {
			res[platform] = regexp.MustCompile(`\s*@` + regexp.QuoteMeta(name) + `\s*`)
		}
	}
	return func(g *Gateway) { g.botNameREs = res }
}

// WithEmptyMessageHandler registers a handler invoked when a message is empty
// after the bot @mention is stripped. When unset, empty messages are silently dropped.
func WithEmptyMessageHandler(h EmptyMessageHandler) Option {
	return func(g *Gateway) { g.emptyHandler = h }
}

func (g *Gateway) stripBotMention(content, platform string) string {
	re, ok := g.botNameREs[platform]
	if !ok || !strings.Contains(content, "@") {
		return content
	}
	return strings.TrimSpace(re.ReplaceAllString(content, " "))
}

// New constructs a Gateway.
func New(msgStore MessageStore, debounce time.Duration, handler CommandHandler, opts ...Option) *Gateway {
	g := &Gateway{
		msgStore:       msgStore,
		debounce:       debounce,
		commandHandler: handler,
		sessions:       make(map[string]*debounceState),
		seen:           make(map[string]struct{}),
		out:            make(chan IngestedMessage, 64),
		cmdCh:          make(chan commandTask, 32),
	}
	for _, o := range opts {
		o(g)
	}
	return g
}

// Out returns the channel of outgoing IngestedMessages.
func (g *Gateway) Out() <-chan IngestedMessage { return g.out }

func (g *Gateway) Run(ctx context.Context) {
	go g.runCommandConsumer(ctx)
	<-ctx.Done()
	close(g.out)
}

func (g *Gateway) runCommandConsumer(ctx context.Context) {
	for {
		select {
		case task := <-g.cmdCh:
			g.commandHandler.HandleCommand(ctx, task.content, task.msg)
		case <-ctx.Done():
			return
		}
	}
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
	stripped := g.stripBotMention(msg.Content, msg.Platform)
	g.mu.Lock()

	if msg.PlatformMessageID != "" {
		_, dup := g.seen[msg.PlatformMessageID]
		if !dup {
			_, dup = g.seenPrev[msg.PlatformMessageID]
		}
		if dup {
			g.mu.Unlock()
			log.Info("duplicate dropped", zap.String("platformMsgID", msg.PlatformMessageID))
			return
		}
		if len(g.seen) >= seenMaxSize {
			g.seenPrev = g.seen
			g.seen = make(map[string]struct{})
		}
		g.seen[msg.PlatformMessageID] = struct{}{}
	}

	// Empty-message short-circuit: must come after dedup (so platform retries
	// don't cause double replies) and before the command/debounce branches (so
	// empty content never enters DB or accumulation state).
	if strings.TrimSpace(stripped) == "" {
		g.mu.Unlock()
		log.Info("empty message after strip",
			zap.String("sessionKey", msg.SessionKey),
			zap.String("platform", msg.Platform),
			zap.String("platformMsgID", msg.PlatformMessageID))
		if g.emptyHandler != nil {
			g.emptyHandler.HandleEmpty(context.Background(), msg)
		}
		return
	}

	if g.commandHandler.IsCommand(stripped) {
		g.mu.Unlock()
		select {
		case g.cmdCh <- commandTask{stripped, msg}:
		default:
			log.Warn("command channel full, dropping command", zap.String("sessionKey", msg.SessionKey))
		}
		return
	}

	// Accumulate into debounce state.
	state, ok := g.sessions[msg.SessionKey]
	if !ok {
		state = &debounceState{}
		g.sessions[msg.SessionKey] = state
	}

	if state.content == "" {
		state.content = stripped
	} else {
		state.content = state.content + mergedSeparator + stripped
	}
	msg.Content = stripped
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
