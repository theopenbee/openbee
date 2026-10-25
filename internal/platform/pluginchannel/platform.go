package pluginchannel

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/platform"
)

const sendTimeout = 120 * time.Second

type Platform struct {
	id       string
	mediaSvc *media.Service
	sup      *Supervisor

	mu       sync.RWMutex
	dispatch func(platform.InboundMessage)
}

func New(spec Spec, mediaSvc *media.Service) *Platform {
	p := &Platform{id: spec.ChannelID, mediaSvc: mediaSvc}
	p.sup = NewSupervisor(spec, p.handleNotify)
	return p
}

func (p *Platform) ID() string                                 { return p.id }
func (p *Platform) Receiver() platform.PlatformReceiverAdapter { return receiver{p} }
func (p *Platform) Sender() platform.PlatformSenderAdapter     { return sender{p} }
func (p *Platform) Done() <-chan struct{}                      { return p.sup.Done() }

func (p *Platform) handleNotify(method string, params json.RawMessage) {
	if method != "inbound" {
		p.sup.log.Debug("ignoring plugin notification", zap.String("method", method))
		return
	}
	var in InboundParams
	if err := json.Unmarshal(params, &in); err != nil {
		p.sup.log.Warn("dropping malformed inbound message", zap.Error(err))
		return
	}
	msg, err := ToInbound(p.id, p.mediaSvc, in)
	if err != nil {
		p.sup.log.Warn("dropping invalid inbound message", zap.Error(err))
		return
	}
	p.mu.RLock()
	dispatch := p.dispatch
	p.mu.RUnlock()
	if dispatch == nil {
		p.sup.log.Warn("dropping inbound message before receiver start")
		return
	}
	dispatch(msg)
}

type receiver struct{ p *Platform }

func (r receiver) Start(ctx context.Context, dispatch func(platform.InboundMessage)) error {
	r.p.mu.Lock()
	r.p.dispatch = dispatch
	r.p.mu.Unlock()
	r.p.sup.Run(ctx)
	return nil
}

type sender struct{ p *Platform }

func (s sender) SendsTextWithMedia() bool { return true }

func (s sender) Send(ctx context.Context, msg platform.OutboundMessage) error {
	key := msg.ReplyTo.SessionKey
	if key == "" {
		key = msg.SessionKey
	}
	chatID, scope, err := SplitSessionKey(s.p.id, key)
	if err != nil {
		return err
	}
	if msg.Content == "" && msg.MediaPath == "" {
		return errors.New("plugin send needs content or media")
	}
	params := SendParams{
		SessionKey:   key,
		ChatID:       chatID,
		SessionScope: scope,
		Proactive:    msg.Proactive,
		Content:      msg.Content,
	}
	if raw, messageID, _ := DecodeEnvelope(msg.ReplyTo.Raw); raw != "" || messageID != "" {
		params.ReplyTo = &ReplyTo{MessageID: messageID, Raw: raw}
	}
	if msg.MediaPath != "" {
		params.Media = &Media{Path: msg.MediaPath, Name: filepath.Base(msg.MediaPath)}
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	var res SendResult
	return s.p.sup.Call(ctx, "send", params, &res)
}

var (
	_ platform.Platform                = (*Platform)(nil)
	_ platform.PlatformReceiverAdapter = receiver{}
	_ platform.PlatformSenderAdapter   = sender{}
	_ platform.CombinedSender          = sender{}
)
