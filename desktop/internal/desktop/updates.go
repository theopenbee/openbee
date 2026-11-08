package desktop

import (
	"os"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	nativeupdater "github.com/egoist/mygo/plugins/updater/native"
)

const (
	windowsExe = "openbee.exe"
	scoopDir   = `\apps\openbee-desktop\`
)

func useUpdater() bool {
	exe, err := os.Executable()
	if err != nil || !SelfUpdates(runtime.GOOS, exe) || !mygo.Updater.Enabled() {
		return false
	}
	mygo.Use(nativeupdater.Plugin)
	return true
}

func SelfUpdates(goos, exe string) bool {
	if goos != "windows" {
		return true
	}
	path := strings.ToLower(strings.ReplaceAll(exe, "/", `\`))
	name := path[strings.LastIndex(path, `\`)+1:]
	return name == windowsExe && !strings.Contains(path, scoopDir)
}
