package icons

import (
	"bytes"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

func TestAllIconsParse(t *testing.T) {
	count := 0
	for _, fsys := range []fs.FS{lucideFS, brandFS} {
		err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".svg") {
				return nil
			}
			data, err := fs.ReadFile(fsys, path)
			if err != nil {
				return err
			}
			if _, err := ui.ParseSVG(data); err != nil {
				t.Errorf("%s: %v", path, err)
			}
			count++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if count < 60 {
		t.Errorf("parsed %d embedded svg files, want at least 60", count)
	}
}

func TestGetKnownIcon(t *testing.T) {
	if Get("arrow-up") == nil {
		t.Error("Get(\"arrow-up\") = nil")
	}
	if Get("arrow-up") != Get("arrow-up") {
		t.Error("Get does not cache the parsed icon")
	}
}

func TestGetUnknownPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Get of an unknown icon did not panic")
		}
	}()
	Get("no-such-icon")
}

func TestLogoUsesBrand(t *testing.T) {
	for _, tk := range []theme.Tokens{theme.Light(), theme.Dark()} {
		for _, name := range []string{"logo-mark", "logo-full"} {
			src := brandSource(name, tk)
			if bytes.Contains(src, []byte("var(")) {
				t.Errorf("%s (dark=%v) still contains var(", name, tk.Dark)
			}
			if !bytes.Contains(src, []byte(hexOf(tk.Brand))) {
				t.Errorf("%s (dark=%v) does not contain the brand colour %s", name, tk.Dark, hexOf(tk.Brand))
			}
		}
	}
	if hexOf(theme.Light().Brand) == hexOf(theme.Dark().Brand) {
		t.Error("light and dark brand colours are identical")
	}
	if LogoMark(theme.Light()) == nil || LogoFull(theme.Dark()) == nil {
		t.Error("logo is nil")
	}
	if LogoMark(theme.Light()) != LogoMark(theme.Light()) {
		t.Error("LogoMark does not cache per theme")
	}
	if LogoMark(theme.Light()) == LogoMark(theme.Dark()) {
		t.Error("LogoMark shares one image between light and dark")
	}
}

func TestEngineMarks(t *testing.T) {
	for _, engine := range []string{"claude", "codex", "pi"} {
		if Engine(engine) == nil {
			t.Errorf("Engine(%q) = nil", engine)
		}
	}
	generic := Engine("gemini")
	if generic == nil {
		t.Fatal("Engine(\"gemini\") = nil, want the generic engine icon")
	}
	if Engine("another-plugin") != generic {
		t.Error("unknown engines do not share the generic icon")
	}
	for _, engine := range []string{"claude", "codex", "pi"} {
		if Engine(engine) == generic {
			t.Errorf("Engine(%q) is the generic icon", engine)
		}
	}
}

func TestLucideLicenseShips(t *testing.T) {
	source, err := os.ReadFile("lucide/LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	shipped, err := os.ReadFile("../../resources/licenses/lucide-LICENSE.txt")
	if err != nil {
		t.Fatalf("the app resources do not carry the Lucide license: %v", err)
	}
	if !bytes.Equal(shipped, source) {
		t.Fatal("resources/licenses/lucide-LICENSE.txt differs from the Lucide license of the icons")
	}
}
