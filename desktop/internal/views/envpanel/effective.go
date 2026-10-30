package envpanel

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	effectiveHeaderTop    = 20
	effectiveHeaderBottom = 16
	effectivePadX         = 24
	effectiveBodyBottom   = 8
	effectiveRowPadY      = 12
	effectiveRowGap       = 4
	effectiveRowColumnGap = 16
	effectiveSkeletonGap  = 12
	effectiveSkeletonH    = 20
	effectiveNoteBottom   = 16
	sourceSize            = 12
)

type effectiveKey struct{}

func Effective(c *ui.Context, s *store.Store, workerID string) {
	tk := theme.Apply(c)
	global, worker := s.Envs("global", ""), s.Envs("worker", workerID)
	global.Ensure()
	worker.Ensure()
	loading := awaiting(global) || awaiting(worker)
	rows := model.EffectiveEnv(global.Data, worker.Data)
	ui.Column(c).Key(effectiveKey{}).MinWidth(0).Clip().Radius(theme.RadiusSurface).Background(tk.Surface).Border(cardBorder, tk.Border).Children(func() {
		ui.Column(c).Padding(effectiveHeaderTop, effectivePadX, effectiveHeaderBottom, effectivePadX).Children(func() {
			ui.Text(c, locales.EnvConfigEffectiveTitle()).FontSize(titleSize).FontWeight(mediumWeight).SingleLine().Role(ui.RoleHeading).Level(2)
			ui.Text(c, locales.WorkerDetailEffectiveEnvDescription()).TextColor(tk.TextSecondary).Margin(descriptionGap, 0, 0, 0)
		})
		ui.Column(c).Padding(0, 0, effectiveBodyBottom, 0).Children(func() {
			switch {
			case loading:
				ui.Column(c).Gap(effectiveSkeletonGap).Padding(0, effectivePadX, effectiveNoteBottom, effectivePadX).Children(func() {
					for i := range skeletonRows {
						kit.SkeletonBar(c, tk, 0, effectiveSkeletonH).Key(i)
					}
				})
			case len(rows) == 0:
				ui.Text(c, locales.EnvConfigNoEffective()).TextColor(tk.TextSecondary).Padding(0, effectivePadX, effectiveNoteBottom, effectivePadX)
			default:
				for _, row := range rows {
					effectiveRow(c, tk, row)
				}
			}
		})
	})
}

func awaiting(r *store.Resource[[]api.EnvConfig]) bool {
	return !r.Loaded && r.Err == nil
}

func effectiveRow(c *ui.Context, tk theme.Tokens, v model.EffectiveVar) {
	ui.Column(c).Key(v.Key).Gap(effectiveRowGap).Padding(effectiveRowPadY, effectivePadX).BorderWidth(1, 0, 0, 0).BorderColor(tk.BorderSubtle).Children(func() {
		ui.Row(c).AlignItems(ui.Baseline).Gap(effectiveRowColumnGap).Children(func() {
			ui.Text(c, v.Key).Font(theme.FontMono).FontSize(keySize).TextColor(tk.Foreground).Grow(1).MinWidth(0)
			source := ui.Text(c, sourceLabel(v.Source)).FontSize(sourceSize).SingleLine().Shrink(0)
			if v.Source == "worker" {
				source.FontWeight(mediumWeight).TextColor(tk.Foreground)
			} else {
				source.TextColor(tk.TextSecondary)
			}
		})
		ui.Text(c, v.Masked).Font(theme.FontMono).FontSize(sourceSize).TextColor(tk.TextSecondary)
	})
}

func sourceLabel(source string) string {
	if source == "worker" {
		return locales.EnvConfigSourceWorker()
	}
	return locales.EnvConfigSourceGlobal()
}
