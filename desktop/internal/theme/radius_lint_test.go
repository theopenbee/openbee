package theme

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	literalRadius = regexp.MustCompile(`\.Radius\(\s*[-0-9.]`)
	literalColor  = regexp.MustCompile(`ui\.(Hex|RGB|RGBA|Oklch)\(`)
)

func TestNoLiteralRadius(t *testing.T) {
	root := filepath.Join("..", "..")
	var files []string
	files = append(files, filepath.Join(root, "main.go"))
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if filepath.Base(path) == "theme" && filepath.Base(filepath.Dir(path)) == "internal" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		shown, err := filepath.Rel(root, path)
		if err != nil {
			shown = path
		}
		scanner := bufio.NewScanner(f)
		line := 0
		for scanner.Scan() {
			line++
			text := scanner.Text()
			if literalRadius.MatchString(text) {
				t.Errorf("%s:%d: literal radius: %s", filepath.ToSlash(shown), line, strings.TrimSpace(text))
			}
			if literalColor.MatchString(text) {
				t.Errorf("%s:%d: literal colour outside internal/theme: %s", filepath.ToSlash(shown), line, strings.TrimSpace(text))
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
	}
}
