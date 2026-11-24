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
		for _, name := range []string{"logo-mark", "logo-mark-small", "logo-full"} {
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

func TestEngineIcon(t *testing.T) {
	generic := Get("cpu")
	if Engine("") != generic {
		t.Error("an engine without an icon does not get the generic icon")
	}
	if Engine("<svg") != generic || Engine("not svg") != generic {
		t.Error("an icon that fails to parse does not fall back to the generic icon")
	}
	plugin := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="M0 0h24v24H0z"/></svg>`
	icon := Engine(plugin)
	if icon == nil || icon == generic {
		t.Fatal("a plugin icon is not parsed")
	}
	if Engine(plugin) != icon {
		t.Error("Engine does not cache the parsed plugin icon")
	}
	if Engine(strings.Replace(plugin, "24H0z", "12H0z", 1)) == icon {
		t.Error("different plugin icons share one image")
	}
}

func TestPluginIcon(t *testing.T) {
	if Plugin("") != nil {
		t.Error("a plugin without an icon gets one")
	}
	if Plugin("<svg") != nil || Plugin("not svg") != nil {
		t.Error("an icon that fails to parse is not nil")
	}
	stroked := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48"><g fill="none"><path stroke="currentColor" stroke-width="4" d="M4 24h40"/></g></svg>`
	icon := Plugin(stroked)
	if icon == nil {
		t.Fatal("a stroked plugin icon is not parsed")
	}
	if Plugin(stroked) != icon {
		t.Error("Plugin does not cache the parsed icon")
	}
	if Engine(stroked) != icon {
		t.Error("Engine and Plugin parse one icon twice")
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

func TestLogoMarkSmallIsDistinct(t *testing.T) {
	tk := theme.Light()
	if LogoMarkSmall(tk) == nil {
		t.Fatal("LogoMarkSmall is nil")
	}
	if LogoMarkSmall(tk) == LogoMark(tk) {
		t.Error("LogoMarkSmall returns the standard mark")
	}
	if bytes.Equal(brandData("logo-mark-small"), brandData("logo-mark")) {
		t.Error("logo-mark-small.svg is identical to logo-mark.svg")
	}
}
