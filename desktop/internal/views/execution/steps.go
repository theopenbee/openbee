package execution

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	listGap     = 2
	listBleed   = 12
	rowH        = 36
	rowPadX     = 12
	rowGap      = 12
	metaGap     = 8
	stepGlyph   = 16
	bodyIndent  = rowPadX + stepGlyph + rowGap
	blockPadY   = 8
	glyphNudge  = 2
	boxTop      = 4
	boxBelow    = 8
	stripPadX   = 12
	stripPadY   = 8
	statsGap    = 6
	monoSize    = 12
	monoLeading = 1.625
	bodyMaxH    = 256
	diffPadY    = 6
	markerW     = 8
	markerGap   = 8
	thinkTop    = 4
	tintAlpha   = 0.10
	dangerEdge  = 0.25
	clipSlack   = 0.5
	turnOpen    = 180
	noLigatures = "calt=0"
)

var rowFade = ui.Animation{Duration: 150 * time.Millisecond, Ease: ui.CubicBezier(0.4, 0, 0.2, 1)}

var toolIcons = map[string]string{
	"command":         "terminal",
	"read":            "file-text",
	"edit":            "file-pen-line",
	"search":          "search",
	"web":             "globe",
	model.ToolMessage: "send",
	model.ToolOther:   "wrench",
}

type (
	openKey      struct{}
	truncatedKey struct{}
)

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

func ToolTitle(step api.ExecutionStep) string {
	kind := model.ToolKind(step.Kind)
	summary := model.OneLine(step.Summary)
	if kind == model.ToolOther || summary == "" {
		return toolLabel(kind, step.Name)
	}
	switch kind {
	case "command":
		return locales.ExecutionToolTitleCommand(summary)
	case "read":
		return locales.ExecutionToolTitleRead(summary)
	case "edit":
		return locales.ExecutionToolTitleEdit(summary)
	case "search":
		return locales.ExecutionToolTitleSearch(summary)
	case "web":
		return locales.ExecutionToolTitleWeb(summary)
	}
	return locales.ExecutionToolTitleMessage(summary)
}

func (p *Panel) timeline(c *ui.Context, tk theme.Tokens, steps []api.ExecutionStep, running bool, failure string) {
	col := ui.Column(c).MinWidth(0).MarginX(-listBleed).Gap(listGap).Role(ui.RoleList)
	open := ui.Local(col, openKey{}, func() map[int]bool { return map[int]bool{} })
	col.Children(func() {
		for i, step := range steps {
			listItem(c, func(item ui.Element) {
				switch step.Type {
				case model.StepThinking:
					thinkingStep(c, tk, item, step.Text, *open, i)
				case model.StepText:
					p.textStep(c, tk, step.Text)
				case model.StepError:
					errorStep(c, tk, step.Text)
				case model.StepTool:
					toolStep(c, tk, item, step, running, *open, i)
				}
			})
		}
		if failure != "" {
			listItem(c, func(ui.Element) { failureStep(c, tk, failure) })
		}
	})
}

func listItem(c *ui.Context, body func(item ui.Element)) {
	item := ui.Column(c).MinWidth(0).Role(ui.RoleListItem)
	item.Children(func() { body(item) })
}

func glyph(c *ui.Context, name string, color ui.Color) ui.Element {
	return ui.Icon(c, icons.Get(name)).Size(stepGlyph, stepGlyph).Shrink(0).TextColor(color)
}

type stepRow struct {
	label      string
	expandable bool
	open       bool
	toggle     func()
	icon       func()
	title      func()
	meta       func()
}

func (r stepRow) show(c *ui.Context, tk theme.Tokens) {
	content := func() {
		r.icon()
		r.title()
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(metaGap).FontSize(metaSize).TextColor(tk.TextMuted).
			FontFeatures("tnum").Children(func() {
			if r.meta != nil {
				r.meta()
			}
			if !r.expandable {
				ui.Box(c).Size(stepGlyph, stepGlyph).Shrink(0)
				return
			}
			turn := float32(0)
			if r.open {
				turn = turnOpen
			}
			chevron := glyph(c, "chevron-down", tk.TextMuted)
			chevron.Rotate(chevron.AnimateTo("turn", turn, ui.Snappy))
		})
	}
	if !r.expandable {
		ui.Row(c).MinWidth(0).Height(rowH).AlignItems(ui.Center).Gap(rowGap).PaddingX(rowPadX).Children(content)
		return
	}
	b := ui.ButtonBase(c).Label(r.label).Expanded(r.open).MinWidth(0).Height(rowH).AlignItems(ui.Center).Justify(ui.Start).
		Gap(rowGap).PaddingX(rowPadX).Radius(theme.RadiusControl).OnClick(r.toggle)
	bg := tk.SurfaceMuted.Alpha(0)
	if b.Hovered() || b.Pressed() {
		bg = tk.SurfaceMuted
	}
	b.Background(b.AnimateColor("bg", bg, rowFade)).Children(content)
}

func fitTitle(c *ui.Context, truncated *bool, text string, body func()) ui.Element {
	box := ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center)
	if w := box.LayoutBounds().W; w > 0 && wider(c.MeasureText, w, text) != *truncated {
		*truncated = !*truncated
		c.Services().Invalidate()
	}
	services := c.Services()
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		if wider(p.MeasureText, r.W, text) != *truncated {
			services.Invalidate()
		}
	})
	return box.Children(body)
}

func titleText(c *ui.Context, tk theme.Tokens, text string) ui.Element {
	return ui.Text(c, text).FontSize(textSize).TextColor(tk.Foreground).SingleLine().Shrink(1).MinWidth(0)
}

func wider(measure func(float32, ...ui.Span) (float32, float32), width float32, text string) bool {
	w, _ := measure(0, ui.Span{Text: text, Size: textSize})
	return w > width+clipSlack
}

func thinkingStep(c *ui.Context, tk theme.Tokens, item ui.Element, text string, open map[int]bool, index int) {
	title, rest := model.SplitThinking(text)
	truncated := ui.Local(item, truncatedKey{}, func() bool { return false })
	expandable := rest != "" || *truncated
	expanded := expandable && open[index]
	thinking := locales.ExecutionThinking()
	stepRow{
		label:      thinking + runJoin + title,
		expandable: expandable,
		open:       expanded,
		toggle:     func() { open[index] = !open[index] },
		icon: func() {
			glyph(c, "sparkles", tk.TextSecondary).Label(thinking).Role(ui.RoleImage)
		},
		title: func() { fitTitle(c, truncated, title, func() { titleText(c, tk, title) }) },
	}.show(c, tk)
	if !expanded {
		return
	}
	body := rest
	if *truncated {
		body = strings.TrimSpace(text)
	}
	ui.Text(c, body).FontSize(textSize).TextColor(tk.TextSecondary).Padding(thinkTop, rowPadX, blockPadY, bodyIndent).Selectable()
}

func (p *Panel) textStep(c *ui.Context, tk theme.Tokens, text string) {
	ui.Row(c).MinWidth(0).AlignItems(ui.Start).Gap(rowGap).Padding(blockPadY, rowPadX).Children(func() {
		glyph(c, "message-square-text", tk.TextSecondary).Margin(glyphNudge, 0, 0, 0)
		ui.Column(c).Grow(1).MinWidth(0).FontSize(textSize).TextColor(tk.Foreground).Children(func() {
			if p.prose != nil {
				p.prose(c, text)
				return
			}
			ui.Text(c, text).Selectable()
		})
	})
}

func errorStep(c *ui.Context, tk theme.Tokens, text string) {
	ui.Row(c).MinWidth(0).AlignItems(ui.Start).Gap(rowGap).Padding(blockPadY, rowPadX).Children(func() {
		glyph(c, "circle-alert", tk.Danger).Margin(glyphNudge, 0, 0, 0)
		ui.Text(c, text).Grow(1).MinWidth(0).FontSize(textSize).TextColor(tk.Danger).Selectable()
	})
}

func toolStep(c *ui.Context, tk theme.Tokens, item ui.Element, step api.ExecutionStep, running bool, open map[int]bool, index int) {
	kind := model.ToolKind(step.Kind)
	active := running && step.Status == model.ToolRunning
	failed := step.Status == model.ToolError
	title := ToolTitle(step)
	summary := strings.TrimSpace(step.Summary)
	truncated := ui.Local(item, truncatedKey{}, func() bool { return false })
	hasBody := model.ToolOutput(step) != "" || step.Diff != ""
	expandable := hasBody || summary != "" && (kind == model.ToolMessage || kind == model.ToolOther || *truncated)
	expanded := expandable && open[index]
	iconColor := tk.TextSecondary
	if failed {
		iconColor = tk.Danger
	}
	stepRow{
		label:      toolName(title, step.Status, active),
		expandable: expandable,
		open:       expanded,
		toggle:     func() { open[index] = !open[index] },
		icon:       func() { glyph(c, toolIcons[kind], iconColor) },
		title: func() {
			box := fitTitle(c, truncated, title, func() {
				if active {
					kit.Shimmer(c, tk, title).Shrink(1)
				} else {
					titleText(c, tk, title)
				}
			})
			if summary != "" && *truncated {
				kit.Tooltip(c, tk, box, title)
			}
		},
		meta: func() {
			if model.ToolDurationShown(step.DurationMS) {
				ui.Text(c, Duration(step.DurationMS)).SingleLine()
			}
			toolMark(c, tk, step.Status, active)
		},
	}.show(c, tk)
	if expanded {
		ToolDetail(c, tk, step)
	}
}

func toolName(title, status string, active bool) string {
	switch {
	case status == model.ToolError:
		return title + runJoin + locales.ExecutionStatusFailed()
	case status == model.ToolDone || active:
		return title
	}
	return title + runJoin + locales.ExecutionToolStopped()
}

func toolMark(c *ui.Context, tk theme.Tokens, status string, active bool) {
	switch {
	case status == model.ToolError:
		glyph(c, "x", tk.Danger).Label(locales.ExecutionStatusFailed()).Role(ui.RoleImage)
	case status == model.ToolDone:
		glyph(c, "check", tk.Success)
	case active:
		kit.Spinner(c, tk, stepGlyph).TextColor(tk.TextSecondary).Label(locales.ExecutionStatusRunning())
	default:
		stopped := locales.ExecutionToolStopped()
		kit.Tooltip(c, tk, glyph(c, "circle-slash", tk.TextMuted).Label(stopped).Role(ui.RoleImage), stopped)
	}
}

func detailBox(c *ui.Context, border ui.Color) ui.Element {
	return ui.Column(c).MinWidth(0).Margin(boxTop, rowPadX, boxBelow, bodyIndent).Radius(theme.RadiusInset).Border(1, border).Clip()
}

func ToolDetail(c *ui.Context, tk theme.Tokens, step api.ExecutionStep) {
	summary := strings.TrimSpace(step.Summary)
	var diff []model.DiffLine
	if step.Diff != "" {
		diff = model.DiffLines(step.Diff)
	}
	output := model.ToolOutput(step)
	hasBody := output != "" || diff != nil
	detailBox(c, tk.Border).Children(func() {
		if summary != "" {
			ui.Row(c).MinWidth(0).AlignItems(ui.Start).Gap(metaGap).Padding(stripPadY, stripPadX).Children(func() {
				text := ui.Text(c, summary).Grow(1).MinWidth(0).TextColor(tk.Foreground).Selectable()
				if model.ToolKind(step.Kind) == model.ToolMessage {
					text.FontSize(textSize)
				} else {
					mono(text)
				}
				if diff != nil {
					diffStats(c, tk, diff)
				}
			})
		}
		if !hasBody {
			return
		}
		section := ui.Column(c).MinWidth(0).MaxHeight(bodyMaxH)
		if summary != "" {
			section.BorderWidth(1, 0, 0, 0).BorderColor(tk.Border)
		}
		section.Children(func() {
			ui.Scroll(c).Shrink(1).MinHeight(0).Children(func() {
				ui.Column(c).MinWidth(0).Children(func() {
					if diff != nil {
						diffView(c, tk, diff)
					}
					if output == "" {
						return
					}
					fg := tk.TextSecondary
					if step.Status == model.ToolError {
						fg = tk.Danger
					}
					out := ui.Column(c).MinWidth(0).Padding(stripPadY, stripPadX)
					if diff != nil {
						out.BorderWidth(1, 0, 0, 0).BorderColor(tk.Border)
					}
					out.Children(func() {
						mono(ui.Text(c, output)).TextColor(fg).Selectable()
					})
				})
			})
		})
	})
}

func mono(e ui.Element) ui.Element {
	return e.Font(theme.FontMono).FontSize(monoSize).LineHeight(monoLeading).FontFeatures(noLigatures)
}

func diffStats(c *ui.Context, tk theme.Tokens, lines []model.DiffLine) {
	added, removed := model.DiffStats(lines)
	mono(ui.Row(c).Shrink(0).Gap(statsGap)).Children(func() {
		if added > 0 {
			ui.Text(c, "+"+strconv.Itoa(added)).TextColor(tk.Success)
		}
		if removed > 0 {
			ui.Text(c, "-"+strconv.Itoa(removed)).TextColor(tk.Danger)
		}
	})
}

func diffView(c *ui.Context, tk theme.Tokens, lines []model.DiffLine) {
	mono(ui.Column(c).MinWidth(0).PaddingY(diffPadY)).Children(func() {
		for _, line := range lines {
			row := ui.Row(c).MinWidth(0).AlignItems(ui.Start).Gap(markerGap).PaddingX(stripPadX)
			fg, markColor, marker := tk.TextSecondary, tk.TextSecondary, " "
			switch line.Kind {
			case model.DiffAdd:
				fg, markColor, marker = tk.Foreground, tk.Success, "+"
				row.Background(tk.Success.Alpha(tintAlpha))
			case model.DiffRemove:
				fg, markColor, marker = tk.Foreground, tk.Danger, "-"
				row.Background(tk.Danger.Alpha(tintAlpha))
			case model.DiffNote:
				fg = tk.TextMuted
			}
			text := line.Text
			if text == "" {
				text = " "
			}
			row.TextColor(fg).Children(func() {
				ui.Text(c, marker).Width(markerW).Shrink(0).TextColor(markColor).Unselectable()
				ui.Text(c, text).Grow(1).MinWidth(0).Selectable()
			})
		}
	})
}

func failureStep(c *ui.Context, tk theme.Tokens, message string) {
	ui.Row(c).MinWidth(0).Height(rowH).AlignItems(ui.Center).Gap(rowGap).PaddingX(rowPadX).Children(func() {
		glyph(c, "circle-alert", tk.Danger)
		titleText(c, tk, locales.ExecutionFailedTitle()).Grow(1)
	})
	detailBox(c, tk.Danger.Alpha(dangerEdge)).Background(tk.Danger.Alpha(tintAlpha)).Children(func() {
		ui.Text(c, message).FontSize(textSize).TextColor(tk.Danger).Padding(stripPadY, stripPadX).Selectable()
	})
}
