//go:build darwin || linux

package servicecmd

import "os/exec"

var execLookPath = exec.LookPath
