package configcmd

import (
	"fmt"
	"slices"

	"github.com/AlecAivazis/survey/v2"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/plugin"
)

type platformSpec struct {
	enabled   *bool
	label     string
	configure func(*configValues) error
}

func platformSpecs(vals *configValues) []platformSpec {
	return []platformSpec{
		{&vals.FeishuEnabled, i18n.M.Prompt.PlatformFeishu, configureFeishu},
		{&vals.DingtalkEnabled, i18n.M.Prompt.PlatformDingTalk, configureDingtalk},
		{&vals.WecomEnabled, i18n.M.Prompt.PlatformWeCom, configureWecom},
		{&vals.TelegramEnabled, i18n.M.Prompt.PlatformTelegram, configureTelegram},
		{&vals.WeixinEnabled, i18n.M.Prompt.PlatformWeixin, configureWeixin},
		{&vals.LinearEnabled, i18n.M.Prompt.PlatformLinear, configureLinear},
	}
}

func configureFeishu(vals *configValues) error {
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
	return promptBotName(&vals.FeishuBotName)
}

func configureDingtalk(vals *configValues) error {
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
	return promptBotName(&vals.DingtalkBotName)
}

func configureWecom(vals *configValues) error {
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
	return promptBotName(&vals.WecomBotName)
}

func configureTelegram(vals *configValues) error {
	if err := survey.AskOne(&survey.Password{
		Message: i18n.M.Prompt.TelegramToken,
		Help:    i18n.M.Prompt.TelegramTokenHelp,
	}, &vals.TelegramToken, survey.WithValidator(survey.Required)); err != nil {
		return handleSurveyErr(err)
	}
	authCodeDefault := vals.TelegramAuthCode
	if authCodeDefault == "" {
		authCodeDefault = randomHex(8)
	}
	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.TelegramAuthCode,
		Default: authCodeDefault,
		Help:    i18n.M.Prompt.TelegramAuthCodeHelp,
	}, &vals.TelegramAuthCode); err != nil {
		return handleSurveyErr(err)
	}
	return promptBotName(&vals.TelegramBotName)
}

func configureWeixin(vals *configValues) error {
	if err := weixinAcquireToken(vals); err != nil {
		return err
	}
	if vals.WeixinBaseURL == "" {
		vals.WeixinBaseURL = "https://ilinkai.weixin.qq.com"
	}
	vals.WeixinCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
	return promptBotName(&vals.WeixinBotName)
}

func weixinAcquireToken(vals *configValues) error {
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
	if !needQRLogin {
		return nil
	}

	fmt.Println(i18n.M.Output.Config.WeixinQRLogin)
	fmt.Println(i18n.M.Output.Config.FetchingQR)

	token, userID, baseURL, err := runWeixinQRLogin()
	if err == nil {
		vals.WeixinToken = token
		vals.WeixinUserID = userID
		if baseURL != "" {
			vals.WeixinBaseURL = baseURL
		}
		fmt.Println(i18n.M.Output.Config.WeixinSuccess)
		return nil
	}

	fmt.Printf(i18n.M.Output.Config.QRFailed+"\n", err)
	fmt.Println(i18n.M.Output.Config.QRFallback)
	if err := survey.AskOne(&survey.Password{
		Message: i18n.M.Prompt.WeixinBotToken,
	}, &vals.WeixinToken, survey.WithValidator(survey.Required)); err != nil {
		return handleSurveyErr(err)
	}
	return handleSurveyErr(survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.WeixinUserID,
		Default: vals.WeixinUserID,
	}, &vals.WeixinUserID, survey.WithValidator(survey.Required)))
}

func configureLinear(vals *configValues) error {
	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.LinearAPIKey,
		Help:    i18n.M.Prompt.LinearAPIKeyHelp,
		Default: vals.LinearAPIKey,
	}, &vals.LinearAPIKey, survey.WithValidator(survey.Required)); err != nil {
		return handleSurveyErr(err)
	}
	labelDefault := vals.LinearLabelName
	if labelDefault == "" {
		labelDefault = "openbee"
	}
	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.LinearLabelName,
		Default: labelDefault,
	}, &vals.LinearLabelName); err != nil {
		return handleSurveyErr(err)
	}
	if vals.LinearPollInterval == "" {
		vals.LinearPollInterval = "10s"
	}
	fmt.Println(i18n.M.Prompt.LinearProjectsHelp)
	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.LinearProjects,
		Default: vals.LinearProjects,
	}, &vals.LinearProjects); err != nil {
		return handleSurveyErr(err)
	}
	fmt.Println(i18n.M.Prompt.LinearStatesHelp)
	return handleSurveyErr(survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.LinearStates,
		Default: vals.LinearStates,
	}, &vals.LinearStates))
}

// runPlatformStep runs the multi-select prompt and per-platform configuration.
func runPlatformStep(vals *configValues) error {
	specs := platformSpecs(vals)
	plugins := loadPluginOptions(plugin.OpenRegistry(plugin.DefaultDir()), vals.Language)

	var defaults []string
	for _, s := range specs {
		if *s.enabled {
			defaults = append(defaults, s.label)
		}
	}
	options := make([]string, 0, len(specs)+len(plugins))
	for _, s := range specs {
		options = append(options, s.label)
	}
	for _, po := range plugins {
		options = append(options, po.Label)
		if on, _ := vals.PluginSections[po.Manifest.ID]["enabled"].(bool); on {
			defaults = append(defaults, po.Label)
		}
	}

	var selected []string
	if err := survey.AskOne(&survey.MultiSelect{
		Message: i18n.M.Prompt.PlatformSelect,
		Options: options,
		Default: defaults,
	}, &selected); err != nil {
		return handleSurveyErr(err)
	}

	for _, s := range specs {
		*s.enabled = false
	}

	byLabel := make(map[string]platformSpec, len(specs))
	for _, s := range specs {
		byLabel[s.label] = s
	}
	for _, p := range selected {
		spec, ok := byLabel[p]
		if !ok {
			continue
		}
		*spec.enabled = true
		if err := spec.configure(vals); err != nil {
			return err
		}
	}
	return configurePlugins(vals, plugins, selected)
}

func configurePlugins(vals *configValues, plugins []pluginOption, selectedLabels []string) error {
	offered := make([]string, 0, len(plugins))
	var selected []string
	for _, po := range plugins {
		offered = append(offered, po.Manifest.ID)
		if slices.Contains(selectedLabels, po.Label) {
			selected = append(selected, po.Manifest.ID)
		}
	}
	applyPluginSelection(vals, offered, selected)
	for _, po := range plugins {
		id := po.Manifest.ID
		if !slices.Contains(selected, id) {
			continue
		}
		fmt.Println(po.Label)
		section, err := promptPluginSection(po.Manifest, vals.Language, vals.PluginSections[id])
		if err != nil {
			return err
		}
		if vals.PluginSections == nil {
			vals.PluginSections = map[string]map[string]any{}
		}
		vals.PluginSections[id] = section
	}
	return nil
}
