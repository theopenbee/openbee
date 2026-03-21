package config

import (
	"os"
	"testing"
	"time"
)

func TestBeeConfig_DerivedFields(t *testing.T) {
	f, _ := os.CreateTemp("", "*.yaml")
	f.WriteString(`
server:
  host: "localhost"
  port: 8080
bee:
  mcp:
    api_key: "test-key"
  claude:
    path: "claude-custom"
`)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Bee.MCPBaseURL != "http://localhost:8080" {
		t.Errorf("MCPBaseURL: want http://localhost:8080 got %q", cfg.Bee.MCPBaseURL)
	}
	if cfg.Bee.MCP.APIKey != "test-key" {
		t.Errorf("MCP.APIKey: want test-key got %q", cfg.Bee.MCP.APIKey)
	}
	if cfg.Bee.Claude.Path != "claude-custom" {
		t.Errorf("Claude.Path: want claude-custom got %q", cfg.Bee.Claude.Path)
	}
}

func TestBeeConfig_BinaryDefault(t *testing.T) {
	f, _ := os.CreateTemp("", "*.yaml")
	f.WriteString(`
server:
  host: "localhost"
  port: 8080
`)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Bee.Claude.Path != "claude" {
		t.Errorf("Claude.Path default: want claude got %q", cfg.Bee.Claude.Path)
	}
}

func TestBeeConfig_Defaults(t *testing.T) {
	f, _ := os.CreateTemp("", "*.yaml")
	f.WriteString(`
server:
  port: 8080
bee:
  name: "bee"
`)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Bee.Feeder.Timeout != 5*time.Minute {
		t.Errorf("default timeout: want 5m got %v", cfg.Bee.Feeder.Timeout)
	}
}

func TestBeeConfig_TelegramDefaults(t *testing.T) {
	f, _ := os.CreateTemp("", "*.yaml")
	f.WriteString(`
server:
  port: 8080
`)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Bee.Platforms.Telegram.MaxMediaSize != 50*1024*1024 {
		t.Errorf("Telegram.MaxMediaSize default: want %d got %d",
			50*1024*1024, cfg.Bee.Platforms.Telegram.MaxMediaSize)
	}
}

func TestBeeConfig_TelegramLoad(t *testing.T) {
	f, _ := os.CreateTemp("", "*.yaml")
	f.WriteString(`
server:
  port: 8080
bee:
  platforms:
    telegram:
      enabled: true
      token: "test-token"
      max_media_size: 10485760
`)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	tc := cfg.Bee.Platforms.Telegram
	if !tc.Enabled {
		t.Error("Telegram.Enabled: want true")
	}
	if tc.Token != "test-token" {
		t.Errorf("Telegram.Token: want test-token got %q", tc.Token)
	}
	if tc.MaxMediaSize != 10485760 {
		t.Errorf("Telegram.MaxMediaSize: want 10485760 got %d", tc.MaxMediaSize)
	}
}
