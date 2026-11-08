package desktop

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
	nativeupdater "github.com/egoist/mygo/plugins/updater/native"
)

const windowsUninstaller = "Uninstall.exe"

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
	_, err := os.Stat(filepath.Join(filepath.Dir(exe), windowsUninstaller))
	return err == nil
}
