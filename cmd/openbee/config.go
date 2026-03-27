package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/claude"
	"github.com/theopenbee/openbee/internal/config"
)

var configTemplate = config.ConfigTemplate

type configValues struct {
	ServerPort string
	ServerHost string
	Debug      bool
	DBPath     string
	MCPAPIKey      string
	WorkerAPIKey   string

	FeishuEnabled   bool
	FeishuAppID     string
	FeishuAppSecret string

	DingtalkEnabled      bool
	DingtalkClientID     string
	DingtalkClientSecret string

	WecomEnabled bool
	WecomBotID   string
	WecomSecret  string

	TelegramEnabled  bool
	TelegramToken    string
	TelegramAuthCode string

	WeixinEnabled    bool
	WeixinToken      string
	WeixinBaseURL    string
	WeixinCDNBaseURL string
	WeixinUserID     string

	ClaudePath             string
	ClaudeTimeout          string
	FeederTimeout          string
	FeederMaxConcurrentBee int
	MessageDebounce        string
	FFprobePath     string
	FFmpegPath      string

	AuthUsername   string
	AuthPassword   string
	AuthJWTSecret  string
	AuthAccessTTL  string
	AuthRefreshTTL string
}

var configOutputPath string

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Interactively generate a config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runConfig(cmd, args); errors.Is(err, claude.ErrInterrupted) {
			return nil
		} else {
			return err
		}
	},
}

func init() {
	configCmd.Flags().StringVarP(&configOutputPath, "output", "o", "config.yaml", "output config file path")
	rootCmd.AddCommand(configCmd)
}

// loadExistingConfig tries to load an existing config file and convert it to configValues
// for use as defaults in the interactive prompts.
func loadExistingConfig(path string) *configValues {
	cfg, err := config.Load(path)
	if err != nil {
		return nil
	}

	return &configValues{
		ServerPort:           strconv.Itoa(cfg.Server.Port),
		ServerHost:           cfg.Server.Host,
		Debug:                cfg.Server.Debug,
		DBPath:               cfg.Database.Path,
		MCPAPIKey:            cfg.Bee.MCP.APIKey,
		WorkerAPIKey:         cfg.Bee.MCP.WorkerAPIKey,
		FeishuEnabled:        cfg.Bee.Platforms.Feishu.Enabled,
		FeishuAppID:          cfg.Bee.Platforms.Feishu.AppID,
		FeishuAppSecret:      cfg.Bee.Platforms.Feishu.AppSecret,
		DingtalkEnabled:      cfg.Bee.Platforms.DingTalk.Enabled,
		DingtalkClientID:     cfg.Bee.Platforms.DingTalk.ClientID,
		DingtalkClientSecret: cfg.Bee.Platforms.DingTalk.ClientSecret,
		WecomEnabled:         cfg.Bee.Platforms.WeCom.Enabled,
		WecomBotID:           cfg.Bee.Platforms.WeCom.BotID,
		WecomSecret:          cfg.Bee.Platforms.WeCom.Secret,
		TelegramEnabled:      cfg.Bee.Platforms.Telegram.Enabled,
		TelegramToken:        cfg.Bee.Platforms.Telegram.Token,
		TelegramAuthCode:     cfg.Bee.Platforms.Telegram.AuthCode,
		WeixinEnabled:        cfg.Bee.Platforms.Weixin.Enabled,
		WeixinToken:          cfg.Bee.Platforms.Weixin.Token,
		WeixinBaseURL:        cfg.Bee.Platforms.Weixin.BaseURL,
		WeixinCDNBaseURL:     cfg.Bee.Platforms.Weixin.CDNBaseURL,
		WeixinUserID:         cfg.Bee.Platforms.Weixin.UserID,
		ClaudePath:           cfg.Bee.Claude.Path,
		ClaudeTimeout:        cfg.Bee.Claude.Timeout.String(),
		FeederTimeout:          cfg.Bee.Feeder.Timeout.String(),
		FeederMaxConcurrentBee: cfg.Bee.Feeder.MaxConcurrentBee,
		MessageDebounce:      cfg.Bee.MessageDebounce.String(),
		FFprobePath:          cfg.Bee.Media.FFprobePath,
		FFmpegPath:           cfg.Bee.Media.FFmpegPath,
		AuthUsername:         cfg.Server.Auth.Username,
		AuthPassword:         cfg.Server.Auth.Password,
		AuthJWTSecret:        cfg.Server.Auth.JWTSecret,
		AuthAccessTTL:        cfg.Server.Auth.AccessTokenTTL.String(),
		AuthRefreshTTL:       cfg.Server.Auth.RefreshTokenTTL.String(),
	}
}

func runConfig(cmd *cobra.Command, args []string) error {
	vals := configValues{
		ServerPort:      "8080",
		ServerHost:      "localhost",
		DBPath:          "./data/openbee.db",
		ClaudePath:      "claude",
		ClaudeTimeout:   "30m",
		FeederTimeout:          "5m",
		FeederMaxConcurrentBee: 5,
		MessageDebounce: "3s",
		FFprobePath:     "ffprobe",
		FFmpegPath:      "ffmpeg",
		AuthUsername:    "admin",
		AuthAccessTTL:  "2h",
		AuthRefreshTTL: "168h",
	}

	// If an existing config file exists, use its values as defaults
	if existing := loadExistingConfig(configOutputPath); existing != nil {
		fmt.Printf("Found existing config at %s, using its values as defaults.\n", configOutputPath)
		vals = *existing
	}

	// Step 1 — Claude config
	fmt.Println("\n=== Claude Configuration ===")

	if err := configureClaudeExecutable(&vals); err != nil {
		return err
	}
	if err := configureClaudeProvider(&vals); err != nil {
		return err
	}

	// Step 2 — Platform config
	fmt.Println("\n=== Platform Configuration ===")

	// Build default selections from existing config
	var defaultPlatforms []string
	if vals.FeishuEnabled {
		defaultPlatforms = append(defaultPlatforms, "Feishu")
	}
	if vals.DingtalkEnabled {
		defaultPlatforms = append(defaultPlatforms, "DingTalk")
	}
	if vals.WecomEnabled {
		defaultPlatforms = append(defaultPlatforms, "WeCom")
	}
	if vals.TelegramEnabled {
		defaultPlatforms = append(defaultPlatforms, "Telegram")
	}
	if vals.WeixinEnabled {
		defaultPlatforms = append(defaultPlatforms, "Weixin")
	}

	var selectedPlatforms []string
	if err := survey.AskOne(&survey.MultiSelect{
		Message: "Which platforms to enable?",
		Options: []string{"Feishu", "DingTalk", "WeCom", "Telegram", "Weixin"},
		Default: defaultPlatforms,
	}, &selectedPlatforms); err != nil {
		return handleSurveyErr(err)
	}

	// Reset platform flags — they'll be re-enabled based on selection
	vals.FeishuEnabled = false
	vals.DingtalkEnabled = false
	vals.WecomEnabled = false
	vals.TelegramEnabled = false
	vals.WeixinEnabled = false

	for _, p := range selectedPlatforms {
		switch p {
		case "Feishu":
			vals.FeishuEnabled = true
			if err := survey.AskOne(&survey.Input{
				Message: "Feishu App ID:",
				Default: vals.FeishuAppID,
			}, &vals.FeishuAppID, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
			if err := survey.AskOne(&survey.Input{
				Message: "Feishu App Secret:",
				Default: vals.FeishuAppSecret,
			}, &vals.FeishuAppSecret, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		case "DingTalk":
			vals.DingtalkEnabled = true
			if err := survey.AskOne(&survey.Input{
				Message: "DingTalk Client ID:",
				Default: vals.DingtalkClientID,
			}, &vals.DingtalkClientID, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
			if err := survey.AskOne(&survey.Input{
				Message: "DingTalk Client Secret:",
				Default: vals.DingtalkClientSecret,
			}, &vals.DingtalkClientSecret, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		case "WeCom":
			vals.WecomEnabled = true
			if err := survey.AskOne(&survey.Input{
				Message: "WeCom Bot ID:",
				Default: vals.WecomBotID,
			}, &vals.WecomBotID, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
			if err := survey.AskOne(&survey.Input{
				Message: "WeCom Secret:",
				Default: vals.WecomSecret,
			}, &vals.WecomSecret, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		case "Telegram":
			vals.TelegramEnabled = true
			if err := survey.AskOne(&survey.Password{
				Message: "Telegram Bot Token:",
				Help:    "Get a token from @BotFather on Telegram",
			}, &vals.TelegramToken, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
			authCodeDefault := vals.TelegramAuthCode
			if authCodeDefault == "" {
				b := make([]byte, 8)
				rand.Read(b)
				authCodeDefault = hex.EncodeToString(b)
			}
			if err := survey.AskOne(&survey.Input{
				Message: "Telegram Auth Code (empty to disable auth):",
				Default: authCodeDefault,
				Help:    "Users must send /auth <code> to use the bot; leave empty to allow all",
			}, &vals.TelegramAuthCode); err != nil {
				return handleSurveyErr(err)
			}
		case "Weixin":
			vals.WeixinEnabled = true

			needQRLogin := true
			if vals.WeixinToken != "" {
				masked := vals.WeixinToken
				if len(masked) > 6 {
					masked = masked[:6] + "***"
				}
				var reacquire bool
				if err := survey.AskOne(&survey.Confirm{
					Message: fmt.Sprintf("Existing Weixin token found (%s). Re-acquire via QR code?", masked),
					Default: false,
				}, &reacquire); err != nil {
					return handleSurveyErr(err)
				}
				needQRLogin = reacquire
			}

			if needQRLogin {
				fmt.Println("\n--- Weixin QR Code Login ---")
				fmt.Println("Fetching QR code...")

				token, userID, baseURL, err := runWeixinQRLogin()
				if err != nil {
					fmt.Printf("QR login failed: %v\n", err)
					fmt.Println("Falling back to manual token entry.")
					if err := survey.AskOne(&survey.Password{
						Message: "Weixin Bot Token:",
					}, &vals.WeixinToken, survey.WithValidator(survey.Required)); err != nil {
						return handleSurveyErr(err)
					}
					if err := survey.AskOne(&survey.Input{
						Message: "Weixin User ID:",
						Default: vals.WeixinUserID,
					}, &vals.WeixinUserID, survey.WithValidator(survey.Required)); err != nil {
						return handleSurveyErr(err)
					}
				} else {
					vals.WeixinToken = token
					vals.WeixinUserID = userID
					if baseURL != "" {
						vals.WeixinBaseURL = baseURL
					}
					fmt.Println("Weixin login successful!")
				}
			}
			if vals.WeixinBaseURL == "" {
				vals.WeixinBaseURL = "https://ilinkai.weixin.qq.com"
			}
			vals.WeixinCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
		}
	}

	// Step 3 — Web Authentication
	fmt.Println("\n=== Web Authentication ===")

	if err := survey.AskOne(&survey.Input{
		Message: "Username:",
		Default: vals.AuthUsername,
	}, &vals.AuthUsername, survey.WithValidator(survey.Required)); err != nil {
		return handleSurveyErr(err)
	}

	if vals.AuthPassword != "" {
		var changePassword bool
		if err := survey.AskOne(&survey.Confirm{
			Message: "Password already configured. Change it?",
			Default: false,
		}, &changePassword); err != nil {
			return handleSurveyErr(err)
		}
		if changePassword {
			if err := promptPassword(&vals); err != nil {
				return err
			}
		}
	} else {
		if err := promptPassword(&vals); err != nil {
			return err
		}
	}

	if vals.AuthJWTSecret != "" {
		var regenerate bool
		if err := survey.AskOne(&survey.Confirm{
			Message: "JWT secret already exists. Regenerate?",
			Default: false,
		}, &regenerate); err != nil {
			return handleSurveyErr(err)
		}
		if regenerate {
			b := make([]byte, 32)
			rand.Read(b)
			vals.AuthJWTSecret = hex.EncodeToString(b)
			fmt.Println("JWT secret regenerated.")
		}
	} else {
		b := make([]byte, 32)
		rand.Read(b)
		vals.AuthJWTSecret = hex.EncodeToString(b)
		fmt.Println("JWT secret generated.")
	}

	// Step 4 — Advanced config
	fmt.Println("\n=== Advanced Configuration ===")

	var customAdvanced bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "Customize advanced settings?",
		Default: false,
	}, &customAdvanced); err != nil {
		return handleSurveyErr(err)
	}

	if customAdvanced {
		if err := survey.AskOne(&survey.Input{
			Message: "Server port:",
			Default: vals.ServerPort,
		}, &vals.ServerPort, survey.WithValidator(func(val interface{}) error {
			s, _ := val.(string)
			if _, err := strconv.Atoi(s); err != nil {
				return fmt.Errorf("port must be an integer")
			}
			return nil
		})); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: "Server Host:",
			Default: vals.ServerHost,
		}, &vals.ServerHost); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Confirm{
			Message: "Debug mode?",
			Default: vals.Debug,
		}, &vals.Debug); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: "Database path:",
			Default: vals.DBPath,
		}, &vals.DBPath); err != nil {
			return handleSurveyErr(err)
		}

		mcpKeyChoice := "Generate randomly"
		if vals.MCPAPIKey != "" {
			mcpKeyChoice = "Enter manually"
		}
		var mcpMethod string
		if err := survey.AskOne(&survey.Select{
			Message: "MCP API Key setup:",
			Options: []string{"Generate randomly", "Enter manually"},
			Default: mcpKeyChoice,
		}, &mcpMethod); err != nil {
			return handleSurveyErr(err)
		}

		switch mcpMethod {
		case "Generate randomly":
			b := make([]byte, 12)
			if _, err := rand.Read(b); err != nil {
				return fmt.Errorf("generate random key: %w", err)
			}
			vals.MCPAPIKey = hex.EncodeToString(b)
			fmt.Printf("Generated MCP API Key: %s\n", vals.MCPAPIKey)
		case "Enter manually":
			if err := survey.AskOne(&survey.Input{
				Message: "MCP API Key:",
				Default: vals.MCPAPIKey,
			}, &vals.MCPAPIKey, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		}

		workerKeyChoice := "Generate randomly"
		if vals.WorkerAPIKey != "" {
			workerKeyChoice = "Enter manually"
		}
		var workerKeyMethod string
		if err := survey.AskOne(&survey.Select{
			Message: "MCP Worker API Key setup:",
			Options: []string{"Generate randomly", "Enter manually"},
			Default: workerKeyChoice,
		}, &workerKeyMethod); err != nil {
			return handleSurveyErr(err)
		}

		switch workerKeyMethod {
		case "Generate randomly":
			b := make([]byte, 12)
			if _, err := rand.Read(b); err != nil {
				return fmt.Errorf("generate random worker key: %w", err)
			}
			vals.WorkerAPIKey = hex.EncodeToString(b)
			fmt.Printf("Generated MCP Worker API Key: %s\n", vals.WorkerAPIKey)
		case "Enter manually":
			if err := survey.AskOne(&survey.Input{
				Message: "MCP Worker API Key:",
				Default: vals.WorkerAPIKey,
			}, &vals.WorkerAPIKey, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		}

		if err := survey.AskOne(&survey.Input{
			Message: "Feeder timeout:",
			Default: vals.FeederTimeout,
		}, &vals.FeederTimeout); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: "Message debounce:",
			Default: vals.MessageDebounce,
		}, &vals.MessageDebounce); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: "FFprobe path:",
			Default: vals.FFprobePath,
		}, &vals.FFprobePath); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: "FFmpeg path:",
			Default: vals.FFmpegPath,
		}, &vals.FFmpegPath); err != nil {
			return handleSurveyErr(err)
		}
	}

	// Auto-generate MCP API Key if not set
	if vals.MCPAPIKey == "" {
		b := make([]byte, 12)
		if _, err := rand.Read(b); err != nil {
			return fmt.Errorf("generate random key: %w", err)
		}
		vals.MCPAPIKey = hex.EncodeToString(b)
		fmt.Printf("Generated MCP API Key: %s\n", vals.MCPAPIKey)
	}

	// Auto-generate Worker API Key if not set
	if vals.WorkerAPIKey == "" {
		b := make([]byte, 12)
		if _, err := rand.Read(b); err != nil {
			return fmt.Errorf("generate random worker key: %w", err)
		}
		vals.WorkerAPIKey = hex.EncodeToString(b)
		fmt.Printf("Generated Worker API Key: %s\n", vals.WorkerAPIKey)
	}

	// Step 4 — Confirm write
	fmt.Printf("\n=== Write Configuration ===\n")
	fmt.Printf("Output file: %s\n", configOutputPath)

	var confirmWrite bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "Confirm write config file?",
		Default: true,
	}, &confirmWrite); err != nil {
		return handleSurveyErr(err)
	}
	if !confirmWrite {
		fmt.Println("Write cancelled.")
		return nil
	}

	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vals); err != nil {
		return fmt.Errorf("render template: %w", err)
	}

	if err := os.WriteFile(configOutputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	fmt.Printf("Config file written to: %s\n", configOutputPath)
	return nil
}

func promptPassword(vals *configValues) error {
	var method string
	if err := survey.AskOne(&survey.Select{
		Message: "Password setup:",
		Options: []string{"Enter manually", "Generate randomly"},
	}, &method); err != nil {
		return handleSurveyErr(err)
	}
	switch method {
	case "Enter manually":
		if err := survey.AskOne(&survey.Password{
			Message: "Password:",
		}, &vals.AuthPassword, survey.WithValidator(survey.Required)); err != nil {
			return handleSurveyErr(err)
		}
	case "Generate randomly":
		b := make([]byte, 16)
		rand.Read(b)
		vals.AuthPassword = hex.EncodeToString(b)
		fmt.Printf("Generated password: %s\n", vals.AuthPassword)
	}
	return nil
}

func handleSurveyErr(err error) error {
	return claude.HandleSurveyErr(err)
}
