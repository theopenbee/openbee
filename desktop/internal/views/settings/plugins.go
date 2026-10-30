package settings

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	tabMarket = iota
	tabInstalled
)

const (
	containerXL   = 576
	container3XL  = 768
	container4XL  = 896
	pluginsGap    = 24
	pluginsBarGap = 12
	textSize      = 14
	metaSize      = 12
	progressGap   = 8
	progressGlyph = 16
	noValue       = "–"
	versionPrefix = "v"
	stateNoConfig = "not_configured"
	opSucceeded   = "succeeded"
	opFailed      = "failed"
)

type pluginsState struct {
	install installState
}

type pluginsKey struct{}

func (d *Dialog) plugins(c *ui.Context, tk theme.Tokens) {
	if d.pluginID != "" {
		d.pluginDetail(c, tk, d.pluginID)
		return
	}
	p := d.s.Plugins()
	p.List.Ensure()
	p.Ops.Ensure()
	root := ui.Column(c).Key(SectionPlugins).MinWidth(0).Gap(pluginsGap)
	st := ui.Local(root, pluginsKey{}, func() pluginsState { return pluginsState{} })
	root.Children(func() {
		d.pluginsBar(c, tk, st)
		ui.Column(c).Key(d.pluginsTab).MinWidth(0).Children(func() {
			if d.pluginsTab == tabInstalled {
				d.installedTab(c, tk)
			} else {
				d.marketTab(c, tk)
			}
		})
		d.installDialog(c, tk, &st.install)
	})
}

func (d *Dialog) pluginsBar(c *ui.Context, tk theme.Tokens, st *pluginsState) {
	list := d.s.Plugins().List
	count := ""
	if list.Loaded {
		count = strconv.Itoa(len(list.Data))
	}
	ui.Row(c).MinWidth(0).Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(pluginsBarGap).Children(func() {
		kit.CountedTabs(c, tk, &d.pluginsTab, locales.PluginsTitle(), []kit.TabItem{
			{Text: locales.PluginsTabsMarket()},
			{Text: locales.PluginsTabsInstalled(), Count: count},
		})
		kit.Button(c, tk, locales.PluginsInstallDialogOpen(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM, Icon: "package-plus"}).
			OnClick(st.install.show)
	})
}

func typeName(t string, categories []api.CatalogTag) string {
	if name := model.CategoryName(categories, t, i18n.Locale()); name != "" {
		return name
	}
	switch t {
	case model.PluginChannel:
		return locales.PluginsTypesChannel()
	case model.PluginEngine:
		return locales.PluginsTypesEngine()
	}
	return t
}

func (d *Dialog) showPlugins(tab int) {
	d.choose(SectionPlugins)
	d.pluginsTab = tab
}

func displayName(names api.LocalizedText, id string) string {
	if name := l10n.Localized(names, i18n.Locale()); name != "" {
		return name
	}
	return id
}

func operationProgress(c *ui.Context, tk theme.Tokens, label string, started time.Time) ui.Element {
	now := c.Now()
	c.After(untilNextSecond(started, now))
	return ui.Row(c).MinHeight(theme.ControlSM).AlignItems(ui.Center).Gap(progressGap).Role(ui.RoleStatus).Children(func() {
		kit.Spinner(c, tk, progressGlyph).TextColor(tk.Accent)
		kit.Shimmer(c, tk, label)
		ui.Text(c, model.Elapsed(started, now)).FontSize(textSize).FontFeatures("tnum").TextColor(tk.TextSecondary).SingleLine()
	})
}

func untilNextSecond(started, now time.Time) time.Duration {
	if started.IsZero() || now.Before(started) {
		return time.Second
	}
	return time.Second - now.Sub(started)%time.Second
}

func opPending(op api.PluginOperation) bool {
	return model.OperationsPending([]api.PluginOperation{op})
}

func failure(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (d *Dialog) toastError(err error) {
	if err = failure(err); err != nil {
		d.s.Toast(kit.Toast{Tone: kit.ToneDanger, Title: l10n.ErrorText(err)})
	}
}
