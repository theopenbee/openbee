package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/theopenbee/openbee/desktop/internal/desktop"
	"github.com/theopenbee/openbee/desktop/internal/localserver"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

func main() {
	localserver.RunChild()
	if err := theme.RegisterFonts(); err != nil {
		log.Fatalf("register fonts: %v", err)
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	if err := desktop.New().Run(); err != nil {
		log.Fatal(err)
	}
}
