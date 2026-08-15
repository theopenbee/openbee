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

const (
	RPCBeeBasePath = "/rpc/bee"
)

//go:embed config.yaml.tmpl
var ConfigTemplate string

func DefaultBeeWorkDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openbee", "bee")
}

func DefaultWorkerBaseDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openbee", "worker")
}

func DefaultLogsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openbee", "logs")
}

func DefaultCodexSessionsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openbee", ".codex", "sessions")
}

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

type EngineTimeoutConfig struct {
	Bee    time.Duration `yaml:"bee"`
	Worker time.Duration `yaml:"worker"`
}

type EngineDefaultConfig struct {
	Default string              `yaml:"default"`
	Timeout EngineTimeoutConfig `yaml:"timeout"`
}

type EngineItemConfig struct {
	Enabled bool              `yaml:"enabled"`
	Path    string            `yaml:"path"`
	Env     map[string]string `yaml:"env"`
}

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

	RPCBaseURL string `yaml:"-"`
}

func (b BeeConfig) WorkerTimeout() time.Duration {
	return b.Engine.Timeout.Worker
}

func (b BeeConfig) EffectiveEngine() string {
	if b.Engine.Default != "" {
		return b.Engine.Default
	}
	return ai.EngineClaude
}

func (b BeeConfig) EngineConfigRaw() map[string]any {
	return b.EngineConfigRawFor(b.EffectiveEngine())
}

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
	MaxMediaSize int    `yaml:"max_media_size"`
	BotName      string `yaml:"bot_name"`      
}

type DingTalkConfig struct {
	Enabled      bool   `yaml:"enabled"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	BotName      string `yaml:"bot_name"` 
}

type WeComConfig struct {
	Enabled      bool   `yaml:"enabled"`
	BotID        string `yaml:"bot_id"`
	Secret       string `yaml:"secret"`
	WebSocketURL string `yaml:"websocket_url"`
	BotName      string `yaml:"bot_name"`
}

type TelegramConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Token        string `yaml:"token"`
	MaxMediaSize int    `yaml:"max_media_size"` 
	AuthCode     string `yaml:"auth_code"`    
	BotName      string `yaml:"bot_name"`     
}

type WeixinConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Token        string `yaml:"token"`
	BaseURL      string `yaml:"base_url"`
	CDNBaseURL   string `yaml:"cdn_base_url"`
	RouteTag     int    `yaml:"route_tag"`
	UserID       string `yaml:"user_id"`
	MaxMediaSize int    `yaml:"max_media_size"` 
	BotName      string `yaml:"bot_name"`      
}

type LinearConfig struct {
	Enabled      bool          `yaml:"enabled"`
	APIKey       string        `yaml:"api_key"`       
	LabelName    string        `yaml:"label_name"`    
	PollInterval time.Duration `yaml:"poll_interval"`  
	Projects     []string      `yaml:"projects"`      
	States       []string      `yaml:"states"`        
	MaxMediaSize int           `yaml:"max_media_size"` 
}

type RPCConfig struct {
	TokenSecret string        `yaml:"token_secret"`
	TokenTTL    time.Duration `yaml:"token_ttl"` 
}

type AuthConfig struct {
	Username        string        `yaml:"username"`          
	Password        string        `yaml:"password"`         
	JWTSecret       string        `yaml:"jwt_secret"`       
	AccessTokenTTL  time.Duration `yaml:"access_token_ttl"` 
	RefreshTokenTTL time.Duration `yaml:"refresh_token_ttl"`
}

type ServerConfig struct {
	Port  int        `yaml:"port"`
	Host  string     `yaml:"host"`
	Debug bool       `yaml:"debug"`
	Auth  AuthConfig `yaml:"auth"`
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
	if cfg.Server.Auth.AccessTokenTTL == 0 {
		cfg.Server.Auth.AccessTokenTTL = 2 * time.Hour
	}
	if cfg.Server.Auth.RefreshTokenTTL == 0 {
		cfg.Server.Auth.RefreshTokenTTL = 7 * 24 * time.Hour
	}
	if cfg.Bee.RPC.TokenSecret == "" {
		cfg.Bee.RPC.TokenSecret = GenerateRandomSecret()
	}
	if cfg.Server.EnvSecret == "" {
		cfg.Server.EnvSecret = GenerateRandomSecret()
	}
	if cfg.Bee.RPC.TokenTTL == 0 {
		cfg.Bee.RPC.TokenTTL = 48 * time.Hour
	}
	if cfg.Bee.Platforms.Linear.LabelName == "" {
		cfg.Bee.Platforms.Linear.LabelName = "openbee"
	}
	if cfg.Bee.Platforms.Linear.PollInterval == 0 {
		cfg.Bee.Platforms.Linear.PollInterval = 10 * time.Second
	}
	return nil
}

func GenerateRandomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return fmt.Sprintf("%x", b)
}

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
