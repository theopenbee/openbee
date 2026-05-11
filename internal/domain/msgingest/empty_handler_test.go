package msgingest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/theopenbee/openbee/internal/domain/msgingest"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/store"
	"github.com/theopenbee/openbee/internal/platform"
)

type fakeSender struct {
	sent      []platform.OutboundMessage
	errReturn error
}

func (f *fakeSender) Send(_ context.Context, msg platform.OutboundMessage) error {
	f.sent = append(f.sent, msg)
	return f.errReturn
}

func emptyInbound() platform.InboundMessage {
	return platform.InboundMessage{
		Platform:          "test",
		SessionKey:        "test:c1:u1",
		Content:           "",
		PlatformMessageID: "pm-1",
	}
}

func TestDefaultEmptyMessageHandler_SendsHint(t *testing.T) {
	if err := i18n.Load("zh"); err != nil {
		t.Fatalf("i18n.Load: %v", err)
	}
	sender := &fakeSender{}
	h := msgingest.NewDefaultEmptyMessageHandler(
		map[string]platform.PlatformSenderAdapter{"test": sender},
	)

	msg := emptyInbound()
	h.HandleEmpty(context.Background(), msg)

	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 send, got %d", len(sender.sent))
	}
	got := sender.sent[0]
	if got.Content != i18n.M.Runtime.EmptyMessage.Hint {
		t.Errorf("Content = %q, want %q", got.Content, i18n.M.Runtime.EmptyMessage.Hint)
	}
	if got.ReplyTo.PlatformMessageID != msg.PlatformMessageID {
		t.Errorf("ReplyTo.PlatformMessageID = %q, want %q",
			got.ReplyTo.PlatformMessageID, msg.PlatformMessageID)
	}
	if got.SourceType != store.SourceTypeSystem {
		t.Errorf("SourceType = %q, want %q", got.SourceType, store.SourceTypeSystem)
	}
}

func TestDefaultEmptyMessageHandler_NoSenderForPlatform(t *testing.T) {
	h := msgingest.NewDefaultEmptyMessageHandler(
		map[string]platform.PlatformSenderAdapter{},
	)
	h.HandleEmpty(context.Background(), emptyInbound())
}

func TestDefaultEmptyMessageHandler_SenderError(t *testing.T) {
	sender := &fakeSender{errReturn: errors.New("send failed")}
	h := msgingest.NewDefaultEmptyMessageHandler(
		map[string]platform.PlatformSenderAdapter{"test": sender},
	)
	h.HandleEmpty(context.Background(), emptyInbound())

	if len(sender.sent) != 1 {
		t.Errorf("expected exactly 1 send attempt, got %d", len(sender.sent))
	}
}
