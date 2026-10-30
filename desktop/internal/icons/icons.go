package icons

import (
	"embed"
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"
)

//go:embed lucide/*.svg
var lucideFS embed.FS

//go:embed brand/*.svg
var brandFS embed.FS

var (
	cacheMu sync.Mutex
	cache   = map[string]*ui.SVG{}
)

func cached(key string, build func() *ui.SVG) *ui.SVG {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if svg, ok := cache[key]; ok {
		return svg
	}
	svg := build()
	cache[key] = svg
	return svg
}

func Get(name string) *ui.SVG {
	return cached("lucide/"+name, func() *ui.SVG {
		data, err := lucideFS.ReadFile("lucide/" + name + ".svg")
		if err != nil {
			panic(fmt.Sprintf("icons: unknown icon %q", name))
		}
		return ui.MustParseSVG(data)
	})
}

func Engine(engine string) *ui.SVG {
	switch engine {
	case "claude", "codex", "pi":
		return cached("brand/"+engine, func() *ui.SVG { return ui.MustParseSVG(brandData(engine)) })
	}
	return Get("cpu")
}
