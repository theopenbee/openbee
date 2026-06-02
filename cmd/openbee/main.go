package main

import (
	"errors"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/theopenbee/openbee/cmd/openbee/internal/cli"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/logger"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	lang := cli.DetectLang()
	if err := i18n.Load(lang); err != nil {
		fmt.Fprintf(os.Stderr, "warning: i18n load failed: %v\n", err)
	}

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
