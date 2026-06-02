package configcmd

import (
	"strconv"
	"strings"

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

	FeishuEnabled   bool
	FeishuAppID     string
	FeishuAppSecret string
	FeishuBotName   string

	DingtalkEnabled      bool
	DingtalkClientID     string
	DingtalkClientSecret string
	DingtalkBotName      string

	WecomEnabled bool
	WecomBotID   string
	WecomSecret  string
	WecomBotName string

	TelegramEnabled  bool
	TelegramToken    string
	TelegramAuthCode string
	TelegramBotName  string

	WeixinEnabled    bool
	WeixinToken      string
	WeixinBaseURL    string
	WeixinCDNBaseURL string
	WeixinUserID     string
	WeixinBotName    string

	LinearEnabled      bool
	LinearAPIKey       string
	LinearLabelName    string
	LinearPollInterval string
	LinearProjects     string // comma-separated user input
	LinearProjectsYAML string // rendered into the YAML inline list, e.g. `"a", "b"`
	LinearStates       string // comma-separated user input
	LinearStatesYAML   string // rendered into the YAML inline list
	LinearMaxMediaSize string

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
	FFprobePath            string
	FFmpegPath             string

	AuthUsername   string
	AuthPassword   string
	AuthJWTSecret  string
	AuthAccessTTL  string
	AuthRefreshTTL string
}

// loadExistingConfig tries to load an existing config file and convert it to configValues
// for use as defaults in the interactive prompts.
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
		FeishuEnabled:          cfg.Bee.Platforms.Feishu.Enabled,
		FeishuAppID:            cfg.Bee.Platforms.Feishu.AppID,
		FeishuAppSecret:        cfg.Bee.Platforms.Feishu.AppSecret,
		FeishuBotName:          cfg.Bee.Platforms.Feishu.BotName,
		DingtalkEnabled:        cfg.Bee.Platforms.DingTalk.Enabled,
		DingtalkClientID:       cfg.Bee.Platforms.DingTalk.ClientID,
		DingtalkClientSecret:   cfg.Bee.Platforms.DingTalk.ClientSecret,
		DingtalkBotName:        cfg.Bee.Platforms.DingTalk.BotName,
		WecomEnabled:           cfg.Bee.Platforms.WeCom.Enabled,
		WecomBotID:             cfg.Bee.Platforms.WeCom.BotID,
		WecomSecret:            cfg.Bee.Platforms.WeCom.Secret,
		WecomBotName:           cfg.Bee.Platforms.WeCom.BotName,
		TelegramEnabled:        cfg.Bee.Platforms.Telegram.Enabled,
		TelegramToken:          cfg.Bee.Platforms.Telegram.Token,
		TelegramAuthCode:       cfg.Bee.Platforms.Telegram.AuthCode,
		TelegramBotName:        cfg.Bee.Platforms.Telegram.BotName,
		WeixinEnabled:          cfg.Bee.Platforms.Weixin.Enabled,
		WeixinToken:            cfg.Bee.Platforms.Weixin.Token,
		WeixinBaseURL:          cfg.Bee.Platforms.Weixin.BaseURL,
		WeixinCDNBaseURL:       cfg.Bee.Platforms.Weixin.CDNBaseURL,
		WeixinUserID:           cfg.Bee.Platforms.Weixin.UserID,
		WeixinBotName:          cfg.Bee.Platforms.Weixin.BotName,
		LinearEnabled:          cfg.Bee.Platforms.Linear.Enabled,
		LinearAPIKey:           cfg.Bee.Platforms.Linear.APIKey,
		LinearLabelName:        cfg.Bee.Platforms.Linear.LabelName,
		LinearPollInterval:     cfg.Bee.Platforms.Linear.PollInterval.String(),
		LinearProjects:         strings.Join(cfg.Bee.Platforms.Linear.Projects, ","),
		LinearStates:           strings.Join(cfg.Bee.Platforms.Linear.States, ","),
		LinearMaxMediaSize:     strconv.Itoa(cfg.Bee.Platforms.Linear.MaxMediaSize),
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
		FFprobePath:            cfg.Bee.Media.FFprobePath,
		FFmpegPath:             cfg.Bee.Media.FFmpegPath,
		AuthUsername:           cfg.Server.Auth.Username,
		AuthPassword:           cfg.Server.Auth.Password,
		AuthJWTSecret:          cfg.Server.Auth.JWTSecret,
		AuthAccessTTL:          cfg.Server.Auth.AccessTokenTTL.String(),
		AuthRefreshTTL:         cfg.Server.Auth.RefreshTokenTTL.String(),
	}
}
