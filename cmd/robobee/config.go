package main

import (
	"bytes"
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

	// Step 1 — Basic config
	fmt.Println("\n=== 基本配置 ===")

	if err := survey.AskOne(&survey.Input{
		Message: "Server 端口:",
		Default: "8080",
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
		Default: "localhost",
	}, &vals.ServerHost); err != nil {
		return handleSurveyErr(err)
	}

	if err := survey.AskOne(&survey.Confirm{
		Message: "Debug 模式?",
		Default: false,
	}, &vals.Debug); err != nil {
		return handleSurveyErr(err)
	}

	if err := survey.AskOne(&survey.Input{
		Message: "数据库路径:",
		Default: "./data/robobee.db",
	}, &vals.DBPath); err != nil {
		return handleSurveyErr(err)
	}

	// Step 2 — MCP config
	fmt.Println("\n=== MCP 配置 ===")

	if err := survey.AskOne(&survey.Password{
		Message: "MCP API Key:",
	}, &vals.MCPAPIKey, survey.WithValidator(survey.Required)); err != nil {
		return handleSurveyErr(err)
	}

	// Step 3 — Platform config
	fmt.Println("\n=== 平台配置 ===")

	var selectedPlatforms []string
	if err := survey.AskOne(&survey.MultiSelect{
		Message: "启用哪些平台？",
		Options: []string{"飞书", "钉钉", "企微"},
	}, &selectedPlatforms); err != nil {
		return handleSurveyErr(err)
	}

	for _, p := range selectedPlatforms {
		switch p {
		case "飞书":
			vals.FeishuEnabled = true
			if err := survey.AskOne(&survey.Input{
				Message: "飞书 App ID:",
			}, &vals.FeishuAppID, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
			if err := survey.AskOne(&survey.Password{
				Message: "飞书 App Secret:",
			}, &vals.FeishuAppSecret, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		case "钉钉":
			vals.DingtalkEnabled = true
			if err := survey.AskOne(&survey.Input{
				Message: "钉钉 Client ID:",
			}, &vals.DingtalkClientID, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
			if err := survey.AskOne(&survey.Password{
				Message: "钉钉 Client Secret:",
			}, &vals.DingtalkClientSecret, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		case "企微":
			vals.WecomEnabled = true
			if err := survey.AskOne(&survey.Input{
				Message: "企微 Bot ID:",
			}, &vals.WecomBotID, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
			if err := survey.AskOne(&survey.Password{
				Message: "企微 Secret:",
			}, &vals.WecomSecret, survey.WithValidator(survey.Required)); err != nil {
				return handleSurveyErr(err)
			}
		}
	}

	// Step 4 — Advanced config
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
			Message: "Claude 可执行文件路径:",
			Default: "claude",
		}, &vals.ClaudePath); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: "Claude 超时:",
			Default: "30m",
		}, &vals.ClaudeTimeout); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: "Feeder 超时:",
			Default: "5m",
		}, &vals.FeederTimeout); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: "消息去抖时间:",
			Default: "3s",
		}, &vals.MessageDebounce); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: "FFprobe 路径:",
			Default: "ffprobe",
		}, &vals.FFprobePath); err != nil {
			return handleSurveyErr(err)
		}

		if err := survey.AskOne(&survey.Input{
			Message: "FFmpeg 路径:",
			Default: "ffmpeg",
		}, &vals.FFmpegPath); err != nil {
			return handleSurveyErr(err)
		}
	}

	// Step 5 — Confirm write
	fmt.Printf("\n=== 写入配置 ===\n")
	fmt.Printf("输出文件: %s\n", configOutputPath)

	if _, err := os.Stat(configOutputPath); err == nil {
		var overwrite bool
		if err := survey.AskOne(&survey.Confirm{
			Message: fmt.Sprintf("文件 %s 已存在，是否覆盖？", configOutputPath),
			Default: false,
		}, &overwrite); err != nil {
			return handleSurveyErr(err)
		}
		if !overwrite {
			fmt.Println("已取消写入。")
			return nil
		}
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
