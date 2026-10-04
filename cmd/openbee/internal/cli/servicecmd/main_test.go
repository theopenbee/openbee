package servicecmd

import (
	"os"
	"os/user"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

// testHome stands in for the install-target user's home during tests.
// resolveInstallOptions MkdirAll's <home>/.openbee when --working-dir is
// empty, so without this every such test would write to the real home — and
// fail where that home is not writable (e.g. CI users with /nonexistent).
var testHome string

func TestMain(m *testing.M) {
	if err := i18n.Load("en"); err != nil {
		panic("i18n.Load: " + err.Error())
	}
	os.Exit(runWithIsolatedHome(m))
}

func runWithIsolatedHome(m *testing.M) int {
	dir, err := os.MkdirTemp("", "servicecmd-home-*")
	if err != nil {
		panic("create test home: " + err.Error())
	}
	defer os.RemoveAll(dir)
	testHome = dir

	// Non-Linux resolves the home via os.UserHomeDir ($HOME, or %USERPROFILE%
	// on Windows); Linux via the run-as user's passwd entry, hence the
	// lookupUser override.
	for _, key := range []string{"HOME", "USERPROFILE"} {
		if err := os.Setenv(key, dir); err != nil {
			panic("set " + key + ": " + err.Error())
		}
	}
	lookupUser = func(name string) (*user.User, error) {
		u, err := user.Lookup(name)
		if err != nil {
			return nil, err
		}
		cp := *u
		cp.HomeDir = dir
		return &cp, nil
	}
	return m.Run()
}
