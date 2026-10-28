package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPlaceholderView(t *testing.T) {
	tt := ui.NewTester(placeholderView, 800, 600)
	if !tt.HasText("OpenBee") {
		t.Errorf("texts %q do not contain the app name", tt.Texts())
	}
}
