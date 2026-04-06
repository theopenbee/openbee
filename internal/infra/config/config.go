package config

import (
	"crypto/rand"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// MCP endpoint path prefixes.
const (
	MCPBeeBasePath    = "/mcp/bee"
	MCPWorkerBasePath = "/mcp/worker"
)

//go:embed config.yaml.tmpl
var ConfigTemplate string

// DefaultBeeWorkDir returns the hardcoded bee working directory: ~/.openbee/bee
func DefaultBeeWorkDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openbee", "bee")
}

// DefaultWorkerBaseDir returns the hardcoded worker base directory: ~/.openbee/worker
func DefaultWorkerBaseDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openbee", "worker")
}

// DefaultLogsDir returns the execution log directory: ~/.openbee/logs
func DefaultLogsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openbee", "logs")
}

type Config struct {
	Language string         `yaml:"language"`
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Bee      BeeConfig      `yaml:"bee"`
}

type ClaudeConfig struct {
	Path    string        `yaml:"path"`
	Timeout time.Duration `yaml:"timeout"`
}

type MediaConfig struct {
	FFprobePath string `yaml:"ffprobe_path"`
	FFmpegPath  string `yaml:"ffmpeg_path"`
}

type BeeConfig struct {
	MessageDebounce time.Duration   `yaml:"message_debounce"`
	Engine          string          `yaml:"engine"`
	Claude          ClaudeConfig    `yaml:"claude"`
	Feeder          FeederConfig    `yaml:"feeder"`
	Platforms       PlatformsConfig `yaml:"platforms"`
	MCP             MCPConfig       `yaml:"mcp"`
	Media           MediaConfig     `yaml:"media"`

	// Derived fields — not in YAML, computed by Load()
	MCPBaseURL string `yaml:"-"` // http://host:port (no path suffix)
}

// EngineConfigRaw returns the raw config map for the selected engine.
func (b BeeConfig) EngineConfigRaw() map[string]any {
	name := b.Engine
	if name == "" {
		name = "claude"
	}
	switch name {
	case "claude":
		return map[string]any{
			"path":    b.Claude.Path,
			"timeout": b.Claude.Timeout,
		}
	default:
		return nil
	}
}

type PlatformsConfig struct {
	Feishu   FeishuConfig   `yaml:"feishu"`
	DingTalk DingTalkConfig `yaml:"dingtalk"`
	WeCom    WeComConfig    `yaml:"wecom"`
	Telegram TelegramConfig `yaml:"telegram"`
	Weixin   WeixinConfig   `yaml:"weixin"`
}

type FeederConfig struct {
	Timeout          time.Duration `yaml:"timeout"`
	MaxConcurrentBee int           `yaml:"max_concurrent_bee"`
}

type FeishuConfig struct {
	Enabled      bool   `yaml:"enabled"`
	AppID        string `yaml:"app_id"`
	AppSecret    string `yaml:"app_secret"`
	MaxMediaSize int    `yaml:"max_media_size"` // maximum media download size in bytes; default 100 MB
}

type DingTalkConfig struct {
	Enabled      bool   `yaml:"enabled"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

type WeComConfig struct {
	Enabled      bool   `yaml:"enabled"`
	BotID        string `yaml:"bot_id"`
	Secret       string `yaml:"secret"`
	WebSocketURL string `yaml:"websocket_url"`
}

type TelegramConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Token        string `yaml:"token"`
	MaxMediaSize int    `yaml:"max_media_size"` // bytes; default 50MB
	AuthCode     string `yaml:"auth_code"`      // passcode for user authorization; empty = no auth required
}

type WeixinConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Token        string `yaml:"token"`
	BaseURL      string `yaml:"base_url"`
	CDNBaseURL   string `yaml:"cdn_base_url"`
	RouteTag     int    `yaml:"route_tag"`
	UserID       string `yaml:"user_id"`
	MaxMediaSize int    `yaml:"max_media_size"` // bytes; default 100MB
}

type MCPConfig struct {
	TokenSecret string        `yaml:"token_secret"` // HMAC-SHA256 secret; empty = auto-generated on startup
	TokenTTL    time.Duration `yaml:"token_ttl"`    // token validity period; default 2h
}


type AuthConfig struct {
	Username        string        `yaml:"username"`          // login username; default "admin"
	Password        string        `yaml:"password"`          // login password; empty = auto-generated on startup
	JWTSecret       string        `yaml:"jwt_secret"`        // HMAC-SHA256 secret; empty = auto-generated on startup
	AccessTokenTTL  time.Duration `yaml:"access_token_ttl"`  // access token lifetime; default 2h
	RefreshTokenTTL time.Duration `yaml:"refresh_token_ttl"` // refresh token lifetime; default 7d
}

type ServerConfig struct {
	Port  int        `yaml:"port"`
	Host  string     `yaml:"host"`
	Debug bool       `yaml:"debug"`
	Auth  AuthConfig `yaml:"auth"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}


func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if err := applyDefaults(&cfg); err != nil {
		return Config{}, err
	}
	host := cfg.Server.Host
	if host == "" {
		host = "localhost"
	}
	cfg.Bee.MCPBaseURL = fmt.Sprintf("http://%s:%d", host, cfg.Server.Port)
	return cfg, nil
}

func applyDefaults(cfg *Config) error {
	if cfg.Bee.MessageDebounce == 0 {
		cfg.Bee.MessageDebounce = 300 * time.Millisecond
	}
	if cfg.Bee.Feeder.Timeout == 0 {
		cfg.Bee.Feeder.Timeout = 5 * time.Minute
	}
	if cfg.Bee.Feeder.MaxConcurrentBee == 0 {
		cfg.Bee.Feeder.MaxConcurrentBee = 5
	}
	if cfg.Bee.Claude.Path == "" {
		cfg.Bee.Claude.Path = "claude"
	}
	if cfg.Bee.Media.FFprobePath == "" {
		cfg.Bee.Media.FFprobePath = "ffprobe"
	}
	if cfg.Bee.Media.FFmpegPath == "" {
		cfg.Bee.Media.FFmpegPath = "ffmpeg"
	}
	if cfg.Bee.Platforms.Feishu.MaxMediaSize == 0 {
		cfg.Bee.Platforms.Feishu.MaxMediaSize = 100 * 1024 * 1024 // 100MB
	}
	if cfg.Bee.Platforms.WeCom.WebSocketURL == "" {
		cfg.Bee.Platforms.WeCom.WebSocketURL = "wss://openws.work.weixin.qq.com"
	}
	if cfg.Bee.Platforms.Telegram.MaxMediaSize == 0 {
		cfg.Bee.Platforms.Telegram.MaxMediaSize = 50 * 1024 * 1024 // 50MB
	}
	if cfg.Bee.Platforms.Weixin.BaseURL == "" {
		cfg.Bee.Platforms.Weixin.BaseURL = "https://ilinkai.weixin.qq.com"
	}
	if cfg.Bee.Platforms.Weixin.CDNBaseURL == "" {
		cfg.Bee.Platforms.Weixin.CDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
	}
	if cfg.Bee.Platforms.Weixin.MaxMediaSize == 0 {
		cfg.Bee.Platforms.Weixin.MaxMediaSize = 100 * 1024 * 1024 // 100MB
	}
	if cfg.Server.Auth.Username == "" {
		cfg.Server.Auth.Username = "admin"
	}
	if cfg.Server.Auth.Password != "" {
		if cfg.Server.Auth.AccessTokenTTL == 0 {
			cfg.Server.Auth.AccessTokenTTL = 2 * time.Hour
		}
		if cfg.Server.Auth.RefreshTokenTTL == 0 {
			cfg.Server.Auth.RefreshTokenTTL = 7 * 24 * time.Hour
		}
	}
	if cfg.Bee.MCP.TokenSecret == "" {
		cfg.Bee.MCP.TokenSecret = GenerateRandomSecret()
	}
	if cfg.Bee.MCP.TokenTTL == 0 {
		cfg.Bee.MCP.TokenTTL = 2 * time.Hour
	}
	return nil
}

// GenerateRandomSecret returns a 32-byte hex-encoded random string.
func GenerateRandomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return fmt.Sprintf("%x", b)
}

// GetLang reads the language field from the config file at path.
// Returns empty string if the file does not exist, cannot be parsed,
// or does not contain a language field. Never returns an error —
// callers fall back to the next priority in the detection chain.
func GetLang(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg struct {
		Language string `yaml:"language"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	return cfg.Language
}
