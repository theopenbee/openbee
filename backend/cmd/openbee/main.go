package main

import (
	"errors"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/cmd/openbee/internal/cli"
	"github.com/theopenbee/openbee/internal/codemode"
	"github.com/theopenbee/openbee/internal/infra/buildinfo"
	"github.com/theopenbee/openbee/internal/infra/logger"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	codemode.MaybeRunChild()

	// Publish build metadata so non-CLI layers (e.g. the HTTP API) can read it
	// without threading the values through every constructor.
	buildinfo.Set(version, commit, date)

	root := cli.NewRoot(cli.BuildInfo{Version: version, Commit: commit, Date: date})
	if err := root.Execute(); err != nil {
		var ece *cli.ExitCodeError
		if errors.As(err, &ece) {
			os.Exit(ece.Code)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		logger.Error("fatal", zap.Error(err))
		os.Exit(1)
	}
}
