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
	MCPPath = ai.MCPPath
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

type BeeConfig struct {
	MessageDebounce time.Duration       `yaml:"message_debounce"`
	Engine          EngineDefaultConfig `yaml:"engine"`
	Feeder          FeederConfig        `yaml:"feeder"`
	LegacyEngines   map[string]any      `yaml:"engines"`
	LegacyPlatforms map[string]any      `yaml:"platforms"`
	RPC             RPCConfig           `yaml:"rpc"`

	RPCBaseURL string `yaml:"-"`
}

func (b BeeConfig) WorkerTimeout() time.Duration {
	return b.Engine.Timeout.Worker
}

func (b BeeConfig) LegacyWarnings() []string {
	var warnings []string
	if len(b.LegacyPlatforms) > 0 {
		warnings = append(warnings, "bee.platforms is no longer used; manage plugins with openbee plugin or the web UI")
	}
	if len(b.LegacyEngines) > 0 {
		warnings = append(warnings, "bee.engines is no longer used; install engine plugins with openbee plugin or the web UI")
	}
	return warnings
}

type FeederConfig struct {
	MaxConcurrentBee int `yaml:"max_concurrent_bee"`
}

type RPCConfig struct {
	TokenSecret          string        `yaml:"token_secret"`
	TokenTTL             time.Duration `yaml:"token_ttl"`
	TokenSecretGenerated bool          `yaml:"-"`
}

type AuthConfig struct {
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
	if cfg.Server.Auth.AccessTokenTTL == 0 {
		cfg.Server.Auth.AccessTokenTTL = 2 * time.Hour
	}
	if cfg.Server.Auth.RefreshTokenTTL == 0 {
		cfg.Server.Auth.RefreshTokenTTL = 7 * 24 * time.Hour
	}
	if cfg.Bee.RPC.TokenSecret == "" {
		cfg.Bee.RPC.TokenSecret = GenerateRandomSecret()
		cfg.Bee.RPC.TokenSecretGenerated = true
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
