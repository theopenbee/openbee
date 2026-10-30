package theme

import (
	_ "embed"
	"sync"

	"github.com/egoist/mygo/ui"
)

const (
	FontBody    = "Inter"
	FontDisplay = "Geist"
	FontMono    = "JetBrains Mono"
)

//go:embed fonts/Inter.ttf
var interTTF []byte

//go:embed fonts/Geist.ttf
var geistTTF []byte

//go:embed fonts/JetBrainsMono.ttf
var jetBrainsMonoTTF []byte

var (
	registerOnce sync.Once
	registerErr  error
)

func RegisterFonts() error {
	registerOnce.Do(func() {
		fonts := []struct {
			data   []byte
			family string
		}{
			{interTTF, FontBody},
			{geistTTF, FontDisplay},
			{jetBrainsMonoTTF, FontMono},
		}
		for _, f := range fonts {
			if err := ui.RegisterFont(f.data, f.family); err != nil {
				registerErr = err
				return
			}
		}
	})
	return registerErr
}
