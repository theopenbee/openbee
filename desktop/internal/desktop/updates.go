package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
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

type appUpdater struct{}

func (appUpdater) Check()                        { updater.CheckForUpdates() }
func (appUpdater) LastCheck() time.Time          { return updater.LastCheck() }
func (appUpdater) AutomaticChecks() bool         { return updater.AutomaticChecks() }
func (appUpdater) SetAutomaticChecks(on bool)    { updater.SetAutomaticChecks(on) }
func (appUpdater) AutomaticDownloads() bool      { return updater.AutomaticDownloads() }
func (appUpdater) SetAutomaticDownloads(on bool) { updater.SetAutomaticDownloads(on) }
