package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	activityGap    = 12
	activityButton = 4
)

type activityKey struct{}

func isBusy(kind model.StatusKind) bool {
	return kind == model.StatusRunning || kind == model.StatusQueued
}

func (v *View) activityBar(c *ui.Context, tk theme.Tokens, p pane) {
	act := v.activity(p.cid)
	kind, ahead := model.ChatStatus(act, p.worker)
	if !isBusy(kind) {
		return
	}
	run := ""
	if act.State == stateRunning {
		run = act.ExecutionID
	}
	ui.Column(c).Key(activityKey{}).Shrink(0).Background(tk.Background).Children(func() {
		column(c, func() {
			ui.Row(c).AlignItems(ui.Center).Gap(activityGap).Padding(0, 0, activityGap, 0).Children(func() {
				if p.worker == nil {
					kit.BeeAvatar(c, tk, kit.AvatarSM)
				} else {
					kit.WorkerAvatar(c, tk, p.worker.ID, p.worker.Name, p.worker.Status, v.s.AvatarImage(p.worker.Avatar), kit.AvatarSM, false)
				}
				kit.Shimmer(c, tk, statusText(kind, ahead)).Role(ui.RoleStatus)
				if run != "" {
					ui.Box(c).Grow(1)
					kit.Button(c, tk, locales.ExecutionShow(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: "list-tree"}).
						Shrink(0).Margin(-activityButton, 0, -activityButton, 0).OnClick(func() { v.showRun(p.cid, run) })
				}
			})
		})
	})
}

func (v *View) showRun(cid, executionID string) {
	v.nav.Push(route.ChatRun(route.Param(cid), executionID))
}
