package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const activityGap = 12

type activityKey struct{}

func isBusy(kind model.StatusKind) bool {
	return kind == model.StatusRunning || kind == model.StatusQueued
}

func (v *View) activityBar(c *ui.Context, tk theme.Tokens, p pane) {
	kind, ahead := model.ChatStatus(v.activity(p.cid), p.worker)
	if !isBusy(kind) {
		return
	}
	ui.Column(c).Key(activityKey{}).Shrink(0).Background(tk.Background).Role(ui.RoleStatus).Children(func() {
		column(c, func() {
			ui.Row(c).AlignItems(ui.Center).Gap(activityGap).Padding(0, 0, activityGap, 0).Children(func() {
				if p.worker == nil {
					kit.BeeAvatar(c, tk, kit.AvatarSM)
				} else {
					kit.WorkerAvatar(c, tk, p.worker.ID, p.worker.Name, p.worker.Status, kit.AvatarSM, true)
				}
				kit.Shimmer(c, tk, statusText(kind, ahead))
			})
		})
	})
}
