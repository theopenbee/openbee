package main

// compat.go provides package-level symbols that upgrade.go still references
// after the daemon and backup commands were moved to their own sub-packages.
// These stubs will be removed when Tasks 6-7 migrate those commands.

import "github.com/theopenbee/openbee/cmd/openbee/internal/cli"

// resolveExecutable delegates to cli.ResolveExecutable.
func resolveExecutable() (string, error) { return cli.ResolveExecutable() }
