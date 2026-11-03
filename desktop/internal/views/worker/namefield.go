package worker

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const nameRowGap = 8

type nameState struct {
	naming    bool
	exhausted bool
}

func (p *Panel) nameField(c *ui.Context, tk theme.Tokens, name *string, ns *nameState, fail func(error), submit func()) {
	help := locales.WorkersFormNameHelper()
	if ns.exhausted {
		help = locales.WorkersFormRandomNameExhausted()
	}
	kit.Field(c, tk, locales.WorkersFormName(), help, "", func() {
		ui.Row(c).MinWidth(0).AlignItems(ui.End).Gap(nameRowGap).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				kit.TextInput(c, tk, name, locales.WorkersFormName(), locales.WorkersFormNamePlaceholder()).
					AutoFocus().OnSubmit(submit)
			})
			kit.IconButton(c, tk, "shuffle", locales.WorkersFormRandomName(),
				kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeMD, Loading: ns.naming, Disabled: ns.exhausted}).
				Shrink(0).OnClick(func() { p.randomName(name, ns, fail) })
		})
	})
}

func (p *Panel) randomName(name *string, ns *nameState, fail func(error)) {
	if ns.naming || ns.exhausted {
		return
	}
	ns.naming = true
	p.s.RandomName(func(r api.RandomName, err error) {
		ns.naming = false
		if err != nil {
			if err = failure(err); err != nil {
				fail(err)
			}
			return
		}
		ns.exhausted = r.Exhausted
		if !r.Exhausted && r.Name != "" {
			*name = r.Name
		}
	})
}
