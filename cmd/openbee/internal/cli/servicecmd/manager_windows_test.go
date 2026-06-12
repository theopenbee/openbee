//go:build windows

package servicecmd

import (
	"strings"
	"testing"
)

func TestRenderSchtaskXML(t *testing.T) {
	got, err := renderSchtaskXML(schtaskTemplateData{
		UserId:     "DESKTOP-A\\me",
		ExePath:    `C:\Program Files\openbee\openbee.exe`,
		ConfigPath: `C:\Users\me\.openbee\config.yaml`,
		LogPath:    `C:\Users\me\.openbee\daemon.log`,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<UserId>DESKTOP-A\\me</UserId>",
		"<RunLevel>LeastPrivilege</RunLevel>",
		"<Hidden>true</Hidden>",
		"<Interval>PT1M</Interval>",
		"openbee.exe",
		"server -c",
		"daemon.log",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("XML missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestEncodeUTF16LE_HasBOM(t *testing.T) {
	got := encodeUTF16LE("A")
	if len(got) != 4 {
		t.Fatalf("want 4 bytes, got %d", len(got))
	}
	if got[0] != 0xFF || got[1] != 0xFE {
		t.Errorf("missing BOM: %x %x", got[0], got[1])
	}
	if got[2] != 'A' || got[3] != 0 {
		t.Errorf("wrong encoding: %x %x", got[2], got[3])
	}
}
