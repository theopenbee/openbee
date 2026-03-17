package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed config.yaml.tmpl
var ConfigTemplate string

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
	Claude          ClaudeConfig    `yaml:"claude"`
	Feeder          FeederConfig    `yaml:"feeder"`
	Platforms       PlatformsConfig `yaml:"platforms"`
	MCP             MCPConfig       `yaml:"mcp"`
	Media           MediaConfig     `yaml:"media"`

	// Derived fields — not in YAML, computed by Load()
	MCPBaseURL string `yaml:"-"` // http://host:port (no path suffix)
}

type PlatformsConfig struct {
	Feishu   FeishuConfig   `yaml:"feishu"`
	DingTalk DingTalkConfig `yaml:"dingtalk"`
	WeCom    WeComConfig    `yaml:"wecom"`
}

type FeederConfig struct {
	Timeout time.Duration `yaml:"timeout"`
}

type FeishuConfig struct {
	Enabled      bool   `yaml:"enabled"`
	AppID        string `yaml:"app_id"`
	AppSecret    string `yaml:"app_secret"`
	MaxMediaSize int    `yaml:"max_media_size"` // 最大媒体下载大小（字节），默认 100MB
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

type MCPConfig struct {
	APIKey string `yaml:"api_key"`
}


type ServerConfig struct {
	Port  int    `yaml:"port"`
	Host  string `yaml:"host"`
	Debug bool   `yaml:"debug"`
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
	cfg.Bee.MCPBaseURL = fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.Port)
	return cfg, nil
}

func applyDefaults(cfg *Config) error {
	if cfg.Bee.MessageDebounce == 0 {
		cfg.Bee.MessageDebounce = 3 * time.Second
	}
	if cfg.Bee.Feeder.Timeout == 0 {
		cfg.Bee.Feeder.Timeout = 5 * time.Minute
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
	return nil
}
