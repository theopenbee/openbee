package pluginmgr

import (
	"context"

	"github.com/theopenbee/openbee/internal/infra/buildinfo"
	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/platform/pluginchannel"
	"github.com/theopenbee/openbee/internal/plugin"
)

type Instance interface {
	Sender() platform.PlatformSenderAdapter
	Run(ctx context.Context, dispatch func(platform.InboundMessage))
}

type InstanceSpec struct {
	ID       string
	Manifest plugin.Manifest
	Config   map[string]any
	BotName  string
	DataDir  string
	OnState  func(state pluginchannel.State, err error)
}

type Factory func(InstanceSpec) Instance

func PluginChannelFactory(runtime *plugin.RuntimeResolver, mediaSvc *media.Service, migrator pluginchannel.SessionKeyMigrator, language string) Factory {
	return func(s InstanceSpec) Instance {
		return pluginchannel.New(pluginchannel.Spec{
			ChannelID: s.ID,
			Dir:       s.Manifest.Dir,
			Command:   s.Manifest.Command,
			Runtime:   runtime,
			OnState:   s.OnState,
			Init: pluginchannel.InitializeParams{
				ProtocolVersion: pluginchannel.ProtocolVersion,
				OpenbeeVersion:  buildinfo.Version,
				ChannelID:       s.ID,
				Language:        language,
				BotName:         s.BotName,
				Config:          s.Config,
				DataDir:         s.DataDir,
				MediaDir:        mediaSvc.InboundDir(),
			},
		}, mediaSvc, migrator)
	}
}
