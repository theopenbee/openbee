package chat

import (
	"context"
	"errors"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	resetBodyGap  = 16
	taskListMax   = 256
	taskListGap   = 8
	taskCardPadX  = 12
	taskCardPadY  = 10
	taskCardGap   = 6
	taskCardLines = 2
)

type resetState struct {
	open     bool
	conflict bool
	tasks    []api.ActiveTask
	err      error
}

func (r *resetState) show() {
	*r = resetState{open: true}
}

func (v *View) resetDialog(c *ui.Context, tk theme.Tokens, p pane) {
	r, ch := &p.cs.reset, p.ch
	description := locales.ChatNewConversationConfirm()
	switch {
	case r.conflict:
		description = locales.ChatNewConversationActive()
	case isBee(p.cid):
		description = locales.ChatNewConversationConfirmBee()
	}
	var body func()
	if len(r.tasks) > 0 || r.err != nil {
		body = func() {
			ui.Column(c).Gap(resetBodyGap).Children(func() {
				if len(r.tasks) > 0 {
					activeTaskList(c, tk, r.tasks)
				}
				if r.err != nil {
					kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: locales.ChatNewConversationFailed(), Body: l10n.ErrorText(r.err)})
				}
			})
		}
	}
	kit.Dialog(c, tk, &r.open, kit.DialogOpts{
		Title:       locales.ChatNewConversation(),
		Description: description,
		Locked:      ch.Clearing,
		Footer: func() {
			kit.Button(c, tk, locales.CommonCancel(), kit.ButtonOpts{Variant: kit.Secondary, Disabled: ch.Clearing}).
				OnClick(func() { r.open = false })
			if r.conflict {
				kit.Button(c, tk, locales.ChatStopAndRestart(), kit.ButtonOpts{Variant: kit.Danger, Loading: ch.Clearing}).
					OnClick(func() { v.clear(p, true) })
				return
			}
			kit.Button(c, tk, locales.ChatNewConversation(), kit.ButtonOpts{Variant: kit.Primary, Loading: ch.Clearing}).
				OnClick(func() { v.clear(p, false) })
		},
	}, body)
}

func (v *View) clear(p pane, force bool) {
	r, cs := &p.cs.reset, p.cs
	p.ch.Clear(force, func(tasks []api.ActiveTask, err error) {
		switch {
		case err == nil:
			r.open = false
			cs.list.ScrollToEnd()
		case errors.Is(err, context.Canceled):
			r.open = false
		case tasks != nil:
			r.conflict, r.tasks, r.err = true, tasks, nil
		default:
			r.err = err
		}
	})
}

func activeTaskList(c *ui.Context, tk theme.Tokens, tasks []api.ActiveTask) {
	ui.Scroll(c).MaxHeight(taskListMax).Children(func() {
		ui.Column(c).Gap(taskListGap).Children(func() {
			for i, task := range tasks {
				ui.Column(c).Key(i).AlignItems(ui.Start).Gap(taskCardGap).Padding(taskCardPadY, taskCardPadX).
					Radius(theme.RadiusInset).Border(1, tk.Border).Background(tk.SurfaceMuted).Children(func() {
					ui.Text(c, task.Instruction).FontSize(bodySize).TextColor(tk.Foreground).MaxLines(taskCardLines)
					if label := taskTypeLabel(task.Type); label != "" {
						kit.SmallBadge(c, tk, label, kit.ToneNeutral)
					}
				})
			}
		})
	})
}
