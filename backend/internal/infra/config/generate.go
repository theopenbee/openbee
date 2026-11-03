package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

type templateValues struct {
	Language        string
	ServerPort      string
	ServerHost      string
	Debug           bool
	DBPath          string
	RPCTokenSecret  string
	ServerEnvSecret string

	EngineDefault       string
	EngineTimeoutBee    string
	EngineTimeoutWorker string

	FeederMaxConcurrentBee int
	MessageDebounce        string

	AuthJWTSecret  string
	AuthAccessTTL  string
	AuthRefreshTTL string
}

func DefaultConfigPath() string {
	h, err := OpenbeeHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(h, "config.yaml")
}

func Prepare(path string) (bool, error) {
	return prepare(path, DefaultConfigPath(), os.Getenv)
}

func prepare(path, defaultPath string, getenv func(string) string) (bool, error) {
	if defaultPath == "" || filepath.Clean(path) != filepath.Clean(defaultPath) {
		return false, nil
	}
	return EnsureFile(path, DetectLanguage(getenv))
}

func EnsureFile(path, lang string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("stat config: %w", err)
	}
	rendered, err := renderTemplate(defaultTemplateValues(filepath.Dir(path), lang))
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create config dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create config: %w", err)
	}
	if _, err := f.Write(rendered); err != nil {
		f.Close()
		os.Remove(path)
		return false, fmt.Errorf("write config: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return false, fmt.Errorf("write config: %w", err)
	}
	return true, nil
}

func DetectLanguage(getenv func(string) string) string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := getenv(key)
		if v == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(v), "zh") {
			return "zh"
		}
		return "en"
	}
	return "en"
}

func defaultTemplateValues(dir, lang string) templateValues {
	return templateValues{
		Language:               lang,
		ServerPort:             "8080",
		ServerHost:             "localhost",
		DBPath:                 filepath.Join(dir, "data", "openbee.db"),
		RPCTokenSecret:         GenerateRandomSecret(),
		ServerEnvSecret:        GenerateRandomSecret(),
		EngineTimeoutBee:       "5m",
		EngineTimeoutWorker:    "30m",
		FeederMaxConcurrentBee: 5,
		MessageDebounce:        "300ms",
		AuthJWTSecret:          GenerateRandomSecret(),
		AuthAccessTTL:          "2h",
		AuthRefreshTTL:         "168h",
	}
}

func renderTemplate(vals templateValues) ([]byte, error) {
	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse config template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vals); err != nil {
		return nil, fmt.Errorf("render config template: %w", err)
	}
	return buf.Bytes(), nil
}
