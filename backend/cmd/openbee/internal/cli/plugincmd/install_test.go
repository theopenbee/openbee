package plugincmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/plugin"
	"github.com/theopenbee/openbee/internal/plugin/catalog"
)

func testCatalog() *Catalog {
	entry := func(id, version string) CatalogPlugin {
		return CatalogPlugin{Entry: catalog.Entry{
			Category: "channel", ID: id, Package: "@theopenbee/channel-" + id, Version: version,
			Name: plugin.LocalizedText{"en": id + " bot", "zh": id + " 机器人"}, Tags: []string{"im"},
		}, Installable: true}
	}
	tg := entry("telegram", "0.2.0")
	tg.Installed, tg.InstalledVersion, tg.UpdateAvailable = true, "0.1.0", true
	dc := entry("discord", "0.1.0")
	dc.Installed, dc.InstalledVersion = true, "0.1.0"
	future := entry("future", "1.0.0")
	future.Installable = false
	return &Catalog{SchemaVersion: 1, Plugins: []CatalogPlugin{dc, entry("dingtalk", "0.1.0"), future, entry("lark", "0.1.0"), tg}}
}

func staticCatalog(cat *Catalog, err error) func() (*Catalog, error) {
	return func() (*Catalog, error) { return cat, err }
}

func TestResolveSpec(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	rel, err := filepath.Rel(cwd, dir)
	require.NoError(t, err)
	cat := staticCatalog(testCatalog(), nil)

	cases := map[string]string{
		dir:                                  dir,
		rel:                                  dir,
		"lark":                               "@theopenbee/channel-lark@0.1.0",
		"@someone/openbee-channel-x":         "@someone/openbee-channel-x",
		"@theopenbee/channel-wecom@0.1.1":    "@theopenbee/channel-wecom@0.1.1",
		"openbee-channel-matrix@1.0.0":       "openbee-channel-matrix@1.0.0",
		"github:someone/openbee-channel-foo": "github:someone/openbee-channel-foo",
		"npm:openbee-channel-matrix":         "openbee-channel-matrix",
	}
	for arg, want := range cases {
		got, err := resolveSpec(arg, cat, os.Stat)
		require.NoError(t, err, arg)
		assert.Equal(t, want, got, arg)
	}
}

func TestResolveSpec_UnknownBareNameIsRejected(t *testing.T) {
	for _, arg := range []string{"telegramx", "channel-telegram"} {
		_, err := resolveSpec(arg, staticCatalog(testCatalog(), nil), os.Stat)
		assert.ErrorIs(t, err, errNotInCatalog, arg)
	}
}

func TestResolveSpec_CatalogUnavailable(t *testing.T) {
	_, err := resolveSpec("lark", staticCatalog(nil, errors.New("dial tcp: no route")), os.Stat)
	assert.ErrorIs(t, err, errCatalogRequired)
	assert.Contains(t, err.Error(), "no route")
}

func TestResolveSpec_DoesNotLoadCatalogForExplicitSpecs(t *testing.T) {
	calls := 0
	cat := func() (*Catalog, error) { calls++; return nil, errors.New("offline") }
	_, err := resolveSpec("@theopenbee/channel-wecom@0.1.1", cat, os.Stat)
	require.NoError(t, err)
	assert.Zero(t, calls)
}

func TestInstallAll_SequentialWithSummary(t *testing.T) {
	api := newFakeAPI()
	api.failInstall["@theopenbee/channel-dingtalk@0.1.0"] = "npm error ETARGET"
	c := api.client(t)
	var out bytes.Buffer
	err := installAndHint(context.Background(), c, []string{"@theopenbee/channel-lark@0.1.0", "@theopenbee/channel-dingtalk@0.1.0", "@theopenbee/channel-wecom@0.1.1"}, &out, time.Millisecond, time.Second)
	assert.Error(t, err)
	assert.Equal(t, []string{"@theopenbee/channel-lark@0.1.0", "@theopenbee/channel-dingtalk@0.1.0", "@theopenbee/channel-wecom@0.1.1"}, api.installs)
	text := out.String()
	assert.Contains(t, text, "openbee plugin configure lark")
	assert.Contains(t, text, "openbee plugin configure wecom")
	assert.NotContains(t, text, "openbee plugin configure dingtalk")
	assert.Contains(t, text, "ETARGET")
}

func TestWaitOperation_Timeout(t *testing.T) {
	api := newFakeAPI()
	api.ops["slow"] = Operation{ID: "slow", Status: "running"}
	_, err := waitOperation(context.Background(), api.client(t), "slow", time.Millisecond, 20*time.Millisecond)
	assert.ErrorIs(t, err, errOperationTimeout)
}

func runCmd(t *testing.T, api *fakeAPI, p Prompter, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := newCommand(deps{
		Out:        &out,
		Prompter:   p,
		Interval:   time.Millisecond,
		NewClient:  func(string) (*Client, error) { return api.client(t), nil },
		RunSetup:   func(string) (map[string]any, error) { return nil, errors.New("no setup in tests") },
		Getenv:     noEnv,
		OpTimeout:  time.Second,
		RunTimeout: 200 * time.Millisecond,
	})
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

func TestSearch_NonInteractivePrintsTable(t *testing.T) {
	api := newFakeAPI()
	api.catalog = testCatalog()
	out, err := runCmd(t, api, &fakePrompter{}, "search")
	require.NoError(t, err)
	assert.Contains(t, out, "lark")
	assert.Contains(t, out, "lark bot")
	assert.Contains(t, out, "0.1.0 → 0.2.0")
	assert.Empty(t, api.installs)
}

func TestSearch_ShowsCategory(t *testing.T) {
	api := newFakeAPI()
	cat := testCatalog()
	engine := cat.Plugins[0]
	engine.ID, engine.Package, engine.Category = "pi", "@theopenbee/engine-pi", "engine"
	engine.Name = plugin.LocalizedText{"en": "Pi engine"}
	engine.Installed, engine.UpdateAvailable = false, false
	cat.Plugins = append(cat.Plugins, engine)
	api.catalog = cat
	out, err := runCmd(t, api, &fakePrompter{}, "search")
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	assert.Equal(t, []string{"ID", "NAME", "CATEGORY", "VERSION", "STATUS"}, strings.Fields(lines[0]))
	rows := map[string][]string{}
	for _, line := range lines[1:] {
		f := strings.Fields(line)
		rows[f[0]] = f
	}
	require.Contains(t, rows, "pi")
	require.Contains(t, rows, "lark")
	assert.Equal(t, []string{"pi", "Pi", "engine", "engine", "0.1.0", "available"}, rows["pi"])
	assert.Equal(t, []string{"lark", "lark", "bot", "channel", "0.1.0", "available"}, rows["lark"])
}

func TestSearch_KeywordMatchesCategory(t *testing.T) {
	api := newFakeAPI()
	cat := testCatalog()
	engine := cat.Plugins[1]
	engine.ID, engine.Package, engine.Category = "pi", "@theopenbee/pi", "engine"
	engine.Name = plugin.LocalizedText{"en": "Pi"}
	cat.Plugins = append(cat.Plugins, engine)
	api.catalog = cat
	out, err := runCmd(t, api, &fakePrompter{}, "search", "engine")
	require.NoError(t, err)
	assert.Contains(t, out, "pi")
	assert.NotContains(t, out, "lark")
}

func TestSearch_KeywordFilters(t *testing.T) {
	api := newFakeAPI()
	api.catalog = testCatalog()
	out, err := runCmd(t, api, &fakePrompter{}, "search", "LARK")
	require.NoError(t, err)
	assert.Contains(t, out, "lark")
	assert.NotContains(t, out, "dingtalk")
}

func TestSearch_MultiSelectInstallsSelected(t *testing.T) {
	api := newFakeAPI()
	api.catalog = testCatalog()
	p := &fakePrompter{interactive: true, answers: []any{[]int{0, 1}}}
	out, err := runCmd(t, api, p, "search")
	require.NoError(t, err)
	assert.Equal(t, []string{"@theopenbee/channel-dingtalk@0.1.0", "@theopenbee/channel-lark@0.1.0"}, api.installs)
	assert.Contains(t, out, "openbee plugin configure dingtalk")
}

func TestInstallCommand_ResolvesCatalogIDs(t *testing.T) {
	api := newFakeAPI()
	api.catalog = testCatalog()
	_, err := runCmd(t, api, &fakePrompter{}, "install", "lark", "dingtalk")
	require.NoError(t, err)
	assert.Equal(t, []string{"@theopenbee/channel-lark@0.1.0", "@theopenbee/channel-dingtalk@0.1.0"}, api.installs)
}

func TestInstallCommand_CatalogUnavailableSendsNothing(t *testing.T) {
	api := newFakeAPI()
	api.catalogErr = "plugin catalog is unavailable: not published"
	out, err := runCmd(t, api, &fakePrompter{}, "install", "lark")
	require.Error(t, err)
	assert.Contains(t, out+err.Error(), "not published")
	assert.Empty(t, api.installs)
}

func TestUpdate_InstallsOnlyUpdatable(t *testing.T) {
	api := newFakeAPI()
	api.catalog = testCatalog()
	_, err := runCmd(t, api, &fakePrompter{}, "update")
	require.NoError(t, err)
	assert.Equal(t, []string{"@theopenbee/channel-telegram@0.2.0"}, api.installs)
}

func TestUpdate_NothingToUpdate(t *testing.T) {
	api := newFakeAPI()
	api.catalog = testCatalog()
	out, err := runCmd(t, api, &fakePrompter{}, "update", "discord")
	require.NoError(t, err)
	assert.Empty(t, api.installs)
	assert.Contains(t, out, "up to date")
}

func TestResolveSpec_ErrorMessagesAreNotDuplicated(t *testing.T) {
	_, err := resolveSpec("telegramx", staticCatalog(testCatalog(), nil), os.Stat)
	assert.Equal(t, "telegramx is not in the plugin catalog; to install the npm package with that name use npm:telegramx", err.Error())

	_, err = resolveSpec("lark", staticCatalog(nil, errors.New("offline")), os.Stat)
	assert.ErrorIs(t, err, errCatalogRequired)
	assert.Equal(t, "Cannot read the plugin catalog to find lark: offline", err.Error())
}

func TestUpdate_WaitsForEnabledPluginsWithoutConfigureHint(t *testing.T) {
	api := newFakeAPI()
	api.catalog = testCatalog()
	api.addPlugin(Detail{Plugin: Plugin{ID: "telegram", Enabled: true, State: "running"}})
	api.states["telegram"] = []string{"starting", "running"}
	out, err := runCmd(t, api, &fakePrompter{}, "update")
	require.NoError(t, err)
	assert.Equal(t, []string{"@theopenbee/channel-telegram@0.2.0"}, api.installs)
	assert.NotContains(t, out, "openbee plugin configure")
	assert.Contains(t, out, "telegram is running")
}

func TestResolveSpec_PathLikeTypoIsRejected(t *testing.T) {
	_, err := resolveSpec("plugins/my-chan", staticCatalog(testCatalog(), nil), os.Stat)
	assert.ErrorIs(t, err, errNoSuchDirectory)
	got, err := resolveSpec("github:someone/openbee-channel-foo", staticCatalog(testCatalog(), nil), os.Stat)
	require.NoError(t, err)
	assert.Equal(t, "github:someone/openbee-channel-foo", got)
}
