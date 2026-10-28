package chat

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	composerBottom     = 12
	composerBottomWide = 24
	composerBoxPad     = 8
	composerGap        = 4
	composerGapWide    = 8
	composerAlertGap   = 12
	composerLine       = 24
	composerFont       = 16
	composerFontWide   = 14
	composerPadY       = 10
	composerPadYWide   = 12
	composerPadX       = 8
	composerPadXWide   = 12
	composerMin        = 44
	composerMinWide    = 48
	composerMax        = 160
	composerMaxLines   = 6
	hintSize           = 12
	pastedName         = "pasted-%d.png"
)

type composerKey struct{}

type inputRowKey struct{}

type composerState struct {
	draft     string
	files     []string
	failed    int
	sending   int
	composing bool
	caret     int
	selecting bool
	caretTo   int
	moveCaret bool
	mention   mentionState
}

func (cm *composerState) canSend() bool {
	return strings.TrimSpace(cm.draft) != "" || len(cm.files) > 0
}

func (cm *composerState) addFiles(paths ...string) {
	for _, p := range paths {
		if !slices.Contains(cm.files, p) {
			cm.files = append(cm.files, p)
		}
	}
}

func (cm *composerState) removeFile(path string) {
	cm.files = slices.DeleteFunc(slices.Clone(cm.files), func(p string) bool { return p == path })
}

func (cm *composerState) restore(text string, files []string) {
	if cm.draft != "" {
		text += "\n" + cm.draft
	}
	cm.draft = text
	later := cm.files
	cm.files = files
	cm.addFiles(later...)
}

func (cm *composerState) placeCaret(at int) {
	cm.caretTo, cm.moveCaret = at, true
}

func composerWide(c *ui.Context) bool {
	w, _ := c.Size()
	return w >= theme.BreakComposer
}

func (v *View) composer(c *ui.Context, tk theme.Tokens, p pane) {
	cm := &p.cs.composer
	wide := composerWide(c)
	bottom := float32(composerBottom)
	if wide {
		bottom = composerBottomWide
	}
	ui.Column(c).Key(composerKey{}).Shrink(0).Background(tk.Background).Padding(0, 0, bottom, 0).Children(func() {
		column(c, func() {
			if cm.failed > 0 {
				title := locales.ChatUploadError(cm.failed)
				kit.Alert(c, tk, kit.AlertOpts{Tone: kit.ToneDanger, Title: title, OnDismiss: func() { cm.failed = 0 }}).
					Margin(0, 0, composerAlertGap, 0)
			}
			v.composerBox(c, tk, p, wide)
		})
	})
}

func (v *View) composerBox(c *ui.Context, tk theme.Tokens, p pane, wide bool) {
	cm := &p.cs.composer
	box := ui.Column(c).Radius(theme.RadiusSurface).Background(tk.Surface).Padding(composerBoxPad)
	if files := box.DroppedFiles(); files != nil {
		v.upload(p.cs, files)
	}
	border := tk.Border
	switch {
	case box.FileDragOver():
		border = tk.Accent
	case box.FocusWithin():
		border = tk.BorderStrong
	}
	gap := float32(composerGap)
	if wide {
		gap = composerGapWide
	}
	box.Border(1, box.AnimateColor("border", border, colorFade)).Children(func() {
		if len(cm.files) > 0 {
			chips(c, tk, cm, wide)
		}
		ui.Row(c).Key(inputRowKey{}).AlignItems(ui.End).Gap(gap).Children(func() {
			if !wide {
				v.attachButton(c, tk, p)
			}
			v.input(c, tk, p, wide)
			if !wide {
				v.sendButton(c, tk, p)
			}
		})
		if wide {
			ui.Row(c).AlignItems(ui.Center).Gap(gap).Children(func() {
				v.attachButton(c, tk, p)
				ui.Text(c, composerHint(p.cid)).FontSize(hintSize).TextColor(tk.TextMuted).SingleLine().Grow(1).MinWidth(0)
				v.sendButton(c, tk, p)
			})
		}
	})
}

func (v *View) input(c *ui.Context, tk theme.Tokens, p pane, wide bool) {
	cm, cs := &p.cs.composer, p.cs
	font, padY, padX, minH := float32(composerFont), float32(composerPadY), float32(composerPadX), float32(composerMin)
	if wide {
		font, padY, padX, minH = composerFontWide, composerPadYWide, composerPadXWide, composerMinWide
	}
	svc := c.Services()
	ta := ui.TextAreaBase(c.Key(p.cid), &cm.draft).Label(locales.ChatMessageLabel()).Placeholder(placeholder(p)).
		FontSize(font).LineHeight(composerLine/font).TextColor(tk.Foreground).Padding(padY, padX).
		Lines(1, composerMaxLines).MinHeight(minH).MaxHeight(composerMax).Grow(1).MinWidth(0).FocusRing(false).
		OnTextKey(func(k ui.TextKey) bool { return v.composerKey(p, k) }).
		OnPaste(func(text string) bool { return v.paste(cs, svc, text) })
	if cm.moveCaret {
		ta.SetTextSelection(cm.caretTo, cm.caretTo)
		cm.moveCaret = false
	}
	start, end := ta.TextSelection()
	cm.caret, cm.selecting, cm.composing = end, start != end, ta.Composing()
	if isBee(p.cid) {
		v.trackMention(cm, ta.Focused())
		v.mentionPopup(c, tk, cm, ta)
	}
}

func (v *View) attachButton(c *ui.Context, tk theme.Tokens, p pane) {
	kit.IconButton(c, tk, "paperclip", locales.ChatUploadFile(), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).
		OnClick(func() { v.attach(p) })
}

func (v *View) sendButton(c *ui.Context, tk theme.Tokens, p pane) {
	cm := &p.cs.composer
	kit.IconButton(c, tk, "arrow-up", locales.ChatSend(), kit.ButtonOpts{Size: kit.SizeSM, Disabled: !cm.canSend() || cm.sending > 0}).
		OnClick(func() { v.send(p, cm.draft) })
}

func (v *View) composerKey(p pane, k ui.TextKey) bool {
	cm := &p.cs.composer
	if cm.composing {
		return k.Key == ui.KeyEnter
	}
	if v.mentionKey(cm, k) {
		return true
	}
	if k.Key != ui.KeyEnter || k.Mods != 0 {
		return false
	}
	if v.send(p, k.Text()) {
		k.SetText("")
	}
	return true
}

func (v *View) send(p pane, text string) bool {
	cm := &p.cs.composer
	content := strings.TrimSpace(text)
	if content == "" && len(cm.files) == 0 {
		return false
	}
	files := cm.files
	cm.draft, cm.files, cm.failed = "", nil, 0
	cm.mention.matches = nil
	cm.sending++
	p.cs.list.ScrollToEnd()
	p.ch.Send(content, files, func(err error) {
		cm.sending--
		if err == nil {
			return
		}
		cm.restore(text, files)
		if !errors.Is(err, context.Canceled) {
			v.s.Toast(kit.Toast{Tone: kit.ToneDanger, Title: locales.ChatSendFailed(), Description: l10n.ErrorText(err)})
		}
	})
	return true
}

func (v *View) upload(cs *convState, paths []string) {
	cm := &cs.composer
	cm.failed = 0
	v.s.Upload(paths, func(ok []string, failed int) {
		cm.addFiles(ok...)
		if failed > 0 {
			cm.failed = failed
		}
	})
}

func (v *View) paste(cs *convState, svc ui.Services, text string) bool {
	if text != "" {
		return false
	}
	data, err := svc.ReadClipboardData(transfer.PNG).Read(transfer.PNG)
	if err != nil || len(data) == 0 {
		return false
	}
	cm := &cs.composer
	cm.failed = 0
	v.s.UploadBytes(fmt.Sprintf(pastedName, time.Now().Unix()), data, func(path string, err error) {
		if err != nil {
			log.Printf("chat: upload pasted image: %v", err)
			cm.failed = 1
			return
		}
		cm.addFiles(path)
	})
	return true
}

func (v *View) attach(p pane) {
	pick, cs := v.pick, p.cs
	v.s.Go(func(context.Context) {
		paths, err := pick()
		v.s.Post(func() {
			switch {
			case err != nil:
				log.Printf("chat: pick files: %v", err)
			case len(paths) > 0 && v.convs[p.cid] == cs:
				v.upload(cs, paths)
			}
		})
	})
}

func pickWithDialog() ([]string, error) {
	return mygo.Dialog.Open(mygo.OpenDialogOptions{Multiple: true})
}

func placeholder(p pane) string {
	if p.worker == nil {
		return locales.ChatMessagePlaceholder(locales.ChatBeeLabel())
	}
	return locales.ChatMessagePlaceholder(p.worker.Name)
}

func composerHint(cid string) string {
	if isBee(cid) {
		return locales.ChatComposerHint()
	}
	return locales.ChatComposerHintPlain()
}
