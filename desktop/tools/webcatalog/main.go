package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

//go:embed desktop.en.json desktop.zh.json
var overlays embed.FS

var languages = []string{"en", "zh"}

type outputs struct {
	Locales map[string][]byte
	Errors  []byte
}

func readJSON[T any](path string) (T, error) {
	var value T
	data, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("%s: %w", path, err)
	}
	return value, nil
}

func build(webDir string) (outputs, error) {
	out := outputs{Locales: map[string][]byte{}}
	messages := map[string]map[string]string{}
	placeholders := map[string]map[string][]string{}
	webs := map[string]map[string]any{}
	for _, lang := range languages {
		web, err := readJSON[map[string]any](filepath.Join(webDir, lang+".json"))
		if err != nil {
			return out, err
		}
		var overlay map[string]string
		data, err := overlays.ReadFile("desktop." + lang + ".json")
		if err != nil {
			return out, err
		}
		if err := json.Unmarshal(data, &overlay); err != nil {
			return out, fmt.Errorf("desktop.%s.json: %w", lang, err)
		}
		webs[lang] = web
		if messages[lang], placeholders[lang], err = Convert(web, overlay); err != nil {
			return out, fmt.Errorf("%s: %w", lang, err)
		}
	}
	source := languages[0]
	for _, lang := range languages[1:] {
		Declare(placeholders[source], placeholders[lang])
	}
	for _, lang := range languages {
		out.Locales[lang] = ARB(lang, messages[lang], placeholders[lang])
	}
	var err error
	out.Errors, err = ErrorsSource(webs[source], placeholders[source])
	return out, err
}

func main() {
	webDir := flag.String("web", "../web/src/locales", "directory with the web en.json and zh.json")
	outDir := flag.String("out", "locales", "directory to write the ARB catalogs to")
	errorsPath := flag.String("errors", "internal/l10n/errors_gen.go", "file to write the error text switch to")
	flag.Parse()
	out, err := build(*webDir)
	if err != nil {
		log.Fatal(err)
	}
	for lang, data := range out.Locales {
		if err := os.WriteFile(filepath.Join(*outDir, lang+".json"), data, 0o644); err != nil {
			log.Fatal(err)
		}
	}
	if err := os.WriteFile(*errorsPath, out.Errors, 0o644); err != nil {
		log.Fatal(err)
	}
}
