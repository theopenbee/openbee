package pluginmgr

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/apperr"
	"github.com/theopenbee/openbee/internal/plugin"
	"github.com/theopenbee/openbee/internal/plugin/catalog"
)

type fakeCatalog struct {
	cat     catalog.Catalog
	err     error
	refresh bool
}

func (f *fakeCatalog) Get(_ context.Context, refresh bool) (catalog.Catalog, error) {
	f.refresh = refresh
	return f.cat, f.err
}

func catalogEntry(id, pkg, version string) catalog.Entry {
	return catalog.Entry{Category: "channel", ID: id, Package: pkg, Version: version, ProtocolVersion: 1, Name: plugin.LocalizedText{"en": id}}
}

func TestManager_CatalogMarksInstalledAndUpdates(t *testing.T) {
	h := newHarness(t)
	h.installNpm("telegram", "0.1.0")
	h.installNpm("discord", "0.2.0")
	h.install("lark")
	src := &fakeCatalog{cat: catalog.Catalog{
		SchemaVersion: 1,
		Categories:    []catalog.Vocab{{ID: "channel"}, {ID: "agent"}},
		Plugins: []catalog.Entry{
			catalogEntry("telegram", "@theopenbee/channel-telegram", "0.2.0"),
			catalogEntry("discord", "@theopenbee/channel-discord", "0.2.0"),
			catalogEntry("lark", "@theopenbee/channel-lark", "0.1.0"),
			catalogEntry("wecom", "@theopenbee/channel-wecom", "0.1.1"),
			{Category: "agent", ID: "coder", Package: "@theopenbee/agent-coder", Version: "1.0.0", ProtocolVersion: 1},
			func() catalog.Entry {
				e := catalogEntry("future", "@theopenbee/channel-future", "1.0.0")
				e.ProtocolVersion = 2
				return e
			}(),
			func() catalog.Entry {
				e := catalogEntry("newer", "@theopenbee/channel-newer", "1.0.0")
				e.MinOpenbeeVersion = "0.9.0"
				return e
			}(),
		},
	}}
	h.m.deps.Catalog = src
	h.m.deps.Version = "0.1.0"

	view, err := h.m.Catalog(context.Background(), true)
	require.NoError(t, err)
	assert.True(t, src.refresh)
	assert.Equal(t, []catalog.Vocab{{ID: "channel"}}, view.Categories)

	byID := map[string]CatalogPlugin{}
	for _, p := range view.Plugins {
		byID[p.ID] = p
	}
	assert.NotContains(t, byID, "coder")

	assert.True(t, byID["telegram"].Installed)
	assert.Equal(t, "0.1.0", byID["telegram"].InstalledVersion)
	assert.True(t, byID["telegram"].UpdateAvailable)
	assert.True(t, byID["telegram"].Installable)

	assert.True(t, byID["discord"].Installed)
	assert.False(t, byID["discord"].UpdateAvailable)

	assert.True(t, byID["lark"].Installed)
	assert.False(t, byID["lark"].UpdateAvailable)

	assert.False(t, byID["wecom"].Installed)
	assert.True(t, byID["wecom"].Installable)

	assert.False(t, byID["future"].Installable)
	assert.False(t, byID["newer"].Installable)
}

func TestManager_CatalogDevVersionIgnoresMinVersion(t *testing.T) {
	h := newHarness(t)
	e := catalogEntry("newer", "@theopenbee/channel-newer", "1.0.0")
	e.MinOpenbeeVersion = "9.0.0"
	h.m.deps.Catalog = &fakeCatalog{cat: catalog.Catalog{SchemaVersion: 1, Plugins: []catalog.Entry{e}}}
	h.m.deps.Version = "dev"
	view, err := h.m.Catalog(context.Background(), false)
	require.NoError(t, err)
	assert.True(t, view.Plugins[0].Installable)
}

func TestManager_CatalogUnavailable(t *testing.T) {
	h := newHarness(t)
	h.m.deps.Catalog = &fakeCatalog{err: errors.New("dial tcp: no route")}
	_, err := h.m.Catalog(context.Background(), false)
	assert.Equal(t, CodeCatalogUnavailable, apperr.Code(err))
}

func TestManager_CatalogDoesNotOfferDowngrade(t *testing.T) {
	h := newHarness(t)
	h.installNpm("telegram", "0.3.0")
	h.m.deps.Catalog = &fakeCatalog{cat: catalog.Catalog{SchemaVersion: 1, Plugins: []catalog.Entry{
		catalogEntry("telegram", "@theopenbee/channel-telegram", "0.2.0"),
	}}}
	view, err := h.m.Catalog(context.Background(), false)
	require.NoError(t, err)
	assert.True(t, view.Plugins[0].Installed)
	assert.False(t, view.Plugins[0].UpdateAvailable)
}

func TestCatalog_IncludesEngineCategory(t *testing.T) {
	h := newHarness(t)
	engine := catalog.Entry{Category: "engine", ID: "claude", Package: "@theopenbee/engine-claude", Version: "0.1.0", ProtocolVersion: 1, Name: plugin.LocalizedText{"en": "Claude Code"}}
	stale := catalog.Entry{Category: "engine", ID: "oldengine", Package: "@theopenbee/engine-old", Version: "0.1.0", ProtocolVersion: 2, Name: plugin.LocalizedText{"en": "Old"}}
	h.m.deps.Catalog = &fakeCatalog{cat: catalog.Catalog{
		SchemaVersion: 1,
		Categories:    []catalog.Vocab{{ID: "channel"}, {ID: "engine"}, {ID: "agent"}},
		Plugins:       []catalog.Entry{catalogEntry("telegram", "@theopenbee/channel-telegram", "0.1.0"), engine, stale},
	}}

	view, err := h.m.Catalog(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, []catalog.Vocab{{ID: "channel"}, {ID: "engine"}}, view.Categories)

	byID := map[string]CatalogPlugin{}
	for _, p := range view.Plugins {
		byID[p.ID] = p
	}
	require.Contains(t, byID, "claude")
	assert.Equal(t, "engine", byID["claude"].Category)
	assert.True(t, byID["claude"].Installable)
	assert.True(t, byID["telegram"].Installable)
	require.Contains(t, byID, "oldengine")
	assert.False(t, byID["oldengine"].Installable)
}

func TestCatalog_SkipsUnknownCategory(t *testing.T) {
	h := newHarness(t)
	h.m.deps.Catalog = &fakeCatalog{cat: catalog.Catalog{
		SchemaVersion: 1,
		Categories:    []catalog.Vocab{{ID: "channel"}, {ID: "agent"}, {ID: "theme"}},
		Plugins: []catalog.Entry{
			catalogEntry("telegram", "@theopenbee/channel-telegram", "0.1.0"),
			{Category: "agent", ID: "coder", Package: "@theopenbee/agent-coder", Version: "1.0.0", ProtocolVersion: 1},
			{Category: "theme", ID: "dark", Package: "@theopenbee/theme-dark", Version: "1.0.0", ProtocolVersion: 1},
			{ID: "nocat", Package: "@theopenbee/nocat", Version: "1.0.0", ProtocolVersion: 1},
		},
	}}

	view, err := h.m.Catalog(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, []catalog.Vocab{{ID: "channel"}}, view.Categories)
	require.Len(t, view.Plugins, 1)
	assert.Equal(t, "telegram", view.Plugins[0].ID)
}
