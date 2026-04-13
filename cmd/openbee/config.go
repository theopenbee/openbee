package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/ai/claude"
	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/skillinstall"
)

var configTemplate = config.ConfigTemplate

// originalMultiSelectTemplate holds the unmodified survey template so we can
// replace the hint text cleanly after each language selection.
var originalMultiSelectTemplate = survey.MultiSelectQuestionTemplate

// multiSelectHintOld is the exact hint fragment to replace inside the template.
const multiSelectHintOld = `[Use arrows to move, space to select,{{- if not .Config.RemoveSelectAll }} <right> to all,{{end}}{{- if not .Config.RemoveSelectNone }} <left> to none,{{end}} type to filter{{- if and .Help (not .ShowHelp)}}, {{ .Config.HelpInput }} for more help{{end}}]`

// applySurveyTemplates patches the survey MultiSelect hint text to use the
// currently loaded i18n locale. Must be called after i18n.Load().
func applySurveyTemplates() {
	survey.MultiSelectQuestionTemplate = strings.Replace(
		originalMultiSelectTemplate,
		multiSelectHintOld,
		i18n.M.Prompt.MultiSelectHint,
		1,
	)
}

type configValues struct {
	Language   string
	ServerPort string
	ServerHost string
	Debug      bool
	DBPath     string
	MCPTokenSecret   string
	MCPTokenTTL      string
	AdvancedEnvSecret string

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

	Engine        string
	ClaudePath    string
	ClaudeTimeout string
	CodexPath     string
	CodexTimeout  string
	PiPath        string
	PiTimeout     string
	PiEnv         map[string]string

	FeederTimeout          string
	FeederMaxConcurrentBee int
	MessageDebounce        string
	FFprobePath            string
	FFmpegPath             string

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
		Language:             cfg.Language,
		ServerPort:           strconv.Itoa(cfg.Server.Port),
		ServerHost:           cfg.Server.Host,
		Debug:                cfg.Server.Debug,
		DBPath:               cfg.Database.Path,
		MCPTokenSecret:        cfg.Bee.MCP.TokenSecret,
		MCPTokenTTL:           cfg.Bee.MCP.TokenTTL.String(),
		AdvancedEnvSecret:     cfg.Advanced.EnvSecret,
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
		Engine:               cfg.Bee.Engine,
		ClaudePath:           cfg.Bee.Claude.Path,
		ClaudeTimeout:        cfg.Bee.Claude.Timeout.String(),
		CodexPath:            cfg.Bee.Codex.Path,
		CodexTimeout:         cfg.Bee.Codex.Timeout.String(),
		PiPath:               cfg.Bee.Pi.Path,
		PiTimeout:            cfg.Bee.Pi.Timeout.String(),
		PiEnv:                cfg.Bee.Pi.Env,
		FeederTimeout:        cfg.Bee.Feeder.Timeout.String(),
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
		ServerPort:             "8080",
		ServerHost:             "localhost",
		DBPath:                 "./data/openbee.db",
		Engine:                 "claude",
		ClaudePath:             "claude",
		ClaudeTimeout:          "30m",
		CodexPath:              "codex",
		CodexTimeout:           "30m",
		PiPath:                 "pi",
		PiTimeout:              "30m",
		MCPTokenTTL:            "2h",
		FeederTimeout:          "5m",
		FeederMaxConcurrentBee: 5,
		MessageDebounce:        "300ms",
		FFprobePath:            "ffprobe",
		FFmpegPath:             "ffmpeg",
		AuthUsername:           "admin",
		AuthAccessTTL:          "2h",
		AuthRefreshTTL:         "168h",
	}

	// If an existing config file exists, load its values as defaults silently
	// (do NOT print anything yet — language hasn't been selected).
	existingFound := false
	if existing := loadExistingConfig(configOutputPath); existing != nil {
		existingFound = true
		vals = *existing
	}

	// Language selection — always shown first, before all other prompts
	lang, err := runLanguageStep(vals.Language)
	if err != nil {
		return err
	}
	vals.Language = lang

	// Now print the "found existing" message using the selected locale
	if existingFound {
		fmt.Printf(i18n.M.Output.Config.FoundExisting+"\n", configOutputPath)
	}

	// Step 1 — Engine config
	fmt.Println(i18n.M.Output.Config.SectionEngine)

	defaultEngineOpt := i18n.M.Prompt.OptionEngineClaude
	switch vals.Engine {
	case "codex":
		defaultEngineOpt = i18n.M.Prompt.OptionEngineCodex
	case "pi":
		defaultEngineOpt = i18n.M.Prompt.OptionEnginePi
	}
	var selectedEngine string
	if err := survey.AskOne(&survey.Select{
		Message: i18n.M.Prompt.EngineSelect,
		Options: []string{
			i18n.M.Prompt.OptionEngineClaude,
			i18n.M.Prompt.OptionEngineCodex,
			i18n.M.Prompt.OptionEnginePi,
		},
		Default: defaultEngineOpt,
	}, &selectedEngine); err != nil {
		return handleSurveyErr(err)
	}

	switch selectedEngine {
	case i18n.M.Prompt.OptionEngineClaude:
		vals.Engine = "claude"
		if err := configureClaudeExecutable(&vals); err != nil {
			return err
		}
		if err := configureClaudeProvider(&vals); err != nil {
			return err
		}
	case i18n.M.Prompt.OptionEngineCodex:
		vals.Engine = "codex"
		if err := configureCodexExecutable(&vals); err != nil {
			return err
		}
	case i18n.M.Prompt.OptionEnginePi:
		vals.Engine = "pi"
		if err := configurePiExecutable(&vals); err != nil {
			return err
		}
	}

	// Step 2 — Platform config
	fmt.Println(i18n.M.Output.Config.SectionPlatform)

	// Build default selections from existing config
	var defaultPlatforms []string
	if vals.FeishuEnabled {
		defaultPlatforms = append(defaultPlatforms, i18n.M.Prompt.PlatformFeishu)
	}
	if vals.DingtalkEnabled {
		defaultPlatforms = append(defaultPlatforms, i18n.M.Prompt.PlatformDingTalk)
	}
	if vals.WecomEnabled {
		defaultPlatforms = append(defaultPlatforms, i18n.M.Prompt.PlatformWeCom)
	}
	if vals.TelegramEnabled {
		defaultPlatforms = append(defaultPlatforms, i18n.M.Prompt.PlatformTelegram)
	}
	if vals.WeixinEnabled {
		defaultPlatforms = append(defaultPlatforms, i18n.M.Prompt.PlatformWeixin)
	}

	var selectedPlatforms []string
	if err := survey.AskOne(&survey.MultiSelect{
		Message: i18n.M.Prompt.PlatformSelect,
		Options: []string{
			i18n.M.Prompt.PlatformFeishu,
			i18n.M.Prompt.PlatformDingTalk,
			i18n.M.Prompt.PlatformWeCom,
			i18n.M.Prompt.PlatformTelegram,
			i18n.M.Prompt.PlatformWeixin,
		},
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
		case i18n.M.Prompt.PlatformFeishu:
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
		case i18n.M.Prompt.PlatformDingTalk:
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
		case i18n.M.Prompt.PlatformWeCom:
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
		case i18n.M.Prompt.PlatformTelegram:
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
		case i18n.M.Prompt.PlatformWeixin:
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

		if vals.AuthJWTSecret != "" {
			var regenerate bool
			if err := survey.AskOne(&survey.Confirm{
				Message: i18n.M.Prompt.JWTRegenConfirm,
				Default: false,
			}, &regenerate); err != nil {
				return handleSurveyErr(err)
			}
			if regenerate {
				vals.AuthJWTSecret = config.GenerateRandomSecret()
				fmt.Println(i18n.M.Output.Config.JWTRegenerated)
			}
		} else {
			vals.AuthJWTSecret = config.GenerateRandomSecret()
			fmt.Println(i18n.M.Output.Config.JWTGenerated)
		}

		if vals.MCPTokenSecret != "" {
			var regenerate bool
			if err := survey.AskOne(&survey.Confirm{
				Message: i18n.M.Prompt.MCPTokenRegenConfirm,
				Default: false,
			}, &regenerate); err != nil {
				return handleSurveyErr(err)
			}
			if regenerate {
				vals.MCPTokenSecret = config.GenerateRandomSecret()
				fmt.Println(i18n.M.Output.Config.MCPTokenSecretRegenerated)
			}
		} else {
			vals.MCPTokenSecret = config.GenerateRandomSecret()
			fmt.Printf(i18n.M.Output.Config.MCPTokenSecretGenerated+"\n", vals.MCPTokenSecret)
		}
	}

	if !customAdvanced {
		if vals.AuthJWTSecret == "" {
			vals.AuthJWTSecret = config.GenerateRandomSecret()
		}
		if vals.MCPTokenSecret == "" {
			vals.MCPTokenSecret = config.GenerateRandomSecret()
		}
		if vals.AdvancedEnvSecret == "" {
			vals.AdvancedEnvSecret = config.GenerateRandomSecret()
		}
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
	installBuiltinSkills()
	return nil
}

// runLanguageStep shows a bilingual language-selection prompt and reloads i18n
// with the chosen language. existingLang should be "" or a previously saved
// language code ("en" or "zh"); it determines the default selection.
func runLanguageStep(existingLang string) (string, error) {
	defaultOpt := "English"
	if existingLang == "zh" {
		defaultOpt = "Chinese"
	}

	var selected string
	if err := survey.AskOne(&survey.Select{
		Message: "Select language",
		Options: []string{"English", "Chinese"},
		Default: defaultOpt,
	}, &selected); err != nil {
		return "", handleSurveyErr(err)
	}

	lang := "en"
	if selected == "Chinese" {
		lang = "zh"
	}

	if err := i18n.Load(lang); err != nil {
		return "", fmt.Errorf("load i18n: %w", err)
	}
	applySurveyTemplates()
	return lang, nil
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

func configureEngineExecutable(binaryName, foundMsg, manualMsg, pathMsg, timeoutMsg string, pathDst, timeoutDst *string) error {
	if found, err := exec.LookPath(binaryName); err == nil {
		fmt.Printf(foundMsg+"\n", found)
		*pathDst = found
	} else {
		fmt.Println(manualMsg)
		if err := survey.AskOne(&survey.Input{
			Message: pathMsg,
			Default: *pathDst,
		}, pathDst, survey.WithValidator(executablePathValidator)); err != nil {
			return handleSurveyErr(err)
		}
	}
	if err := survey.AskOne(&survey.Input{
		Message: timeoutMsg,
		Default: *timeoutDst,
	}, timeoutDst); err != nil {
		return handleSurveyErr(err)
	}
	return nil
}

func executablePathValidator(val any) error {
	path, _ := val.(string)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf(i18n.M.Validate.FileNotFound, path)
	}
	if info.IsDir() {
		return fmt.Errorf(i18n.M.Validate.PathIsDir, path)
	}
	if info.Mode()&0111 == 0 {
		return fmt.Errorf(i18n.M.Validate.FileNotExec, path)
	}
	return nil
}

func installBuiltinSkills() {
	results, err := skillinstall.InstallSkillsToDefaults()
	if err != nil {
		fmt.Printf(i18n.M.Output.Config.SkillsInstallWarning+"\n", err)
		return
	}
	for _, r := range results {
		switch r.Action {
		case skillinstall.ActionInstalled:
			fmt.Printf(i18n.M.Output.Config.SkillInstalled+"\n", r.Name)
		case skillinstall.ActionUpdated:
			fmt.Printf(i18n.M.Output.Config.SkillUpdated+"\n", r.Name)
		// ActionUpToDate: silent
		}
	}
}
