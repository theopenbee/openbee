package local

import "github.com/theopenbee/openbee/internal/platform"

// PlatformID is the platform identifier for the local chat platform.
const PlatformID = "local"

// LocalPlatform bundles LocalReceiver and LocalSender and implements platform.Platform.
type LocalPlatform struct {
	receiver *LocalReceiver
	sender   *LocalSender
}

// NewPlatform constructs a LocalPlatform from pre-built receiver and sender.
func NewPlatform(receiver *LocalReceiver, sender *LocalSender) *LocalPlatform {
	return &LocalPlatform{receiver: receiver, sender: sender}
}

func (p *LocalPlatform) ID() string                                { return PlatformID }
func (p *LocalPlatform) Receiver() platform.PlatformReceiverAdapter { return p.receiver }
func (p *LocalPlatform) Sender() platform.PlatformSenderAdapter    { return p.sender }
