package main

// compat.go provides package-level symbols that backup.go, restore.go, and
// upgrade.go still reference after the daemon commands were moved to daemoncmd.
// These stubs will be removed when Tasks 5-7 migrate those commands into their
// own sub-packages.

import (
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli"
	"github.com/theopenbee/openbee/cmd/openbee/internal/cli/daemoncmd"
)

// cfgPath is the shared --config flag value used by backup and restore commands.
// (server's cfgPath has moved into daemoncmd as a package var there.)
var cfgPath string

// openbeeStateDir delegates to daemoncmd.OpenbeeStateDir.
func openbeeStateDir() string { return daemoncmd.OpenbeeStateDir() }

// daemonPIDFile delegates to daemoncmd.DaemonPIDFile.
func daemonPIDFile() string { return daemoncmd.DaemonPIDFile() }

// doStop delegates to daemoncmd.DoStop.
func doStop(pidFile string) error { return daemoncmd.DoStop(pidFile) }

// resolveExecutable delegates to cli.ResolveExecutable.
func resolveExecutable() (string, error) { return cli.ResolveExecutable() }
