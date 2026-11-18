package shortcuts

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"
)

const darwin = "darwin"

type Shortcut struct {
	Mods ui.Modifiers
	Key  ui.Key
}

var (
	QuickSearch          = Shortcut{ui.Cmd, ui.KeyK}
	ShowShortcuts        = Shortcut{ui.Cmd, ui.KeySlash}
	OpenSettings         = Shortcut{ui.Cmd, ui.KeyComma}
	NewWorker            = Shortcut{ui.Cmd, ui.KeyN}
	NewConversation      = Shortcut{ui.Cmd | ui.Shift, ui.KeyO}
	PreviousConversation = Shortcut{ui.Alt, ui.KeyUp}
	NextConversation     = Shortcut{ui.Alt, ui.KeyDown}
	Close                = Shortcut{0, ui.KeyEscape}
	Send                 = Shortcut{0, ui.KeyEnter}
	NewLine              = Shortcut{ui.Shift, ui.KeyEnter}
)

func (s Shortcut) Is(mods ui.Modifiers, key ui.Key) bool {
	return s.Mods == mods && s.Key == key
}

func (s Shortcut) Caps() []string {
	return caps(s, runtime.GOOS)
}

func (s Shortcut) Text() string {
	return text(s, runtime.GOOS)
}

func (s Shortcut) Register(c *ui.Context, fn func()) {
	c.OnShortcut(s.Mods, s.Key, fn)
}

func (s Shortcut) On(e ui.Element, fn func()) ui.Element {
	return e.OnShortcut(s.Mods, s.Key, fn)
}

func text(s Shortcut, goos string) string {
	sep := "+"
	if goos == darwin {
		sep = ""
	}
	return strings.Join(caps(s, goos), sep)
}

var modCaps = []struct {
	mod        ui.Modifiers
	mac, other string
}{
	{ui.Ctrl, "⌃", "Ctrl"},
	{ui.Alt, "⌥", "Alt"},
	{ui.Shift, "⇧", "Shift"},
	{ui.Super, "⌘", "Win"},
}

func caps(s Shortcut, goos string) []string {
	mac := goos == darwin
	var out []string
	for _, m := range modCaps {
		switch {
		case s.Mods&m.mod == 0:
		case mac:
			out = append(out, m.mac)
		default:
			out = append(out, m.other)
		}
	}
	return append(out, keyCap(s.Key, mac))
}

func keyCap(key ui.Key, mac bool) string {
	switch key {
	case ui.KeyUp:
		return "↑"
	case ui.KeyDown:
		return "↓"
	case ui.KeyLeft:
		return "←"
	case ui.KeyRight:
		return "→"
	case ui.KeySlash:
		return "/"
	case ui.KeyComma:
		return ","
	case ui.KeyEscape:
		if mac {
			return "esc"
		}
		return "Esc"
	case ui.KeyEnter:
		if mac {
			return "↩"
		}
		return "Enter"
	}
	if key >= ui.KeyA && key <= ui.KeyZ {
		return string(rune('A' + int(key-ui.KeyA)))
	}
	return ""
}
