package delivery

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

type MessageLookup interface {
	GetByID(ctx context.Context, id string) (store.StoredMessage, error)
	LatestBySessionKey(ctx context.Context, sessionKey string) (store.StoredMessage, bool, error)
}

type Target struct {
	MessageID  string
	SessionKey string
}

type Resolver struct {
	messages MessageLookup
	senders  map[string]platform.PlatformSenderAdapter
}

func NewResolver(messages MessageLookup, senders map[string]platform.PlatformSenderAdapter) *Resolver {
	return &Resolver{messages: messages, senders: senders}
}

func (r *Resolver) Resolve(ctx context.Context, t Target) (platform.PlatformSenderAdapter, platform.OutboundMessage, error) {
	switch {
	case t.MessageID != "" && t.SessionKey != "":
		return nil, platform.OutboundMessage{}, errors.New("message_id and session_key are mutually exclusive")
	case t.SessionKey != "":
		return r.proactive(ctx, t.SessionKey)
	case t.MessageID != "":
		return r.reply(ctx, t.MessageID)
	default:
		return nil, platform.OutboundMessage{}, errors.New("message_id or session_key is required")
	}
}

func (r *Resolver) reply(ctx context.Context, messageID string) (platform.PlatformSenderAdapter, platform.OutboundMessage, error) {
	stored, err := r.messages.GetByID(ctx, messageID)
	if err != nil {
		return nil, platform.OutboundMessage{}, fmt.Errorf("get message: %w", err)
	}
	sender, err := r.sender(stored.Platform)
	if err != nil {
		return nil, platform.OutboundMessage{}, err
	}
	return sender, platform.OutboundMessage{
		SessionKey: stored.SessionKey,
		ReplyTo: platform.InboundMessage{
			Platform:   stored.Platform,
			SessionKey: stored.SessionKey,
			Raw:        stored.Raw,
		},
		InboundMsgID: messageID,
	}, nil
}

func (r *Resolver) proactive(ctx context.Context, sessionKey string) (platform.PlatformSenderAdapter, platform.OutboundMessage, error) {
	latest, found, err := r.messages.LatestBySessionKey(ctx, sessionKey)
	if err != nil {
		return nil, platform.OutboundMessage{}, err
	}
	platformID := latest.Platform
	if !found {
		platformID, _, _ = strings.Cut(sessionKey, ":")
	}
	sender, err := r.sender(platformID)
	if err != nil {
		return nil, platform.OutboundMessage{}, err
	}
	return sender, platform.OutboundMessage{
		SessionKey: sessionKey,
		ReplyTo: platform.InboundMessage{
			Platform:   platformID,
			SessionKey: sessionKey,
			Raw:        latest.Raw,
		},
		Proactive: true,
	}, nil
}

func (r *Resolver) sender(platformID string) (platform.PlatformSenderAdapter, error) {
	sender, ok := r.senders[platformID]
	if !ok {
		return nil, fmt.Errorf("no sender registered for platform %q", platformID)
	}
	return sender, nil
}
