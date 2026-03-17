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
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/spf13/cobra"
	"github.com/robobee/core/internal/config"
)

var errInterrupted = errors.New("interrupted")

var configTemplate = config.ConfigTemplate

type configValues struct {
	ServerPort   string
	ServerHost   string
	Debug        bool
	DBPath       string
	MCPAPIKey    string

	FeishuEnabled    bool
	FeishuAppID      string
	FeishuAppSecret  string

	DingtalkEnabled      bool
	DingtalkClientID     string
	DingtalkClientSecret string

	WecomEnabled bool
	WecomBotID   string
	WecomSecret  string

	ClaudePath      string
	ClaudeTimeout   string
	FeederTimeout   string
	MessageDebounce string
	FFprobePath     string
	FFmpegPath      string
}

var configOutputPath string

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "交互式生成配置文件",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runConfig(cmd, args); errors.Is(err, errInterrupted) {
			return nil
		} else {
			return err
		}
	},
}

func init() {
	configCmd.Flags().StringVarP(&configOutputPath, "output", "o", "config.yaml", "输出配置文件路径")
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
		FeishuEnabled:        cfg.Bee.Platforms.Feishu.Enabled,
		FeishuAppID:          cfg.Bee.Platforms.Feishu.AppID,
		FeishuAppSecret:      cfg.Bee.Platforms.Feishu.AppSecret,
		DingtalkEnabled:      cfg.Bee.Platforms.DingTalk.Enabled,
		DingtalkClientID:     cfg.Bee.Platforms.DingTalk.ClientID,
		DingtalkClientSecret: cfg.Bee.Platforms.DingTalk.ClientSecret,
		WecomEnabled:         cfg.Bee.Platforms.WeCom.Enabled,
		WecomBotID:           cfg.Bee.Platforms.WeCom.BotID,
		WecomSecret:          cfg.Bee.Platforms.WeCom.Secret,
		ClaudePath:           cfg.Bee.Claude.Path,
		ClaudeTimeout:        cfg.Bee.Claude.Timeout.String(),
		FeederTimeout:        cfg.Bee.Feeder.Timeout.String(),
		MessageDebounce:      cfg.Bee.MessageDebounce.String(),
		FFprobePath:          cfg.Bee.Media.FFprobePath,
		FFmpegPath:           cfg.Bee.Media.FFmpegPath,
	}
}

func runConfig(cmd *cobra.Command, args []string) error {
	vals := configValues{
		ServerPort:      "8080",
		ServerHost:      "localhost",
		DBPath:          "./data/robobee.db",
		ClaudePath:      "claude",
		ClaudeTimeout:   "30m",
		FeederTimeout:   "5m",
		MessageDebounce: "3s",
		FFprobePath:     "ffprobe",
		FFmpegPath:      "ffmpeg",
	}

	// If an existing config file exists, use its values as defaults
	if existing := loadExistingConfig(configOutputPath); existing != nil {
		fmt.Printf("已检测到现有配置文件: %s，将使用其中的值作为默认值。\n", configOutputPath)
		vals = *existing
	}

	// Step 1 — Basic config
	fmt.Println("\n=== 基本配置 ===")

	if err := survey.AskOne(&survey.Input{
		Message: "Server 端口:",
		Default: vals.ServerPort,
	}, &vals.ServerPort, survey.WithValidator(func(val interface{}) error {
		s, _ := val.(string)
		if _, err := strconv.Atoi(s); err != nil {
			return fmt.Errorf("端口必须是整数")
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
		Message: "Debug 模式?",
		Default: vals.Debug,
	}, &vals.Debug); err != nil {
		return handleSurveyErr(err)
	}

	if err := survey.AskOne(&survey.Input{
		Message: "数据库路径:",
		Default: vals.DBPath,
	}, &vals.DBPath); err != nil {
		return handleSurveyErr(err)
	}

	// Step 2 — Claude config
	fmt.Println("\n=== Claude 配置 ===")

	if err := configureClaudeExecutable(&vals); err != nil {
		return err
	}
	if err := configureClaudeProvider(&vals); err != nil {
		return err
	}

	// Step 3 — MCP config
	fmt.Println("\n=== MCP 配置 ===")

	mcpKeyChoice := "随机生成"
	if vals.MCPAPIKey != "" {
		mcpKeyChoice = "手动输入"
	}
	var mcpMethod string
	if err := survey.AskOne(&survey.Select{
		Message: "MCP API Key 设置方式:",
		Options: []string{"随机生成", "手动输入"},
		Default: mcpKeyChoice,
	}, &mcpMethod); err != nil {
		return handleSurveyErr(err)
	}

	switch mcpMethod {
	case "随机生成":
		b := make([]byte, 12)
		if _, err := rand.Read(b); err != nil {
			return fmt.Errorf("生成随机 key 失败: %w", err)
		}
		vals.MCPAPIKey = hex.EncodeToString(b)
		fmt.Printf("已生成 MCP API Key: %s\n", vals.MCPAPIKey)
	case "手动输入":
		mcpDefault := ""
		if vals.MCPAPIKey != "" {
			mcpDefault = vals.MCPAPIKey
		}
		if err := survey.AskOne(&survey.Input{
			Message: "MCP API Key:",
			Default: mcpDefault,
		}, &vals.MCPAPIKey, survey.WithValidator(survey.Required)); err != nil {
			return handleSurveyErr(err)
		}
	}

	// Step 4 — Platform config
	fmt.Println("\n=== 平台配置 ===")

	// Build default selections from existing config
	var defaultPlatforms []string
	if vals.FeishuEnabled {
		defaultPlatforms = append(defaultPlatforms, "飞书")
	}
	if vals.DingtalkEnabled {
		defaultPlatforms = append(defaultPlatforms, "钉钉")
	}
	if vals.WecomEnabled {
		defaultPlatforms = append(defaultPlatforms, "企微")
	}

	var selectedPlatforms []string
	if err := survey.AskOne(&survey.MultiSelect{
		Message: "启用哪些平台？",
		Options: []string{"飞书", "钉钉", "企微"},
		Default: defaultPlatforms,
	}, &selectedPlatforms); err != nil {
		return handleSurveyErr(err)
	}

	// Reset platform flags — they'll be re-enabled based on selection
	vals.FeishuEnabled = false
	vals.DingtalkEnabled = false
	vals.WecomEnabled = false

	for _, p := range selectedPlatforms {
		switch p {
		case "飞书":
			vals.FeishuEnabled = true
			if err := survey.AskOne(&survey.Input{
				Message: "飞书 App ID:",
				Default: vals.FeishuAppID,
			}, &vals.FeishuAppID, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
			if err := survey.AskOne(&survey.Input{
				Message: "飞书 App Secret:",
				Default: vals.FeishuAppSecret,
			}, &vals.FeishuAppSecret, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		case "钉钉":
			vals.DingtalkEnabled = true
			if err := survey.AskOne(&survey.Input{
				Message: "钉钉 Client ID:",
				Default: vals.DingtalkClientID,
			}, &vals.DingtalkClientID, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
			if err := survey.AskOne(&survey.Input{
				Message: "钉钉 Client Secret:",
				Default: vals.DingtalkClientSecret,
			}, &vals.DingtalkClientSecret, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		case "企微":
			vals.WecomEnabled = true
			if err := survey.AskOne(&survey.Input{
				Message: "企微 Bot ID:",
				Default: vals.WecomBotID,
			}, &vals.WecomBotID, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
			if err := survey.AskOne(&survey.Input{
				Message: "企微 Secret:",
				Default: vals.WecomSecret,
			}, &vals.WecomSecret, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		}
	}

	// Step 5 — Advanced config
	fmt.Println("\n=== 高级配置 ===")

	var customAdvanced bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "是否自定义高级配置？",
		Default: false,
	}, &customAdvanced); err != nil {
		return handleSurveyErr(err)
	}

	if customAdvanced {
		if err := survey.AskOne(&survey.Input{
			Message: "Feeder 超时:",
			Default: vals.FeederTimeout,
		}, &vals.FeederTimeout); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: "消息去抖时间:",
			Default: vals.MessageDebounce,
		}, &vals.MessageDebounce); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: "FFprobe 路径:",
			Default: vals.FFprobePath,
		}, &vals.FFprobePath); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: "FFmpeg 路径:",
			Default: vals.FFmpegPath,
		}, &vals.FFmpegPath); err != nil {
			return handleSurveyErr(err)
		}
	}

	// Step 6 — Confirm write
	fmt.Printf("\n=== 写入配置 ===\n")
	fmt.Printf("输出文件: %s\n", configOutputPath)

	var confirmWrite bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "确认写入配置文件？",
		Default: true,
	}, &confirmWrite); err != nil {
		return handleSurveyErr(err)
	}
	if !confirmWrite {
		fmt.Println("已取消写入。")
		return nil
	}

	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		return fmt.Errorf("解析模板失败: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vals); err != nil {
		return fmt.Errorf("渲染模板失败: %w", err)
	}

	if err := os.WriteFile(configOutputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	fmt.Printf("配置文件已生成: %s\n", configOutputPath)
	return nil
}

func handleSurveyErr(err error) error {
	if errors.Is(err, terminal.InterruptErr) {
		fmt.Println("\n已取消。")
		return errInterrupted
	}
	return err
}
