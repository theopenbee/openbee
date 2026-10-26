package pluginchannel

import (
	"fmt"
	"os"
	"sort"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/internal/infra/buildinfo"
	"github.com/theopenbee/openbee/internal/infra/media"
	"github.com/theopenbee/openbee/internal/platform"
	"github.com/theopenbee/openbee/internal/plugin"
)

type BuildOptions struct {
	Registry *plugin.Registry
	Runtime  *plugin.RuntimeResolver
	MediaSvc *media.Service
	Migrator SessionKeyMigrator
	Language string
}

type BuiltChannel struct {
	Channel platform.Channel
	Done    <-chan struct{}
}

func BuildChannels(sections map[string]map[string]any, opts BuildOptions) []BuiltChannel {
	ids := make([]string, 0, len(sections))
	for id := range sections {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var out []BuiltChannel
	for _, id := range ids {
		section := sections[id]
		enabled, err := plugin.SectionEnabled(section)
		if err != nil {
			log.Error("plugin channel skipped", zap.String("channel", id), zap.Error(err))
			continue
		}
		if !enabled {
			continue
		}
		p, err := buildOne(id, section, opts)
		if err != nil {
			log.Error("plugin channel skipped", zap.String("channel", id), zap.Error(err))
			continue
		}
		out = append(out, BuiltChannel{
			Channel: platform.Channel{Platform: p, ExtractContext: ExtractContextFor(id)},
			Done:    p.Done(),
		})
	}
	return out
}

func buildOne(id string, section map[string]any, opts BuildOptions) (*Platform, error) {
	entry, ok, err := opts.Registry.Get(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("plugin %q is not installed; run `openbee plugin install`", id)
	}
	m, err := plugin.LoadManifest(entry.Path)
	if err != nil {
		return nil, err
	}
	if m.ID != id {
		return nil, fmt.Errorf("plugin at %s declares id %q, expected %q", entry.Path, m.ID, id)
	}
	cfg, err := plugin.ResolveConfig(m, section)
	if err != nil {
		return nil, err
	}
	dataDir := opts.Registry.DataDir(id)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create plugin data dir: %w", err)
	}
	if err := os.Chmod(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("secure plugin data dir: %w", err)
	}
	botName, _ := section["bot_name"].(string)
	return New(Spec{
		ChannelID: id,
		Dir:       m.Dir,
		Command:   m.Command,
		Runtime:   opts.Runtime,
		Init: InitializeParams{
			ProtocolVersion: ProtocolVersion,
			OpenbeeVersion:  buildinfo.Version,
			ChannelID:       id,
			Language:        opts.Language,
			BotName:         botName,
			Config:          cfg,
			DataDir:         dataDir,
			MediaDir:        opts.MediaSvc.InboundDir(),
		},
	}, opts.MediaSvc, opts.Migrator), nil
}
