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
