package config

import (
	"crypto/rand"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	ai "github.com/theopenbee/openbee/internal/ai"
)

// RPC endpoint path prefixes.
const (
	RPCBeeBasePath = "/rpc/bee"
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

// DefaultCodexSessionsDir returns the codex session store directory: ~/.openbee/.codex/sessions
func DefaultCodexSessionsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openbee", ".codex", "sessions")
}

// DefaultPiSessionsDir returns the pi session store directory: ~/.openbee/.pi/sessions
func DefaultPiSessionsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openbee", ".pi", "sessions")
}

type Config struct {
	Language string         `yaml:"language"`
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Bee      BeeConfig      `yaml:"bee"`
}

// EngineTimeoutConfig holds separate timeout durations for the bee and worker roles.
type EngineTimeoutConfig struct {
	Bee    time.Duration `yaml:"bee"`
	Worker time.Duration `yaml:"worker"`
}

// EngineDefaultConfig holds the global engine default name and per-role timeouts.
type EngineDefaultConfig struct {
	Default string              `yaml:"default"`
	Timeout EngineTimeoutConfig `yaml:"timeout"`
}

// EngineItemConfig is the per-engine enable/path config.
type EngineItemConfig struct {
	Enabled bool              `yaml:"enabled"`
	Path    string            `yaml:"path"`
	Env     map[string]string `yaml:"env"`
}

// EnginesConfig groups all per-engine configs under the engines: YAML namespace.
type EnginesConfig struct {
	Claude EngineItemConfig `yaml:"claude"`
	Codex  EngineItemConfig `yaml:"codex"`
	Pi     EngineItemConfig `yaml:"pi"`
}

func (e EnginesConfig) itemFor(name string) EngineItemConfig {
	switch name {
	case ai.EngineClaude:
		return e.Claude
	case ai.EngineCodex:
		return e.Codex
	case ai.EnginePi:
		return e.Pi
	}
	return EngineItemConfig{}
}

// IsEnabled reports whether the named engine is enabled.
func (e EnginesConfig) IsEnabled(name string) bool { return e.itemFor(name).Enabled }

type MediaConfig struct {
	FFprobePath string `yaml:"ffprobe_path"`
	FFmpegPath  string `yaml:"ffmpeg_path"`
}

type BeeConfig struct {
	MessageDebounce time.Duration       `yaml:"message_debounce"`
	Engine          EngineDefaultConfig `yaml:"engine"`
	Engines         EnginesConfig       `yaml:"engines"`
	Feeder          FeederConfig        `yaml:"feeder"`
	Platforms       PlatformsConfig     `yaml:"platforms"`
	RPC             RPCConfig           `yaml:"rpc"`
	Media           MediaConfig         `yaml:"media"`

	// Derived fields — not in YAML, computed by Load()
	RPCBaseURL string `yaml:"-"` // http://host:port (no path suffix)
}

// WorkerTimeout returns the worker engine execution timeout.
func (b BeeConfig) WorkerTimeout() time.Duration {
	return b.Engine.Timeout.Worker
}

// EffectiveEngine returns the configured default engine name, defaulting to "claude".
func (b BeeConfig) EffectiveEngine() string {
	if b.Engine.Default != "" {
		return b.Engine.Default
	}
	return ai.EngineClaude
}

// EngineConfigRaw returns the raw config map for the default engine.
func (b BeeConfig) EngineConfigRaw() map[string]any {
	return b.EngineConfigRawFor(b.EffectiveEngine())
}

// EngineConfigRawFor returns the raw config map for the named engine.
func (b BeeConfig) EngineConfigRawFor(name string) map[string]any {
	item := b.Engines.itemFor(name)
	if item.Path == "" {
		return nil
	}
	return map[string]any{
		"path": item.Path,
		"env":  item.Env,
	}
}

type PlatformsConfig struct {
	Feishu   FeishuConfig   `yaml:"feishu"`
	DingTalk DingTalkConfig `yaml:"dingtalk"`
	WeCom    WeComConfig    `yaml:"wecom"`
	Telegram TelegramConfig `yaml:"telegram"`
	Weixin   WeixinConfig   `yaml:"weixin"`
	Linear   LinearConfig   `yaml:"linear"`
}

func (p PlatformsConfig) BotNames() []string {
	var names []string
	for _, n := range []string{
		p.Feishu.BotName,
		p.DingTalk.BotName,
		p.WeCom.BotName,
		p.Telegram.BotName,
		p.Weixin.BotName,
	} {
		if n != "" {
			names = append(names, n)
		}
	}
	return names
}

type FeederConfig struct {
	MaxConcurrentBee int `yaml:"max_concurrent_bee"`
}

type FeishuConfig struct {
	Enabled      bool   `yaml:"enabled"`
	AppID        string `yaml:"app_id"`
	AppSecret    string `yaml:"app_secret"`
	MaxMediaSize int    `yaml:"max_media_size"` // maximum media download size in bytes; default 100 MB
	BotName      string `yaml:"bot_name"`       // bot display name used to strip @mention in group commands
}

type DingTalkConfig struct {
	Enabled      bool   `yaml:"enabled"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	BotName      string `yaml:"bot_name"` // bot display name used to strip @mention in group commands
}

type WeComConfig struct {
	Enabled      bool   `yaml:"enabled"`
	BotID        string `yaml:"bot_id"`
	Secret       string `yaml:"secret"`
	WebSocketURL string `yaml:"websocket_url"`
	BotName      string `yaml:"bot_name"` // bot display name used to strip @mention in group commands
}

type TelegramConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Token        string `yaml:"token"`
	MaxMediaSize int    `yaml:"max_media_size"` // bytes; default 50MB
	AuthCode     string `yaml:"auth_code"`      // passcode for user authorization; empty = no auth required
	BotName      string `yaml:"bot_name"`       // bot display name used to strip @mention in group commands
}

type WeixinConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Token        string `yaml:"token"`
	BaseURL      string `yaml:"base_url"`
	CDNBaseURL   string `yaml:"cdn_base_url"`
	RouteTag     int    `yaml:"route_tag"`
	UserID       string `yaml:"user_id"`
	MaxMediaSize int    `yaml:"max_media_size"` // bytes; default 100MB
	BotName      string `yaml:"bot_name"`       // bot display name used to strip @mention in group commands
}

type LinearConfig struct {
	Enabled      bool          `yaml:"enabled"`
	APIKey       string        `yaml:"api_key"`        // Linear personal API key (required when enabled)
	LabelName    string        `yaml:"label_name"`     // gating label; default "openbee"
	PollInterval time.Duration `yaml:"poll_interval"`  // default 10s
	Projects     []string      `yaml:"projects"`       // project name allow-list; empty = process nothing
	States       []string      `yaml:"states"`         // workflow-state name allow-list; empty = skip
	MaxMediaSize int           `yaml:"max_media_size"` // bytes; default 50 MB
}

type RPCConfig struct {
	TokenSecret string        `yaml:"token_secret"` // HMAC-SHA256 secret; empty = auto-generated on startup
	TokenTTL    time.Duration `yaml:"token_ttl"`    // token validity period; default 2h
}

type AuthConfig struct {
	Username        string        `yaml:"username"`          // DEPRECATED: web login now uses DB users; ignored for login
	Password        string        `yaml:"password"`          // DEPRECATED: web login now uses DB users; ignored for login
	JWTSecret       string        `yaml:"jwt_secret"`        // HMAC-SHA256 secret; empty = auto-generated on startup
	AccessTokenTTL  time.Duration `yaml:"access_token_ttl"`  // access token lifetime; default 2h
	RefreshTokenTTL time.Duration `yaml:"refresh_token_ttl"` // refresh token lifetime; default 7d
}

type ServerConfig struct {
	Port  int        `yaml:"port"`
	Host  string     `yaml:"host"`
	Debug bool       `yaml:"debug"`
	Auth  AuthConfig `yaml:"auth"`
	// EnvSecret is a hex-encoded 32-byte (64 hex chars) key used for AES-256-GCM
	// encryption of env var values stored in bee_env_configs. Auto-generated by
	// applyDefaults() if empty.
	//
	// WARNING: Rotating this secret after env configs have been stored will make
	// all existing encrypted values unreadable. Workers and bees will fail to
	// launch until the affected rows are deleted or re-created.
	EnvSecret string `yaml:"env_secret"`
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
	cfg.Bee.RPCBaseURL = fmt.Sprintf("http://%s:%d", host, cfg.Server.Port)
	return cfg, nil
}

func applyDefaults(cfg *Config) error {
	if cfg.Bee.MessageDebounce == 0 {
		cfg.Bee.MessageDebounce = 300 * time.Millisecond
	}
	if cfg.Bee.Feeder.MaxConcurrentBee == 0 {
		cfg.Bee.Feeder.MaxConcurrentBee = 5
	}
	if cfg.Bee.Engine.Timeout.Bee == 0 {
		cfg.Bee.Engine.Timeout.Bee = 5 * time.Minute
	}
	if cfg.Bee.Engine.Timeout.Worker == 0 {
		cfg.Bee.Engine.Timeout.Worker = 30 * time.Minute
	}
	if cfg.Bee.Engines.Claude.Path == "" {
		cfg.Bee.Engines.Claude.Path = "claude"
	}
	if cfg.Bee.Engines.Codex.Path == "" {
		cfg.Bee.Engines.Codex.Path = "codex"
	}
	if cfg.Bee.Engines.Pi.Path == "" {
		cfg.Bee.Engines.Pi.Path = "pi"
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
	if cfg.Bee.Platforms.Linear.MaxMediaSize == 0 {
		cfg.Bee.Platforms.Linear.MaxMediaSize = 50 * 1024 * 1024 // 50MB
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
	if cfg.Bee.RPC.TokenSecret == "" {
		cfg.Bee.RPC.TokenSecret = GenerateRandomSecret()
	}
	if cfg.Server.EnvSecret == "" {
		cfg.Server.EnvSecret = GenerateRandomSecret()
	}
	if cfg.Bee.RPC.TokenTTL == 0 {
		cfg.Bee.RPC.TokenTTL = 2 * time.Hour
	}
	if cfg.Bee.Platforms.Linear.LabelName == "" {
		cfg.Bee.Platforms.Linear.LabelName = "openbee"
	}
	if cfg.Bee.Platforms.Linear.PollInterval == 0 {
		cfg.Bee.Platforms.Linear.PollInterval = 10 * time.Second
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
