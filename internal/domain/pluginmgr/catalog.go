package pluginmgr

import (
	"context"
	"errors"

	"github.com/theopenbee/openbee/internal/platform/pluginchannel"
	"github.com/theopenbee/openbee/internal/plugin"
	"github.com/theopenbee/openbee/internal/plugin/catalog"
)

const supportedCategory = "channel"

type CatalogSource interface {
	Get(ctx context.Context, refresh bool) (catalog.Catalog, error)
}

type CatalogPlugin struct {
	catalog.Entry
	Installed        bool
	InstalledVersion string
	UpdateAvailable  bool
	Installable      bool
}

type CatalogView struct {
	SchemaVersion int
	Categories    []catalog.Vocab
	Tags          []catalog.Vocab
	Plugins       []CatalogPlugin
}

func (m *Manager) Catalog(ctx context.Context, refresh bool) (CatalogView, error) {
	if m.deps.Catalog == nil {
		return CatalogView{}, errCatalogUnavailable(errors.New("no catalog source configured"))
	}
	cat, err := m.deps.Catalog.Get(ctx, refresh)
	if err != nil {
		return CatalogView{}, errCatalogUnavailable(err)
	}
	entries, err := m.deps.Registry.List()
	if err != nil {
		return CatalogView{}, err
	}
	installed := make(map[string]plugin.Entry, len(entries))
	for _, e := range entries {
		installed[e.ID] = e
	}
	view := CatalogView{SchemaVersion: cat.SchemaVersion, Tags: cat.Tags, Categories: []catalog.Vocab{}, Plugins: []CatalogPlugin{}}
	for _, c := range cat.Categories {
		if c.ID == supportedCategory {
			view.Categories = append(view.Categories, c)
		}
	}
	for _, p := range cat.Plugins {
		if p.Category != supportedCategory {
			continue
		}
		cp := CatalogPlugin{Entry: p, Installable: p.ProtocolVersion == pluginchannel.ProtocolVersion && m.versionAllows(p.MinOpenbeeVersion)}
		if e, ok := installed[p.ID]; ok {
			cp.Installed = true
			cp.InstalledVersion = e.Version
			cp.UpdateAvailable = e.Source == plugin.SourceNpm && e.Package == p.Package && e.Version != p.Version
		}
		view.Plugins = append(view.Plugins, cp)
	}
	return view, nil
}

func (m *Manager) versionAllows(min string) bool {
	if min == "" {
		return true
	}
	cmp, ok := catalog.CompareVersions(m.deps.Version, min)
	return !ok || cmp >= 0
}
