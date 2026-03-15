package main

import (
	"log/slog"
	"os"

	"github.com/robobee/core/internal/config"
)

func main() {
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	app, err := buildApp(cfg)
	if err != nil {
		slog.Error("failed to build app", "error", err)
		os.Exit(1)
	}

	app.Run()
}
