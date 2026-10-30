package worker

import (
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	headerGap      = 16
	identityGap    = 6
	nameSize       = 18
	engineGlyph    = 14
	engineGap      = 6
	menuGlyph      = 16
	menuWidth      = 192
	menuItemHeight = 36
	menuItemPadX   = 11
	menuItemGap    = 10
	menuItemInset  = 6
	dangerTint     = 0.08
	skeletonFace   = 48
	skeletonGap    = 8
	skeletonBlocks = 12
	nameBarW       = 128
	nameBarH       = 20
	engineBarW     = 160
	engineBarH     = 16
	lineBarH       = 16
	controlBarW    = 256
	controlBarH    = 40
	smallBlockH    = 96
	largeBlockH    = 160
)

func (p *Panel) header(c *ui.Context, tk theme.Tokens, st *panelState, w api.Worker) {
	ui.Column(c).MinWidth(0).Gap(headerGap).Children(func() {
		ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(headerGap).Children(func() {
			kit.WorkerAvatar(c, tk, w.ID, w.Name, w.Status, kit.AvatarLG, true)
			ui.Column(c).Grow(1).MinWidth(0).Gap(identityGap).Children(func() {
				ui.Text(c, w.Name).FontSize(nameSize).FontWeight(mediumWeight).SingleLine().MaxWidthPercent(100).
					Role(ui.RoleHeading).Level(3)
				if w.Engine != "" {
					engineLine(c, tk, w.Engine, l10n.EngineName(model.FindEngine(p.s.EnabledEngines(), w.Engine)))
				}
			})
		})
		description := w.Description
		if description == "" {
			description = locales.CommonNoDescription()
		}
		ui.Text(c, description).TextColor(tk.TextSecondary)
		ui.Row(c).Wrap().AlignItems(ui.Center).Gap(buttonsGap).Children(func() {
			kit.Button(c, tk, locales.WorkerDetailEditInfo(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM, Icon: "pencil", Disabled: st.editing}).
				OnClick(func() { st.editing = true })
			p.moreMenu(c, tk, st, w)
		})
	})
}

func engineLine(c *ui.Context, tk theme.Tokens, engine, name string) {
	ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(engineGap).TextColor(tk.TextSecondary).Children(func() {
		ui.Icon(c, icons.Engine(engine)).Size(engineGlyph, engineGlyph).Shrink(0)
		ui.Text(c, name).SingleLine().MinWidth(0)
	})
}

func (p *Panel) moreMenu(c *ui.Context, tk theme.Tokens, st *panelState, w api.Worker) {
	b := ui.ButtonBase(c).Label(locales.WorkersRowActions(w.Name)).Role(ui.RoleMenuButton).Expanded(st.menuOpen).
		Size(theme.ControlSM, theme.ControlSM).Center().Radius(theme.RadiusControl)
	bg, fg := tk.SurfaceMuted.Alpha(0), tk.TextSecondary
	if b.Hovered() || b.Pressed() {
		bg, fg = tk.SurfaceMuted, tk.Foreground
	} else if st.menuOpen {
		bg = tk.SurfaceMuted
	}
	b.Background(b.AnimateColor("bg", bg, fast)).TextColor(b.AnimateColor("fg", fg, fast)).
		OnClick(func() { st.menuOpen = !st.menuOpen }).Children(func() {
		ui.Icon(c, icons.Get("ellipsis")).Size(menuGlyph, menuGlyph)
	})
	kit.Popover(c, tk, b, &st.menuOpen, menuWidth, func() {
		ui.Column(c).FocusGroup(ui.Vertical).Role(ui.RoleMenu).Children(func() {
			item := ui.ButtonBase(c).Label(locales.WorkersDeleteWorker()).Role(ui.RoleMenuItem).MinHeight(menuItemHeight).
				PaddingX(menuItemPadX).Gap(menuItemGap).AlignItems(ui.Center).Justify(ui.Start).
				Radius(theme.Concentric(theme.RadiusPanel, menuItemInset)).TextColor(tk.Danger)
			tint := tk.SurfaceRaised.Mix(tk.Danger, dangerTint)
			bg := tint.Alpha(0)
			if item.Hovered() || item.Pressed() || item.FocusVisible() {
				bg = tint
			}
			item.Background(item.AnimateColor("bg", bg, fast)).FocusRing(false).OnClick(func() {
				st.menuOpen = false
				st.del.show(w)
			}).Children(func() {
				ui.Icon(c, icons.Get("trash-2")).Size(menuGlyph, menuGlyph).Shrink(0)
				ui.Text(c, locales.WorkersDeleteWorker()).SingleLine()
			})
		})
	})
}

func skeleton(c *ui.Context, tk theme.Tokens) {
	ui.Column(c).MinWidth(0).Gap(sectionGap).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(headerGap).Children(func() {
			kit.SkeletonCircle(c, tk, skeletonFace)
			ui.Column(c).Grow(1).Gap(skeletonGap).Children(func() {
				kit.SkeletonBar(c, tk, nameBarW, nameBarH)
				kit.SkeletonBar(c, tk, engineBarW, engineBarH)
			})
		})
		kit.SkeletonBar(c, tk, 0, lineBarH)
		kit.SkeletonBar(c, tk, controlBarW, controlBarH)
		ui.Column(c).Gap(skeletonBlocks).Children(func() {
			kit.SkeletonBar(c, tk, 0, smallBlockH).Radius(theme.RadiusInset)
			kit.SkeletonBar(c, tk, 0, largeBlockH).Radius(theme.RadiusInset)
		})
	})
}
