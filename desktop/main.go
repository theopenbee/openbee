package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func placeholderView(c *ui.Context) {
	ui.Column(c).Fill().Center().Children(func() {
		ui.Text(c, "OpenBee").FontSize(26).Bold()
	})
}

func main() {
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "OpenBee",
			Width:     1280,
			Height:    800,
			MinWidth:  720,
			MinHeight: 480,
			StateKey:  "main",
			Content:   ui.View(placeholderView),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
