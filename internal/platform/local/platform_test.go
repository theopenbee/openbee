package local_test

import (
	"testing"

	"github.com/theopenbee/openbee/internal/platform/local"
)

func TestLocalPlatform_AccountName(t *testing.T) {
	p := local.NewPlatform(local.NewLocalReceiver(1), local.NewLocalSender(local.NewSSEHub()))
	if p.ID() != "local" {
		t.Errorf("ID() = %q, want local", p.ID())
	}
	if got := p.AccountName(); got != "default" {
		t.Errorf("AccountName() = %q, want default", got)
	}
}
