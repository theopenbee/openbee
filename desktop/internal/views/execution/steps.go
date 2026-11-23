package execution

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	stepsGap       = 20
	stepGap        = 12
	stepBox        = 28
	stepGlyph      = 14
	stepLineTop    = 32
	stepLineLeft   = (stepBox - 1) / 2.0
	stepBodyTop    = 4
	stepTextTop    = 2
	stepLabelGap   = 4
	toolGap        = 6
	toolHeaderGap  = 8
	trailingGap    = 4
	textDot        = 6
	chipPadX       = 12
	chipPadY       = 6
	quoteBar       = 2
	quotePad       = 12
	messageLines   = 2
	thinkingLines  = 3
	clipSlack      = 0.5
	outputMaxH     = 224
	outputSize     = 11
	outputLeading  = 1.625
	outputPadX     = 12
	outputPadY     = 10
	failurePadX    = 12
	failurePadY    = 8
	dangerFill     = 0.10
	dangerEdge     = 0.30
	dangerEdgeSoft = 0.25
)

var toolIcons = map[string]string{
	"command":         "terminal",
	"read":            "file-text",
	"edit":            "file-pen-line",
	"search":          "search",
	"web":             "globe",
	model.ToolMessage: "send",
	model.ToolOther:   "wrench",
}

type stepTone int

const (
	toneQuiet stepTone = iota
	toneActive
	toneDanger
)

type openKey struct{}

func toolLabel(kind, name string) string {
	switch kind {
	case "command":
		return locales.ExecutionToolCommand()
	case "read":
		return locales.ExecutionToolRead()
	case "edit":
		return locales.ExecutionToolEdit()
	case "search":
		return locales.ExecutionToolSearch()
	case "web":
		return locales.ExecutionToolWeb()
	case model.ToolMessage:
		return locales.ExecutionToolMessage()
	}
	return locales.ExecutionToolOther(name)
}

func (p *Panel) timeline(c *ui.Context, tk theme.Tokens, steps []api.ExecutionStep, running bool, failure string) {
	col := ui.Column(c).MinWidth(0).Gap(stepsGap).Role(ui.RoleList)
	open := ui.Local(col, openKey{}, func() map[int]bool { return map[int]bool{} })
	col.Children(func() {
		for i, step := range steps {
			last := i == len(steps)-1 && failure == ""
			stepItem(c, tk, last, func() {
				switch step.Type {
				case model.StepThinking:
					thinkingStep(c, tk, step.Text, *open, i)
				case model.StepText:
					p.textStep(c, tk, step.Text)
				case model.StepError:
					errorStep(c, tk, step.Text)
				case model.StepTool:
					toolStep(c, tk, step, running, *open, i)
				}
			})
		}
		if failure != "" {
			stepItem(c, tk, true, func() { failureStep(c, tk, failure) })
		}
	})
}

func stepItem(c *ui.Context, tk theme.Tokens, last bool, body func()) {
	ui.Row(c).MinWidth(0).AlignItems(ui.Start).Gap(stepGap).Role(ui.RoleListItem).Children(func() {
		if !last {
			ui.Box(c).Absolute().Top(stepLineTop).Bottom(-stepsGap).Left(stepLineLeft).Width(1).Background(tk.Border)
		}
		body()
	})
}

func stepIcon(c *ui.Context, tk theme.Tokens, tone stepTone, glyph func()) {
	bg, border, fg := tk.Surface, tk.Border, tk.TextSecondary
	switch tone {
	case toneActive:
		border, fg = tk.BorderStrong, tk.Foreground
	case toneDanger:
		bg, border, fg = tk.Surface.Mix(tk.Danger, dangerFill), tk.Border.Mix(tk.Danger, dangerEdge), tk.Danger
	}
	ui.Box(c).Size(stepBox, stepBox).Shrink(0).Center().Radius(theme.RadiusPill).Background(bg).Border(1, border).
		TextColor(fg).Children(glyph)
}

func glyph(c *ui.Context, name string) func() {
	return func() { ui.Icon(c, icons.Get(name)).Size(stepGlyph, stepGlyph) }
}

func stepBody(c *ui.Context, gap, top float32) ui.Element {
	return ui.Column(c).Grow(1).MinWidth(0).Gap(gap).Padding(top, 0, 0, 0)
}

func thinkingStep(c *ui.Context, tk theme.Tokens, text string, open map[int]bool, index int) {
	expanded := open[index]
	stepIcon(c, tk, toneQuiet, glyph(c, "brain"))
	stepBody(c, stepLabelGap, stepBodyTop).Children(func() {
		ui.Text(c, locales.ExecutionThinking()).FontSize(metaSize).TextColor(tk.TextMuted)
		body := ui.Text(c, text).FontSize(textSize).TextColor(tk.TextSecondary)
		if !expanded {
			body.MaxLines(thinkingLines)
		}
		if expanded || clipsLines(c, body, text, thinkingLines) {
			expandToggle(c, tk, expanded, func() { open[index] = !expanded })
		}
	})
}

func clipsLines(c *ui.Context, e ui.Element, text string, lines int) bool {
	decided := false
	if w := e.LayoutBounds().W; w > 0 {
		decided = overLines(c.MeasureText, w, text, lines)
	}
	services := c.Services()
	e.Draw(func(p *ui.Painter, r ui.Rect) {
		if overLines(p.MeasureText, r.W, text, lines) != decided {
			services.Invalidate()
		}
	})
	return decided
}

func overLines(measure func(float32, ...ui.Span) (float32, float32), width float32, text string, lines int) bool {
	_, full := measure(width, ui.Span{Text: text, Size: textSize})
	_, limit := measure(width, ui.Span{Text: strings.Repeat("M\n", lines-1) + "M", Size: textSize})
	return full > limit+clipSlack
}

func expandToggle(c *ui.Context, tk theme.Tokens, expanded bool, toggle func()) {
	label := locales.ArcExpandableTextMore()
	turn := float32(0)
	if expanded {
		label, turn = locales.ArcExpandableTextLess(), 180
	}
	b := ui.ButtonBase(c).Label(label).Expanded(expanded).AlignSelf(ui.Start).AlignItems(ui.Center).Gap(trailingGap).
		Radius(theme.RadiusControl).FontSize(textSize).FontWeight(mediumWeight)
	fg := tk.Foreground
	if b.Hovered() || b.Pressed() {
		fg = tk.TextSecondary
	}
	b.TextColor(fg).OnClick(toggle).Children(func() {
		ui.Text(c, label).SingleLine()
		chevron := ui.Icon(c, icons.Get("chevron-down")).Size(stepGlyph, stepGlyph).TextColor(tk.TextMuted)
		chevron.Rotate(chevron.AnimateTo("turn", turn, ui.Snappy))
	})
}

func (p *Panel) textStep(c *ui.Context, tk theme.Tokens, text string) {
	stepIcon(c, tk, toneActive, func() {
		ui.Box(c).Size(textDot, textDot).Radius(theme.RadiusPill).Background(tk.Foreground)
	})
	stepBody(c, 0, stepTextTop).FontSize(textSize).TextColor(tk.Foreground).Children(func() {
		if p.prose != nil {
			p.prose(c, text)
			return
		}
		ui.Text(c, text).Selectable()
	})
}

func errorStep(c *ui.Context, tk theme.Tokens, text string) {
	stepIcon(c, tk, toneDanger, glyph(c, "circle-alert"))
	stepBody(c, 0, stepBodyTop).Children(func() {
		ui.Text(c, text).FontSize(textSize).TextColor(tk.Danger).Selectable()
	})
}

func toolStep(c *ui.Context, tk theme.Tokens, step api.ExecutionStep, running bool, open map[int]bool, index int) {
	kind := model.ToolKind(step.Kind)
	active := running && step.Status == model.ToolRunning
	failed := step.Status == model.ToolError
	label := toolLabel(kind, step.Name)
	expanded := open[index] && step.Output != ""
	tone := toneQuiet
	switch {
	case failed:
		tone = toneDanger
	case active:
		tone = toneActive
	}
	stepIcon(c, tk, tone, func() {
		if active {
			kit.Spinner(c, tk, stepGlyph).TextColor(tk.Foreground).Label(locales.ExecutionStatusRunning())
			return
		}
		glyph(c, toolIcons[kind])()
	})
	stepBody(c, toolGap, stepBodyTop).Children(func() {
		header := func() {
			if active {
				kit.Shimmer(c, tk, label).Shrink(1)
			} else {
				ui.Text(c, label).FontSize(textSize).FontWeight(mediumWeight).TextColor(tk.Foreground).SingleLine().MinWidth(0)
			}
			toolMark(c, tk, step.Status, active)
			ui.Box(c).Grow(1)
			toolTrailing(c, tk, step, expanded)
		}
		if step.Output != "" {
			ui.ButtonBase(c).Label(toolName(label, step.Status, active)).Expanded(expanded).MinWidth(0).AlignItems(ui.Center).Justify(ui.Start).
				Gap(toolHeaderGap).Radius(theme.RadiusControl).OnClick(func() { open[index] = !open[index] }).Children(header)
		} else {
			ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(toolHeaderGap).Children(header)
		}
		toolSummary(c, tk, kind, step.Summary)
		if expanded {
			toolOutput(c, tk, step.Output, failed)
		}
	})
}

func toolName(label, status string, active bool) string {
	switch {
	case status == model.ToolError:
		return label + runJoin + locales.ExecutionStatusFailed()
	case status == model.ToolDone || active:
		return label
	}
	return label + runJoin + locales.ExecutionToolStopped()
}

func toolMark(c *ui.Context, tk theme.Tokens, status string, active bool) {
	switch {
	case status == model.ToolError:
		ui.Icon(c, icons.Get("x")).Size(stepGlyph, stepGlyph).Shrink(0).TextColor(tk.Danger)
	case status == model.ToolDone:
		ui.Icon(c, icons.Get("check")).Size(stepGlyph, stepGlyph).Shrink(0).TextColor(tk.Success)
	case active:
	default:
		stopped := locales.ExecutionToolStopped()
		mark := ui.Icon(c, icons.Get("circle-slash")).Size(stepGlyph, stepGlyph).Shrink(0).TextColor(tk.TextMuted).
			Label(stopped).Role(ui.RoleImage)
		kit.Tooltip(c, tk, mark, stopped)
	}
}

func toolTrailing(c *ui.Context, tk theme.Tokens, step api.ExecutionStep, expanded bool) {
	ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(trailingGap).FontSize(metaSize).TextColor(tk.TextMuted).
		FontFeatures("tnum").Children(func() {
		if model.ToolDurationShown(step.DurationMS) {
			ui.Text(c, Duration(step.DurationMS)).SingleLine()
		}
		if step.Output != "" {
			turn := float32(0)
			if expanded {
				turn = 180
			}
			chevron := ui.Icon(c, icons.Get("chevron-down")).Size(stepGlyph, stepGlyph)
			chevron.Rotate(chevron.AnimateTo("turn", turn, ui.Snappy))
		}
	})
}

func toolSummary(c *ui.Context, tk theme.Tokens, kind, summary string) {
	if summary == "" {
		return
	}
	if kind == model.ToolMessage {
		ui.Column(c).MinWidth(0).BorderWidth(0, 0, 0, quoteBar).BorderColor(tk.Border).PaddingStart(quotePad).Children(func() {
			ui.Text(c, summary).FontSize(textSize).TextColor(tk.TextSecondary).MaxLines(messageLines)
		})
		return
	}
	chip := ui.Row(c).MinWidth(0).Radius(theme.RadiusControl).Background(tk.SurfaceMuted).Padding(chipPadY, chipPadX).Children(func() {
		ui.Text(c, summary).Font(theme.FontMono).FontSize(metaSize).TextColor(tk.TextSecondary).SingleLine().Grow(1).MinWidth(0)
	})
	kit.Tooltip(c, tk, chip, summary)
}

func toolOutput(c *ui.Context, tk theme.Tokens, output string, failed bool) {
	fg, border := tk.TextSecondary, tk.Border
	if failed {
		fg, border = tk.Danger, tk.Border.Mix(tk.Danger, dangerEdgeSoft)
	}
	ui.Column(c).MinWidth(0).MaxHeight(outputMaxH).Radius(theme.RadiusControl).Border(1, border).Clip().Children(func() {
		ui.Scroll(c).Shrink(1).MinHeight(0).Children(func() {
			ui.Column(c).MinWidth(0).Padding(outputPadY, outputPadX).Children(func() {
				ui.Text(c, output).Font(theme.FontMono).FontSize(outputSize).LineHeight(outputLeading).TextColor(fg).Selectable()
			})
		})
	})
}

func failureStep(c *ui.Context, tk theme.Tokens, message string) {
	stepIcon(c, tk, toneDanger, glyph(c, "circle-alert"))
	stepBody(c, toolGap, stepBodyTop).Children(func() {
		ui.Text(c, locales.ExecutionFailedTitle()).FontSize(textSize).FontWeight(mediumWeight).TextColor(tk.Danger)
		ui.Column(c).MinWidth(0).Radius(theme.RadiusControl).Border(1, tk.Border.Mix(tk.Danger, dangerEdgeSoft)).
			Background(tk.Surface.Mix(tk.Danger, dangerFill)).Padding(failurePadY, failurePadX).Children(func() {
			ui.Text(c, message).FontSize(metaSize).TextColor(tk.Danger).Selectable()
		})
	})
}
