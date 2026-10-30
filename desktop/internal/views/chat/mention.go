package chat

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

const (
	mentionLimit    = 8
	mentionWidth    = 256
	mentionEdge     = 32
	mentionGap      = 12
	mentionPad      = 8
	mentionListMax  = 288
	mentionRow      = 44
	mentionRowPadX  = 12
	mentionRowGap   = 12
	mentionNameSize = 14
	mentionAppear   = 150 * time.Millisecond
)

type mentionState struct {
	start   int
	query   string
	matches []api.Worker
	active  int
	reveal  bool
	quiet   bool
	quietAt string
	seen    string
}

func mentionQuery(text string, caret int) (int, string, bool) {
	runes := []rune(text)
	before := runes[:min(max(caret, 0), len(runes))]
	at := -1
	for i := len(before) - 1; i >= 0; i-- {
		if before[i] == '@' {
			at = i
			break
		}
	}
	if at < 0 {
		return 0, "", false
	}
	query := string(before[at+1:])
	if strings.ContainsAny(query, " \n") {
		return 0, "", false
	}
	return at, query, true
}

func mentionCandidates(workers []api.Worker, query string) []api.Worker {
	q := strings.ToLower(query)
	var out []api.Worker
	for _, w := range workers {
		if len(out) == mentionLimit {
			break
		}
		if strings.Contains(strings.ToLower(w.Name), q) {
			out = append(out, w)
		}
	}
	return out
}

func applyMention(text string, start, caret int, name string) (string, int) {
	runes := []rune(text)
	caret = min(max(caret, 0), len(runes))
	start = min(max(start, 0), caret)
	inserted := "@" + name + " "
	return string(runes[:start]) + inserted + string(runes[caret:]), start + utf8.RuneCountInString(inserted)
}

func (v *View) mentionWorkers() []api.Worker {
	if !v.s.Workers.Loaded {
		return nil
	}
	return v.s.Workers.Data
}

func (v *View) trackMention(cm *composerState, focused bool) {
	ms := &cm.mention
	typed := cm.draft != ms.seen
	ms.seen = cm.draft
	if ms.quiet && cm.draft != ms.quietAt {
		ms.quiet = false
	}
	start, query, ok := mentionQuery(cm.draft, cm.caret)
	open := len(ms.matches) > 0
	var matches []api.Worker
	if ok && focused && !cm.selecting && !ms.quiet && (typed || open && start == ms.start && query == ms.query) {
		matches = mentionCandidates(v.mentionWorkers(), query)
	}
	if start != ms.start || query != ms.query {
		ms.active = 0
	}
	ms.start, ms.query, ms.matches = start, query, matches
	ms.active = min(ms.active, max(len(matches)-1, 0))
}

func (v *View) mentionKey(cm *composerState, k ui.TextKey) bool {
	ms := &cm.mention
	if len(ms.matches) == 0 {
		return false
	}
	if k.Key == ui.KeyEnter && k.Mods&^ui.Shift == 0 {
		return v.pickByKey(cm, k)
	}
	if k.Mods != 0 {
		return false
	}
	switch k.Key {
	case ui.KeyUp:
		ms.active, ms.reveal = max(ms.active-1, 0), true
	case ui.KeyDown:
		ms.active, ms.reveal = min(ms.active+1, len(ms.matches)-1), true
	case ui.KeyEscape:
		ms.quiet, ms.quietAt, ms.matches = true, k.Text(), nil
	case ui.KeyTab:
		return v.pickByKey(cm, k)
	default:
		return false
	}
	return true
}

func (v *View) pickByKey(cm *composerState, k ui.TextKey) bool {
	ms := &cm.mention
	text := k.Text()
	caret := cm.caret + utf8.RuneCountInString(text) - utf8.RuneCountInString(cm.draft)
	start, query, ok := mentionQuery(text, caret)
	if !ok {
		return false
	}
	matches, active := ms.matches, ms.active
	if start != ms.start || query != ms.query {
		matches, active = mentionCandidates(v.mentionWorkers(), query), 0
	}
	if len(matches) == 0 {
		return false
	}
	next, at := applyMention(text, start, caret, matches[active].Name)
	k.SetText(next)
	cm.placeCaret(at)
	ms.matches = nil
	return true
}

func (v *View) pickByClick(cm *composerState, name string) {
	ms := &cm.mention
	next, at := applyMention(cm.draft, ms.start, cm.caret, name)
	cm.draft = next
	cm.placeCaret(at)
	ms.matches = nil
}

func (v *View) mentionPopup(c *ui.Context, tk theme.Tokens, cm *composerState, anchor ui.Element) {
	ms := &cm.mention
	if len(ms.matches) == 0 {
		return
	}
	w, _ := c.Size()
	ui.Overlay(c, func() {
		panel := ui.Column(c).AttachTo(anchor, ui.AnchorTopLeft, ui.AnchorBottomLeft).Margin(0, 0, mentionGap, 0).
			Width(min(mentionWidth, w-mentionEdge)).Padding(mentionPad).Radius(theme.RadiusPanel).
			Background(tk.SurfaceRaised).Border(1, tk.Border).TextColor(tk.Foreground).KeepFocus()
		theme.WithShadow(panel, tk.ShadowFloating)
		shown := panel.Keyframes("appear", []ui.Keyframe{{At: 0, Value: 0}, {At: mentionAppear, Value: 1, Ease: easeEnter}})
		panel.Opacity(shown).Children(func() {
			ui.Scroll(c).MaxHeight(mentionListMax).Children(func() {
				for i, worker := range ms.matches {
					v.mentionOption(c, tk, cm, i, worker)
				}
			})
		})
		ms.reveal = false
	})
}

func (v *View) mentionOption(c *ui.Context, tk theme.Tokens, cm *composerState, i int, w api.Worker) {
	active := i == cm.mention.active
	b := ui.ButtonBase(c).Key(w.ID).Label(w.Name).KeepFocus().MinHeight(mentionRow).Padding(0, mentionRowPadX).
		Gap(mentionRowGap).AlignItems(ui.Center).Justify(ui.Start).Radius(theme.Concentric(theme.RadiusPanel, mentionPad)).
		FontSize(mentionNameSize).Cursor(ui.CursorPointer)
	bg := tk.SurfaceMuted.Alpha(0)
	if active || b.Hovered() || b.Pressed() {
		bg = tk.SurfaceMuted
	}
	b.Background(b.AnimateColor("bg", bg, colorFade)).OnClick(func() { v.pickByClick(cm, w.Name) }).Children(func() {
		kit.WorkerAvatar(c, tk, w.ID, w.Name, w.Status, kit.AvatarSM, true)
		ui.Text(c, w.Name).SingleLine().MinWidth(0)
	})
	if active && cm.mention.reveal {
		b.ScrollIntoView()
	}
}
