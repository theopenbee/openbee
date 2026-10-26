package configcmd

import (
	"strconv"

	"github.com/theopenbee/openbee/internal/infra/config"
)

type configValues struct {
	Language        string
	ServerPort      string
	ServerHost      string
	Debug           bool
	DBPath          string
	RPCTokenSecret  string
	RPCTokenTTL     string
	ServerEnvSecret string

	EngineDefault       string
	EngineTimeoutBee    string
	EngineTimeoutWorker string
	ClaudeEnabled       bool
	CodexEnabled        bool
	PiEnabled           bool
	ClaudePath          string
	CodexPath           string
	PiPath              string

	FeederMaxConcurrentBee int
	MessageDebounce        string

	AuthUsername   string
	AuthPassword   string
	AuthJWTSecret  string
	AuthAccessTTL  string
	AuthRefreshTTL string
}

// loadExistingConfig returns nil if no usable config file exists at path.
func loadExistingConfig(path string) *configValues {
	cfg, err := config.Load(path)
	if err != nil {
		return nil
	}

	return &configValues{
		Language:               cfg.Language,
		ServerPort:             strconv.Itoa(cfg.Server.Port),
		ServerHost:             cfg.Server.Host,
		Debug:                  cfg.Server.Debug,
		DBPath:                 cfg.Database.Path,
		RPCTokenSecret:         cfg.Bee.RPC.TokenSecret,
		RPCTokenTTL:            cfg.Bee.RPC.TokenTTL.String(),
		ServerEnvSecret:        cfg.Server.EnvSecret,
		EngineDefault:          cfg.Bee.Engine.Default,
		EngineTimeoutBee:       cfg.Bee.Engine.Timeout.Bee.String(),
		EngineTimeoutWorker:    cfg.Bee.Engine.Timeout.Worker.String(),
		ClaudeEnabled:          cfg.Bee.Engines.Claude.Enabled,
		CodexEnabled:           cfg.Bee.Engines.Codex.Enabled,
		PiEnabled:              cfg.Bee.Engines.Pi.Enabled,
		ClaudePath:             cfg.Bee.Engines.Claude.Path,
		CodexPath:              cfg.Bee.Engines.Codex.Path,
		PiPath:                 cfg.Bee.Engines.Pi.Path,
		FeederMaxConcurrentBee: cfg.Bee.Feeder.MaxConcurrentBee,
		MessageDebounce:        cfg.Bee.MessageDebounce.String(),
		AuthUsername:           cfg.Server.Auth.Username,
		AuthPassword:           cfg.Server.Auth.Password,
		AuthJWTSecret:          cfg.Server.Auth.JWTSecret,
		AuthAccessTTL:          cfg.Server.Auth.AccessTokenTTL.String(),
		AuthRefreshTTL:         cfg.Server.Auth.RefreshTokenTTL.String(),
	}
}
