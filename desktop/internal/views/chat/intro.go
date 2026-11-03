package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	introGap       = 20
	introPadY      = 40
	introTextGap   = 8
	introNameSize  = 22
	introWidth     = 448
	introDescLines = 4
)

func workerIntro(c *ui.Context, tk theme.Tokens, w api.Worker) {
	ui.Column(c).AlignItems(ui.Center).Gap(introGap).PaddingY(introPadY).Children(func() {
		kit.WorkerAvatar(c, tk, w.ID, w.Name, w.Status, kit.AvatarXL, true)
		ui.Column(c).AlignItems(ui.Center).Gap(introTextGap).MaxWidth(introWidth).Children(func() {
			ui.Text(c, w.Name).FontSize(introNameSize).FontWeight(mediumWeight).TextColor(tk.Foreground).TextAlign(ui.Center).
				Role(ui.RoleHeading).Level(2)
			if w.Description != "" {
				ui.Text(c, w.Description).FontSize(bodySize).TextColor(tk.TextSecondary).TextAlign(ui.Center).MaxLines(introDescLines)
			}
		})
		ui.Text(c, locales.ChatEmptyWorker(w.Name)).FontSize(bodySize).TextColor(tk.TextMuted).TextAlign(ui.Center).MaxWidth(introWidth)
	})
}
