package shortcuts

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCapsFollowThePlatform(t *testing.T) {
	cases := []struct {
		name  string
		s     Shortcut
		goos  string
		caps  []string
		label string
	}{
		{"mac command", Shortcut{ui.Super, ui.KeyK}, "darwin", []string{"⌘", "K"}, "⌘K"},
		{"mac shift command", Shortcut{ui.Super | ui.Shift, ui.KeyO}, "darwin", []string{"⇧", "⌘", "O"}, "⇧⌘O"},
		{"mac option arrow", Shortcut{ui.Alt, ui.KeyDown}, "darwin", []string{"⌥", "↓"}, "⌥↓"},
		{"mac escape", Shortcut{0, ui.KeyEscape}, "darwin", []string{"esc"}, "esc"},
		{"windows control", Shortcut{ui.Ctrl, ui.KeySlash}, "windows", []string{"Ctrl", "/"}, "Ctrl+/"},
		{"windows control shift", Shortcut{ui.Ctrl | ui.Shift, ui.KeyO}, "windows", []string{"Ctrl", "Shift", "O"}, "Ctrl+Shift+O"},
		{"linux alt arrow", Shortcut{ui.Alt, ui.KeyUp}, "linux", []string{"Alt", "↑"}, "Alt+↑"},
		{"linux enter", Shortcut{ui.Shift, ui.KeyEnter}, "linux", []string{"Shift", "Enter"}, "Shift+Enter"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := caps(tc.s, tc.goos); !slices.Equal(got, tc.caps) {
				t.Errorf("caps = %q, want %q", got, tc.caps)
			}
			if got := text(tc.s, tc.goos); got != tc.label {
				t.Errorf("text = %q, want %q", got, tc.label)
			}
		})
	}
}

func TestShortcutsAreDistinct(t *testing.T) {
	all := []Shortcut{QuickSearch, ShowShortcuts, OpenSettings, NewWorker, NewConversation, PreviousConversation, NextConversation, Close, Send, NewLine}
	for i, a := range all {
		for _, b := range all[i+1:] {
			if a == b {
				t.Errorf("shortcut %v is used twice", a)
			}
		}
	}
}

func TestIs(t *testing.T) {
	if !NextConversation.Is(ui.Alt, ui.KeyDown) {
		t.Error("Alt+Down is not the next conversation")
	}
	if NextConversation.Is(ui.Alt|ui.Shift, ui.KeyDown) {
		t.Error("Alt+Shift+Down matched the next conversation")
	}
}
