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
mcp:
  api_key: "test-key"
bee:
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
	if cfg.Bee.MCPAPIKey != "test-key" {
		t.Errorf("MCPAPIKey: want test-key got %q", cfg.Bee.MCPAPIKey)
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

	if cfg.Bee.Feeder.Interval != 10*time.Second {
		t.Errorf("default interval: want 10s got %v", cfg.Bee.Feeder.Interval)
	}
	if cfg.Bee.Feeder.BatchSize != 10 {
		t.Errorf("default batch_size: want 10 got %d", cfg.Bee.Feeder.BatchSize)
	}
	if cfg.Bee.Feeder.Timeout != 5*time.Minute {
		t.Errorf("default timeout: want 5m got %v", cfg.Bee.Feeder.Timeout)
	}
	if cfg.Bee.Feeder.QueueWarnThreshold != 100 {
		t.Errorf("default queue_warn_threshold: want 100 got %d", cfg.Bee.Feeder.QueueWarnThreshold)
	}
}
