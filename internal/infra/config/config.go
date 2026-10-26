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

type BeeConfig struct {
	MessageDebounce time.Duration       `yaml:"message_debounce"`
	Engine          EngineDefaultConfig `yaml:"engine"`
	Engines         EnginesConfig       `yaml:"engines"`
	Feeder          FeederConfig        `yaml:"feeder"`
	LegacyPlatforms map[string]any      `yaml:"platforms"`
	RPC             RPCConfig           `yaml:"rpc"`

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

type FeederConfig struct {
	MaxConcurrentBee int `yaml:"max_concurrent_bee"`
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
	Port               int        `yaml:"port"`
	Host               string     `yaml:"host"`
	Debug              bool       `yaml:"debug"`
	Auth               AuthConfig `yaml:"auth"`
	EnvSecret          string     `yaml:"env_secret"`
	EnvSecretGenerated bool       `yaml:"-"`
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
		cfg.Server.EnvSecretGenerated = true
	}
	if cfg.Bee.RPC.TokenTTL == 0 {
		cfg.Bee.RPC.TokenTTL = 48 * time.Hour
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
