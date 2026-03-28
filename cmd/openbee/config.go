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
	"github.com/theopenbee/openbee/internal/i18n"
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
		MessageDebounce: "300ms",
		FFprobePath:     "ffprobe",
		FFmpegPath:      "ffmpeg",
		AuthUsername:    "admin",
		AuthAccessTTL:  "2h",
		AuthRefreshTTL: "168h",
	}

	// If an existing config file exists, use its values as defaults
	if existing := loadExistingConfig(configOutputPath); existing != nil {
		fmt.Printf(i18n.M.Output.Config.FoundExisting+"\n", configOutputPath)
		vals = *existing
	}

	// Step 1 — Claude config
	fmt.Println(i18n.M.Output.Config.SectionClaude)

	if err := configureClaudeExecutable(&vals); err != nil {
		return err
	}
	if err := configureClaudeProvider(&vals); err != nil {
		return err
	}

	// Step 2 — Platform config
	fmt.Println(i18n.M.Output.Config.SectionPlatform)

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
		Message: i18n.M.Prompt.PlatformSelect,
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
				Message: i18n.M.Prompt.FeishuAppID,
				Default: vals.FeishuAppID,
			}, &vals.FeishuAppID, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
			if err := survey.AskOne(&survey.Input{
				Message: i18n.M.Prompt.FeishuAppSecret,
				Default: vals.FeishuAppSecret,
			}, &vals.FeishuAppSecret, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		case "DingTalk":
			vals.DingtalkEnabled = true
			if err := survey.AskOne(&survey.Input{
				Message: i18n.M.Prompt.DingtalkClientID,
				Default: vals.DingtalkClientID,
			}, &vals.DingtalkClientID, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
			if err := survey.AskOne(&survey.Input{
				Message: i18n.M.Prompt.DingtalkClientSecret,
				Default: vals.DingtalkClientSecret,
			}, &vals.DingtalkClientSecret, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		case "WeCom":
			vals.WecomEnabled = true
			if err := survey.AskOne(&survey.Input{
				Message: i18n.M.Prompt.WecomBotID,
				Default: vals.WecomBotID,
			}, &vals.WecomBotID, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
			if err := survey.AskOne(&survey.Input{
				Message: i18n.M.Prompt.WecomSecret,
				Default: vals.WecomSecret,
			}, &vals.WecomSecret, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		case "Telegram":
			vals.TelegramEnabled = true
			if err := survey.AskOne(&survey.Password{
				Message: i18n.M.Prompt.TelegramToken,
				Help:    i18n.M.Prompt.TelegramTokenHelp,
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
				Message: i18n.M.Prompt.TelegramAuthCode,
				Default: authCodeDefault,
				Help:    i18n.M.Prompt.TelegramAuthCodeHelp,
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
					Message: fmt.Sprintf(i18n.M.Prompt.WeixinReacquire, masked),
					Default: false,
				}, &reacquire); err != nil {
					return handleSurveyErr(err)
				}
				needQRLogin = reacquire
			}

			if needQRLogin {
				fmt.Println(i18n.M.Output.Config.WeixinQRLogin)
				fmt.Println(i18n.M.Output.Config.FetchingQR)

				token, userID, baseURL, err := runWeixinQRLogin()
				if err != nil {
					fmt.Printf(i18n.M.Output.Config.QRFailed+"\n", err)
					fmt.Println(i18n.M.Output.Config.QRFallback)
					if err := survey.AskOne(&survey.Password{
						Message: i18n.M.Prompt.WeixinBotToken,
					}, &vals.WeixinToken, survey.WithValidator(survey.Required)); err != nil {
						return handleSurveyErr(err)
					}
					if err := survey.AskOne(&survey.Input{
						Message: i18n.M.Prompt.WeixinUserID,
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
					fmt.Println(i18n.M.Output.Config.WeixinSuccess)
				}
			}
			if vals.WeixinBaseURL == "" {
				vals.WeixinBaseURL = "https://ilinkai.weixin.qq.com"
			}
			vals.WeixinCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
		}
	}

	// Step 3 — Web Authentication
	fmt.Println(i18n.M.Output.Config.SectionAuth)

	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.Username,
		Default: vals.AuthUsername,
	}, &vals.AuthUsername, survey.WithValidator(survey.Required)); err != nil {
		return handleSurveyErr(err)
	}

	if vals.AuthPassword != "" {
		var changePassword bool
		if err := survey.AskOne(&survey.Confirm{
			Message: i18n.M.Prompt.PasswordChangeConfirm,
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
			Message: i18n.M.Prompt.JWTRegenConfirm,
			Default: false,
		}, &regenerate); err != nil {
			return handleSurveyErr(err)
		}
		if regenerate {
			b := make([]byte, 32)
			rand.Read(b)
			vals.AuthJWTSecret = hex.EncodeToString(b)
			fmt.Println(i18n.M.Output.Config.JWTRegenerated)
		}
	} else {
		b := make([]byte, 32)
		rand.Read(b)
		vals.AuthJWTSecret = hex.EncodeToString(b)
		fmt.Println(i18n.M.Output.Config.JWTGenerated)
	}

	// Step 4 — Advanced config
	fmt.Println(i18n.M.Output.Config.SectionAdvanced)

	var customAdvanced bool
	if err := survey.AskOne(&survey.Confirm{
		Message: i18n.M.Prompt.AdvancedConfirm,
		Default: false,
	}, &customAdvanced); err != nil {
		return handleSurveyErr(err)
	}

	if customAdvanced {
		if err := survey.AskOne(&survey.Input{
			Message: i18n.M.Prompt.ServerPort,
			Default: vals.ServerPort,
		}, &vals.ServerPort, survey.WithValidator(func(val interface{}) error {
			s, _ := val.(string)
			if _, err := strconv.Atoi(s); err != nil {
				return errors.New(i18n.M.Validate.PortInteger)
			}
			return nil
		})); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: i18n.M.Prompt.ServerHost,
			Default: vals.ServerHost,
		}, &vals.ServerHost); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Confirm{
			Message: i18n.M.Prompt.DebugMode,
			Default: vals.Debug,
		}, &vals.Debug); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: i18n.M.Prompt.DBPath,
			Default: vals.DBPath,
		}, &vals.DBPath); err != nil {
			return handleSurveyErr(err)
		}

		mcpKeyChoice := i18n.M.Prompt.OptionGenerateRandom
		if vals.MCPAPIKey != "" {
			mcpKeyChoice = i18n.M.Prompt.OptionEnterManually
		}
		var mcpMethod string
		if err := survey.AskOne(&survey.Select{
			Message: i18n.M.Prompt.MCPAPIKeySetup,
			Options: []string{i18n.M.Prompt.OptionGenerateRandom, i18n.M.Prompt.OptionEnterManually},
			Default: mcpKeyChoice,
		}, &mcpMethod); err != nil {
			return handleSurveyErr(err)
		}

		switch mcpMethod {
		case i18n.M.Prompt.OptionGenerateRandom:
			b := make([]byte, 12)
			if _, err := rand.Read(b); err != nil {
				return fmt.Errorf("generate random key: %w", err)
			}
			vals.MCPAPIKey = hex.EncodeToString(b)
			fmt.Printf(i18n.M.Output.Config.MCPKeyGenerated+"\n", vals.MCPAPIKey)
		case i18n.M.Prompt.OptionEnterManually:
			if err := survey.AskOne(&survey.Input{
				Message: i18n.M.Prompt.MCPAPIKey,
				Default: vals.MCPAPIKey,
			}, &vals.MCPAPIKey, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		}

		workerKeyChoice := i18n.M.Prompt.OptionGenerateRandom
		if vals.WorkerAPIKey != "" {
			workerKeyChoice = i18n.M.Prompt.OptionEnterManually
		}
		var workerKeyMethod string
		if err := survey.AskOne(&survey.Select{
			Message: i18n.M.Prompt.MCPWorkerAPIKeySetup,
			Options: []string{i18n.M.Prompt.OptionGenerateRandom, i18n.M.Prompt.OptionEnterManually},
			Default: workerKeyChoice,
		}, &workerKeyMethod); err != nil {
			return handleSurveyErr(err)
		}

		switch workerKeyMethod {
		case i18n.M.Prompt.OptionGenerateRandom:
			b := make([]byte, 12)
			if _, err := rand.Read(b); err != nil {
				return fmt.Errorf("generate random worker key: %w", err)
			}
			vals.WorkerAPIKey = hex.EncodeToString(b)
			fmt.Printf(i18n.M.Output.Config.WorkerKeyGenerated+"\n", vals.WorkerAPIKey)
		case i18n.M.Prompt.OptionEnterManually:
			if err := survey.AskOne(&survey.Input{
				Message: i18n.M.Prompt.MCPWorkerAPIKey,
				Default: vals.WorkerAPIKey,
			}, &vals.WorkerAPIKey, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		}

		if err := survey.AskOne(&survey.Input{
			Message: i18n.M.Prompt.FeederTimeout,
			Default: vals.FeederTimeout,
		}, &vals.FeederTimeout); err != nil {
			return handleSurveyErr(err)
		}

		var concurrentBeeStr string
		if err := survey.AskOne(&survey.Input{
			Message: i18n.M.Prompt.MaxConcurrentBee,
			Default: strconv.Itoa(vals.FeederMaxConcurrentBee),
		}, &concurrentBeeStr, survey.WithValidator(func(ans interface{}) error {
			s, _ := ans.(string)
			n, err := strconv.Atoi(s)
			if err != nil || n <= 0 {
				return errors.New(i18n.M.Validate.PositiveInteger)
			}
			return nil
		})); err != nil {
			return handleSurveyErr(err)
		}
		vals.FeederMaxConcurrentBee, _ = strconv.Atoi(concurrentBeeStr)

		if err := survey.AskOne(&survey.Input{
			Message: i18n.M.Prompt.MessageDebounce,
			Default: vals.MessageDebounce,
		}, &vals.MessageDebounce); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: i18n.M.Prompt.FFprobePath,
			Default: vals.FFprobePath,
		}, &vals.FFprobePath); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: i18n.M.Prompt.FFmpegPath,
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
		fmt.Printf(i18n.M.Output.Config.MCPKeyGenerated+"\n", vals.MCPAPIKey)
	}

	// Auto-generate Worker API Key if not set
	if vals.WorkerAPIKey == "" {
		b := make([]byte, 12)
		if _, err := rand.Read(b); err != nil {
			return fmt.Errorf("generate random worker key: %w", err)
		}
		vals.WorkerAPIKey = hex.EncodeToString(b)
		fmt.Printf(i18n.M.Output.Config.WorkerKeyGenerated+"\n", vals.WorkerAPIKey)
	}

	// Step 4 — Confirm write
	fmt.Println(i18n.M.Output.Config.SectionWrite)
	fmt.Printf(i18n.M.Output.Config.OutputFile+"\n", configOutputPath)

	var confirmWrite bool
	if err := survey.AskOne(&survey.Confirm{
		Message: i18n.M.Prompt.ConfirmWrite,
		Default: true,
	}, &confirmWrite); err != nil {
		return handleSurveyErr(err)
	}
	if !confirmWrite {
		fmt.Println(i18n.M.Output.Config.WriteCancelled)
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

	fmt.Printf(i18n.M.Output.Config.Written+"\n", configOutputPath)
	return nil
}

func promptPassword(vals *configValues) error {
	var method string
	if err := survey.AskOne(&survey.Select{
		Message: i18n.M.Prompt.PasswordSetup,
		Options: []string{i18n.M.Prompt.OptionEnterManually, i18n.M.Prompt.OptionGenerateRandom},
	}, &method); err != nil {
		return handleSurveyErr(err)
	}
	switch method {
	case i18n.M.Prompt.OptionEnterManually:
		if err := survey.AskOne(&survey.Password{
			Message: i18n.M.Prompt.Password,
		}, &vals.AuthPassword, survey.WithValidator(survey.Required)); err != nil {
			return handleSurveyErr(err)
		}
	case i18n.M.Prompt.OptionGenerateRandom:
		b := make([]byte, 16)
		rand.Read(b)
		vals.AuthPassword = hex.EncodeToString(b)
		fmt.Printf(i18n.M.Output.Config.PasswordGenerated+"\n", vals.AuthPassword)
	}
	return nil
}

func handleSurveyErr(err error) error {
	return claude.HandleSurveyErr(err)
}
