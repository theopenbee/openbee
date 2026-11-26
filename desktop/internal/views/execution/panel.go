package execution

import (
	"net/http"
	"strconv"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/icons"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	sectionGap       = 24
	summaryGap       = 12
	summaryBelow     = 20
	identityGap      = 10
	metaGapX         = 12
	metaGapY         = 4
	metaIconGap      = 6
	engineGlyph      = 14
	liveDot          = 6
	liveRing         = 2
	textSize         = 14
	metaSize         = 12
	mediumWeight     = 500
	instructionLines = 3
	footerTop        = 16
	stateGap         = 12
	runJoin          = " · "
	stepsUnsupported = "transcript_unsupported"
	stepsUnavailable = "engine_unavailable"
)

var livePing = ui.CubicBezier(0, 0, 0.2, 1)

type Prose func(c *ui.Context, text string)

type Panel struct {
	s     *store.Store
	nav   route.Nav
	prose Prose
}

type runKey struct{}

func New(s *store.Store, nav route.Nav, prose Prose) *Panel {
	return &Panel{s: s, nav: nav, prose: prose}
}

func Duration(ms int64) string {
	d := model.SplitDuration(ms)
	switch {
	case d.Hours > 0 && d.Minutes > 0:
		return locales.ExecutionHoursMinutes(strconv.Itoa(d.Hours), strconv.Itoa(d.Minutes))
	case d.Hours > 0:
		return locales.ExecutionHours(d.Hours)
	case d.Minutes > 0 && d.Seconds > 0:
		return locales.ExecutionMinutesSeconds(strconv.Itoa(d.Minutes), strconv.Itoa(d.Seconds))
	case d.Minutes > 0:
		return locales.ExecutionMinutes(d.Minutes)
	}
	return locales.ExecutionSeconds(d.Seconds)
}

func StatusLabel(status string) string {
	switch status {
	case model.ExecutionRunning:
		return locales.ExecutionStatusRunning()
	case model.ExecutionCompleted:
		return locales.ExecutionStatusCompleted()
	case model.ExecutionFailed:
		return locales.ExecutionStatusFailed()
	}
	return status
}

func RunLabel(run api.ExecutionSummary, latest bool) string {
	label := StatusLabel(run.Status)
	if latest {
		label = locales.ExecutionLatest() + label
	}
	if run.CompletedAt == nil {
		return label
	}
	return label + runJoin + locales.ExecutionTook(Duration(*run.CompletedAt-run.StartedAt))
}

func statusTone(status string) kit.Tone {
	switch status {
	case model.ExecutionRunning:
		return kit.ToneInfo
	case model.ExecutionCompleted:
		return kit.ToneSuccess
	case model.ExecutionFailed:
		return kit.ToneDanger
	}
	return kit.ToneNeutral
}

func (p *Panel) View(c *ui.Context, conversationID string, panel route.Panel) {
	if panel.Mode != route.ModeRun || panel.Run == "" {
		return
	}
	tk := theme.Apply(c)
	r := p.s.Execution(panel.Run)
	r.Ensure()
	ui.Column(c).Key(panel.Run).MinWidth(0).Gap(sectionGap).Children(func() {
		switch {
		case api.IsStatus(r.Err, http.StatusNotFound):
			ui.Text(c, locales.ExecutionNotFound()).FontSize(textSize).TextColor(tk.TextSecondary)
		case r.Loaded:
			p.detail(c, tk, route.Param(conversationID), r.Data)
		case r.Err != nil:
			ui.Column(c).AlignItems(ui.Start).Gap(stateGap).Children(func() {
				ui.Text(c, locales.CommonLoadError()).FontSize(textSize).TextColor(tk.TextSecondary)
				kit.Button(c, tk, locales.CommonRetry(), kit.ButtonOpts{Variant: kit.Secondary, Size: kit.SizeSM, Loading: r.Loading}).
					OnClick(r.Invalidate)
			})
		default:
			ui.Text(c, locales.ExecutionLoading()).FontSize(textSize).TextColor(tk.TextSecondary).Role(ui.RoleStatus)
		}
	})
}

func (p *Panel) detail(c *ui.Context, tk theme.Tokens, param string, e api.ExecutionDetail) {
	steps := model.KnownSteps(e.Steps)
	running := e.Status == model.ExecutionRunning
	failure := ""
	if e.Status == model.ExecutionFailed {
		failure = e.Error
	}
	p.summary(c, tk, param, e)
	notice(c, tk, e, len(steps) == 0)
	if len(steps) > 0 || failure != "" {
		p.timeline(c, tk, steps, running, failure)
	}
	footer(c, tk, running, steps)
}

func (p *Panel) summary(c *ui.Context, tk theme.Tokens, param string, e api.ExecutionDetail) {
	p.s.Workers.Ensure()
	w, known := p.s.Worker(e.WorkerID)
	ui.Column(c).MinWidth(0).Gap(summaryGap).Padding(0, 0, summaryBelow, 0).BorderWidth(0, 0, 1, 0).BorderColor(tk.Border).Children(func() {
		ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(identityGap).Children(func() {
			name := locales.ChatDeletedWorker()
			switch {
			case model.IsBeeRun(e):
				name = locales.ChatBeeLabel()
				kit.BeeAvatar(c, tk, kit.AvatarSM)
			case known:
				name = w.Name
				kit.WorkerAvatar(c, tk, w.ID, w.Name, w.Status, p.s.AvatarImage(w.Avatar), kit.AvatarSM, false)
			default:
				kit.UnknownAvatar(c, tk, kit.AvatarSM)
			}
			ui.Text(c, name).FontSize(textSize).FontWeight(mediumWeight).TextColor(tk.Foreground).SingleLine().Grow(1).MinWidth(0)
			kit.SmallBadge(c, tk, StatusLabel(e.Status), statusTone(e.Status))
		})
		if instruction := model.PlainInstruction(e.Instruction); instruction != "" {
			ui.Text(c, instruction).FontSize(textSize).TextColor(tk.Foreground).MaxLines(instructionLines)
		}
		p.meta(c, tk, e)
		if runs := model.SwitcherRuns(e); len(runs) > 1 {
			p.switcher(c, tk, param, e.ID, runs)
		}
	})
}

func (p *Panel) meta(c *ui.Context, tk theme.Tokens, e api.ExecutionDetail) {
	now := c.Now()
	running := e.Status == model.ExecutionRunning
	if running {
		c.After(untilNextSecond(time.UnixMilli(e.StartedAt), now))
	}
	ui.Row(c).Wrap().MinWidth(0).AlignItems(ui.Center).GapX(metaGapX).GapY(metaGapY).FontSize(metaSize).
		TextColor(tk.TextSecondary).FontFeatures("tnum").Children(func() {
		if e.Engine != "" {
			info := model.FindEngine(p.s.EnabledEngines(), e.Engine)
			ui.Row(c).MinWidth(0).AlignItems(ui.Center).Gap(metaIconGap).Children(func() {
				ui.Icon(c, icons.Engine(info.Icon)).Size(engineGlyph, engineGlyph).Shrink(0)
				ui.Text(c, l10n.EngineName(info)).SingleLine().MinWidth(0)
			})
		}
		ui.Text(c, locales.ExecutionStartedAt(model.MessageTime(e.StartedAt, now, i18n.Locale()))).SingleLine()
		span, ok := model.RunSpan(e.ExecutionSummary, now)
		switch {
		case !ok:
		case running:
			ui.Row(c).AlignItems(ui.Center).Gap(metaIconGap).Children(func() {
				live(c, tk)
				ui.Text(c, locales.ExecutionElapsed(Duration(span))).SingleLine()
			})
		default:
			ui.Text(c, locales.ExecutionTook(Duration(span))).SingleLine()
		}
	})
}

func untilNextSecond(started, now time.Time) time.Duration {
	if started.IsZero() || now.Before(started) {
		return time.Second
	}
	return time.Second - now.Sub(started)%time.Second
}

func live(c *ui.Context, tk theme.Tokens) {
	dot := ui.Box(c).Size(liveDot, liveDot).Shrink(0).Radius(theme.RadiusPill).Background(tk.Success)
	dot.Children(func() {
		ping := livePing(dot.Loop("ping", time.Second, ui.Linear))
		scale := 1 + liveRing*ping
		ui.Box(c).Absolute().Top(0).Right(0).Bottom(0).Left(0).Radius(theme.RadiusPill).Background(tk.Success).
			Opacity(0.6*(1-ping)).Scale(scale, scale)
	})
}

func (p *Panel) switcher(c *ui.Context, tk theme.Tokens, param, current string, runs []api.ExecutionSummary) {
	now, lang := c.Now(), i18n.Locale()
	options := make([]kit.SelectOption, len(runs))
	for i, run := range runs {
		options[i] = kit.SelectOption{Value: run.ID, Text: locales.ExecutionRunOption(model.MessageTime(run.StartedAt, now, lang), StatusLabel(run.Status))}
	}
	box := ui.Column(c).MinWidth(0)
	chosen := ui.Local(box, runKey{}, func() string { return current })
	*chosen = current
	box.Children(func() {
		kit.Select(c, tk, chosen, locales.ExecutionRuns(), options, "").OnChange(func() {
			if *chosen != current {
				p.nav.Replace(route.ChatRun(param, *chosen))
			}
		})
	})
}

func notice(c *ui.Context, tk theme.Tokens, e api.ExecutionDetail, empty bool) {
	switch {
	case e.StepsError != "":
		tone, title := kit.ToneWarning, locales.ExecutionStepsErrorTranscriptFailed()
		switch e.StepsError {
		case stepsUnsupported:
			tone, title = kit.ToneInfo, locales.ExecutionStepsErrorTranscriptUnsupported()
		case stepsUnavailable:
			title = locales.ExecutionStepsErrorEngineUnavailable()
		}
		kit.Alert(c, tk, kit.AlertOpts{Tone: tone, Title: title})
	case !empty:
	case e.Status == model.ExecutionRunning:
		kit.Shimmer(c, tk, locales.ExecutionWaiting())
	default:
		ui.Text(c, locales.ExecutionNoSteps()).FontSize(textSize).TextColor(tk.TextSecondary)
	}
}

func footer(c *ui.Context, tk theme.Tokens, running bool, steps []api.ExecutionStep) {
	if !running && len(steps) == 0 {
		return
	}
	text := locales.ExecutionLive()
	if !running {
		count, tools := model.StepStats(steps)
		text = locales.ExecutionStepCount(locales.ExecutionSteps(count), locales.ExecutionToolCalls(tools))
	}
	ui.Column(c).MinWidth(0).Padding(footerTop, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(tk.Border).Children(func() {
		ui.Text(c, text).FontSize(metaSize).TextColor(tk.TextMuted)
	})
}
