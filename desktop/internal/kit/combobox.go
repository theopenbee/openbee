package kit

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

type ComboOption struct {
	Value, Detail string
}

type ComboOpts struct {
	Options   []ComboOption
	Help      string
	Loading   string
	Failed    string
	NoMatches string
	Load      func()
}

const (
	comboDetailGap = 12
	comboStatusGap = 6
	comboSpinner   = 12
)

type (
	comboCaretKey  struct{}
	comboTypingKey struct{}
)

func Combobox(c *ui.Context, tk theme.Tokens, value *string, label, placeholder string, o ComboOpts) ui.Element {
	var in ui.Element
	ui.Column(c).Gap(fieldGap).MinWidth(0).Children(func() {
		in = comboInput(c, tk, value, label, placeholder, o)
	})
	return in
}

func comboInput(c *ui.Context, tk theme.Tokens, value *string, label, placeholder string, o ComboOpts) ui.Element {
	var cb ui.ComboboxParts
	var in ui.Element
	shell := inputShell(c, tk, ui.Row(c).MinHeight(theme.ControlMD).AlignItems(ui.Stretch))
	caretToEnd := ui.Local(shell, comboCaretKey{}, func() bool { return false })
	typing := ui.Local(shell, comboTypingKey{}, func() bool { return false })
	shell.Children(func() {
		cb = ui.ComboboxBase(c, value)
		in = editable(c, cb.Input, label, placeholder).PaddingX(inputPadding).Grow(1)
	})
	if *caretToEnd {
		*caretToEnd = false
		end := utf8.RuneCountInString(*value)
		in.SetTextSelection(end, end)
	}
	if cb.Open() && o.Load != nil {
		o.Load()
	}
	typed := cb.Open() && in.Changed()
	*typing = cb.Open() && (*typing || typed)
	shown := o.Options
	if *typing {
		shown = matchOptions(o.Options, *value)
	}
	if typed && !exactFirst(shown, *value) {
		clearHighlight(cb)
	}
	if cb.Open() && in.Submitted() {
		cb.SetOpen(false)
	}
	w, _ := c.Size()
	popupRadius := theme.Concentric(theme.RadiusPanel, selectPopupPadding)
	itemRadius := theme.Concentric(theme.RadiusControl, selectItemRadiusCut)
	current := *value
	cb.Popup(func(panel ui.Element) {
		floatingPanel(panel, tk, popupRadius).Margin(selectOffset, 0, 0, 0).Padding(selectPopupPadding).
			MaxWidth(w - selectWindowEdge).MaxHeight(selectPopupMaxH).Clip()
		appear(panel, durationStandard, selectPopupRise, popoverScale)
		ui.Scroll(c).Shrink(1).MinHeight(0).Children(func() {
			ui.Column(c).PaddingY(selectViewportPadY).Children(func() {
				for _, opt := range shown {
					comboItem(c, tk, cb, opt, opt.Value == current, itemRadius)
				}
			})
		})
	})
	if v, ok := cb.Chosen(); ok {
		if v != *value {
			*value = v
			in.SetChanged()
		}
		*caretToEnd = true
	}
	note, busy := o.Help, false
	if status := o.status(shown, *typing, *value); cb.Open() && len(shown) == 0 && status != "" {
		note, busy = status, status == o.Loading
	}
	if note != "" {
		comboNote(c, tk, note, busy, cb.Open() && len(shown) > 0)
	}
	return in
}

func (o ComboOpts) status(shown []ComboOption, filtering bool, query string) string {
	switch {
	case o.Loading != "":
		return o.Loading
	case o.Failed != "":
		return o.Failed
	case filtering && strings.TrimSpace(query) != "" && len(o.Options) > 0 && len(shown) == 0:
		return o.NoMatches
	}
	return ""
}

func comboItem(c *ui.Context, tk theme.Tokens, cb ui.ComboboxParts, opt ComboOption, chosen bool, radius float32) {
	item := cb.Item(opt.Value).MinHeight(selectItemHeight).Padding(0, selectItemPadEnd, 0, selectItemPadStart).
		Gap(comboDetailGap).AlignItems(ui.Center).Radius(radius).FontSize(bodyFontSize).TextColor(tk.Foreground)
	item.Background(item.AnimateColor("bg", highlight(tk, item.Highlighted()), instant))
	item.Children(func() {
		ui.Text(c, opt.Value).SingleLine().MinWidth(0)
		if opt.Detail != "" && opt.Detail != opt.Value {
			ui.Text(c, opt.Detail).SingleLine().MinWidth(0).MarginStart(ui.Auto).TextColor(tk.TextMuted)
		}
		if chosen {
			ui.Icon(c, icons.Get("check")).Size(selectChevron, selectChevron).
				Attach(ui.AnchorRight, ui.AnchorRight).Right(selectIndicatorEnd)
		}
	})
}

func comboNote(c *ui.Context, tk theme.Tokens, text string, busy, covered bool) {
	var note ui.Element
	if busy {
		note = ui.Row(c).Gap(comboStatusGap).AlignItems(ui.Center).MinWidth(0)
		note.Children(func() {
			Spinner(c, tk, comboSpinner)
			fieldNote(c, text, tk.TextMuted).MinWidth(0)
		})
	} else {
		note = fieldNote(c, text, tk.TextMuted)
	}
	note.Role(ui.RoleStatus)
	if covered {
		note.Invisible()
	}
}

func clearHighlight(cb ui.ComboboxParts) {
	cb.SetOpen(false)
	cb.SetOpen(true)
}

func exactFirst(shown []ComboOption, text string) bool {
	return len(shown) > 0 && shown[0].Value == strings.TrimSpace(text)
}

func matchOptions(options []ComboOption, query string) []ComboOption {
	typed := strings.TrimSpace(query)
	q := strings.ToLower(typed)
	if q == "" {
		return options
	}
	var exact, folded, starts, contains []ComboOption
	for _, o := range options {
		value, detail := strings.ToLower(o.Value), strings.ToLower(o.Detail)
		switch {
		case o.Value == typed:
			exact = append(exact, o)
		case value == q:
			folded = append(folded, o)
		case strings.HasPrefix(value, q) || strings.HasPrefix(detail, q):
			starts = append(starts, o)
		case strings.Contains(value, q) || strings.Contains(detail, q):
			contains = append(contains, o)
		}
	}
	return slices.Concat(exact, folded, starts, contains)
}
