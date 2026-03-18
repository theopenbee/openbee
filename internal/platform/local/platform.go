package local

import "github.com/theopenbee/openbee/internal/platform"

// LocalPlatform bundles LocalReceiver and LocalSender and implements platform.Platform.
type LocalPlatform struct {
	receiver *LocalReceiver
	sender   *LocalSender
}

// NewPlatform constructs a LocalPlatform from pre-built receiver and sender.
func NewPlatform(receiver *LocalReceiver, sender *LocalSender) *LocalPlatform {
	return &LocalPlatform{receiver: receiver, sender: sender}
}

func (p *LocalPlatform) ID() string                                { return "local" }
func (p *LocalPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *LocalPlatform) Sender() platform.PlatformSenderAdapter    { return p.sender }
