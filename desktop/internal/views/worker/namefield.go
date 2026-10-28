package worker

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const nameRowGap = 8

func (p *Panel) nameField(c *ui.Context, tk theme.Tokens, es *editState, submit func()) {
	help := locales.WorkersFormNameHelper()
	if es.exhausted {
		help = locales.WorkersFormRandomNameExhausted()
	}
	kit.Field(c, tk, locales.WorkersFormName(), help, "", func() {
		ui.Row(c).MinWidth(0).AlignItems(ui.End).Gap(nameRowGap).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				kit.TextInput(c, tk, &es.form.Name, locales.WorkersFormName(), locales.WorkersFormNamePlaceholder()).
					AutoFocus().OnSubmit(submit)
			})
			kit.IconButton(c, tk, "shuffle", locales.WorkersFormRandomName(),
				kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD, Loading: es.naming, Disabled: es.exhausted}).
				Shrink(0).OnClick(func() { p.randomName(es) })
		})
	})
}

func (p *Panel) randomName(es *editState) {
	if es.naming || es.exhausted {
		return
	}
	es.naming = true
	p.s.RandomName(func(r api.RandomName, err error) {
		es.naming = false
		if err != nil {
			if err = failure(err); err != nil {
				es.err = err
			}
			return
		}
		es.exhausted = r.Exhausted
		if !r.Exhausted && r.Name != "" {
			es.form.Name = r.Name
		}
	})
}
