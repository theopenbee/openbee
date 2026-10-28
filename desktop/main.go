package main

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/prefs"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views"
)

func main() {
	if err := theme.RegisterFonts(); err != nil {
		log.Fatalf("register fonts: %v", err)
	}
	mygo.App.WhenReady(func() {
		if err := open(); err != nil {
			log.Fatal(err)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func open() error {
	data, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return fmt.Errorf("user data directory: %w", err)
	}
	cache, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return fmt.Errorf("cache directory: %w", err)
	}
	var win *mygo.Window
	st := store.New(store.Options{
		Post:      func(fn func()) { win.Update(fn) },
		Prefs:     prefs.NewFileStore(filepath.Join(data, "prefs.json")),
		Tokens:    prefs.NewSecretTokenStore(data),
		CacheDir:  cache,
		SetLocale: l10n.SetLang,
		SetThemeSource: func(pref string) {
			mygo.Theme.SetSource(mygo.ThemeSource(pref))
		},
	})
	root := views.NewRoot(st)
	win = mygo.NewWindow(mygo.WindowOptions{
		Title:     "OpenBee",
		Width:     1280,
		Height:    800,
		MinWidth:  720,
		MinHeight: 480,
		StateKey:  "main",
		Content:   ui.View(root.View),
	})
	st.Start()
	return nil
}
