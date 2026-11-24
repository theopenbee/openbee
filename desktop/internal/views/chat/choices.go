package chat

import (
	"context"
	"errors"
	"net/http"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	choicesTop    = 8
	choicesGap    = 8
	choiceChecked = "check"
)

func (v *View) choices(c *ui.Context, tk theme.Tokens, p pane, m model.Message) {
	if len(m.Buttons) == 0 {
		return
	}
	pending, inFlight := p.ch.Choosing(m.ID)
	ui.Row(c).Wrap().Gap(choicesGap).Margin(choicesTop, 0, 0, 0).MinWidth(0).MaxWidthPercent(100).Children(func() {
		for i, b := range m.Buttons {
			o := kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM, Wrap: true}
			if b.Style == api.ButtonDanger {
				o.Variant = kit.Danger
			}
			switch {
			case inFlight && pending == i:
				o.Loading = true
			case inFlight:
				o.Disabled = true
			case m.Choice != nil && *m.Choice == i:
				o.Inert, o.Icon = true, choiceChecked
			case m.Choice != nil:
				o.Disabled = true
			}
			el := kit.Button(c, tk, b.Label, o).MinWidth(0).MaxWidthPercent(100)
			switch {
			case o.Inert:
				el.Description(locales.ChatChoiceChosen())
			case !o.Loading && !o.Disabled:
				el.OnClick(func() { v.choose(p, m.ID, i) })
			}
		}
	})
}

func (v *View) choose(p pane, id string, index int) {
	p.cs.list.ScrollToEnd()
	p.ch.Choose(id, index, func(err error) {
		switch {
		case err == nil, errors.Is(err, context.Canceled):
		case api.IsStatus(err, http.StatusConflict):
			v.s.Toast(kit.Toast{Tone: kit.ToneDanger, Title: locales.ChatChoiceTaken()})
		default:
			v.s.Toast(kit.Toast{Tone: kit.ToneDanger, Title: locales.ChatChoiceFailed(), Description: l10n.ErrorText(err)})
		}
	})
}
