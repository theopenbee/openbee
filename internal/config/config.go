package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultBeeWorkDir returns the hardcoded bee working directory: ~/.robobee/bee
func DefaultBeeWorkDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".robobee", "bee")
}

// DefaultWorkerBaseDir returns the hardcoded worker base directory: ~/.robobee/worker
func DefaultWorkerBaseDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".robobee", "worker")
}

type Config struct {
	Server       ServerConfig        `yaml:"server"`
	Database     DatabaseConfig      `yaml:"database"`
	Runtime      RuntimeConfig       `yaml:"runtime"`
	MessageQueue MessageQueueConfig  `yaml:"message_queue"`
	MCP          MCPConfig           `yaml:"mcp"`
	Bee          BeeConfig           `yaml:"bee"`
}

type BeeConfig struct {
	Feeder    FeederConfig    `yaml:"feeder"`
	Platforms PlatformsConfig `yaml:"platforms"`

	// Derived fields — not in YAML, computed by Load()
	MCPBaseURL string `yaml:"-"` // http://host:port (no path suffix)
	MCPAPIKey  string `yaml:"-"` // copied from MCPConfig.APIKey
	Binary     string `yaml:"-"` // copied from Runtime.ClaudeCode.Binary
}

type PlatformsConfig struct {
	Feishu   FeishuConfig   `yaml:"feishu"`
	DingTalk DingTalkConfig `yaml:"dingtalk"`
}

type FeederConfig struct {
	Interval           time.Duration `yaml:"interval"`
	BatchSize          int           `yaml:"batch_size"`
	Timeout            time.Duration `yaml:"timeout"`
	QueueWarnThreshold int           `yaml:"queue_warn_threshold"`
}

type FeishuConfig struct {
	Enabled   bool   `yaml:"enabled"`
	AppID     string `yaml:"app_id"`
	AppSecret string `yaml:"app_secret"`
}

type DingTalkConfig struct {
	Enabled      bool   `yaml:"enabled"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

type MCPConfig struct {
	APIKey string `yaml:"api_key"`
}


type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type RuntimeConfig struct {
	ClaudeCode RuntimeEntry `yaml:"claude_code"`
	// Derived fields — not in YAML, computed by Load()
	MCPBaseURL string `yaml:"-"`
	MCPAPIKey  string `yaml:"-"`
}

type RuntimeEntry struct {
	Binary  string        `yaml:"binary"`
	Timeout time.Duration `yaml:"timeout"`
}

type MessageQueueConfig struct {
	DebounceWindow time.Duration `yaml:"debounce_window"`
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
	cfg.Bee.MCPBaseURL = fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	cfg.Bee.MCPAPIKey = cfg.MCP.APIKey
	cfg.Bee.Binary = cfg.Runtime.ClaudeCode.Binary
	cfg.Runtime.MCPBaseURL = cfg.Bee.MCPBaseURL
	cfg.Runtime.MCPAPIKey = cfg.MCP.APIKey
	return cfg, nil
}

func applyDefaults(cfg *Config) error {
	if cfg.MessageQueue.DebounceWindow == 0 {
		cfg.MessageQueue.DebounceWindow = 3 * time.Second
	}
	if cfg.Bee.Feeder.Interval == 0 {
		cfg.Bee.Feeder.Interval = 10 * time.Second
	}
	if cfg.Bee.Feeder.BatchSize == 0 {
		cfg.Bee.Feeder.BatchSize = 10
	}
	if cfg.Bee.Feeder.Timeout == 0 {
		cfg.Bee.Feeder.Timeout = 5 * time.Minute
	}
	if cfg.Bee.Feeder.QueueWarnThreshold == 0 {
		cfg.Bee.Feeder.QueueWarnThreshold = 100
	}
	if cfg.Runtime.ClaudeCode.Binary == "" {
		cfg.Runtime.ClaudeCode.Binary = "claude"
	}
	return nil
}
