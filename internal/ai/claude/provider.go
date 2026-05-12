package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

// ErrInterrupted is returned when the user cancels an interactive prompt (Ctrl+C).
var ErrInterrupted = errors.New("interrupted")

// Provider display names used in the selection menu and switch cases.
const (
	ProviderKimiCode   = "KimiCode"
	ProviderMoonshot   = "Moonshot (Kimi)"
	ProviderDeepSeek   = "DeepSeek"
	ProviderGLM        = "Zhipu (GLM)"
	ProviderMiniMax    = "MiniMax"
	ProviderAliyun     = "Alibaba Cloud (Qwen)"
	ProviderVolcengine = "Volcengine (Doubao)"
	ProviderTencent    = "Tencent Cloud"
	ProviderMimo       = "Xiaomi Mimo"
	ProviderCustom     = "Custom provider"
)

// Anthropic environment variable keys.
const (
	envAnthropicAuthToken            = "ANTHROPIC_AUTH_TOKEN"
	envAnthropicAPIKey               = "ANTHROPIC_API_KEY"
	envAnthropicBaseURL              = "ANTHROPIC_BASE_URL"
	envAnthropicModel                = "ANTHROPIC_MODEL"
	envAnthropicSmallFastModel       = "ANTHROPIC_SMALL_FAST_MODEL"
	envAnthropicDefaultSonnetModel   = "ANTHROPIC_DEFAULT_SONNET_MODEL"
	envAnthropicDefaultOpusModel     = "ANTHROPIC_DEFAULT_OPUS_MODEL"
	envAnthropicDefaultHaikuModel    = "ANTHROPIC_DEFAULT_HAIKU_MODEL"
	envClaudeCodeSubagentModel       = "CLAUDE_CODE_SUBAGENT_MODEL"
	envEnableToolSearch              = "ENABLE_TOOL_SEARCH"
	envAPITimeoutMS                  = "API_TIMEOUT_MS"
	envClaudeCodeDisableNonessential = "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"
)

// HandleSurveyErr converts a survey interrupt into ErrInterrupted and passes
// other errors through unchanged. It is exported so that cmd-layer code can
// share the same sentinel error without duplicating the check.
func HandleSurveyErr(err error) error {
	if errors.Is(err, terminal.InterruptErr) {
		fmt.Println(i18n.M.Prompt.Cancelled)
		return ErrInterrupted
	}
	return err
}

func promptAPIKey(message string) (string, error) {
	var apiKey string
	if err := survey.AskOne(&survey.Input{
		Message: message,
	}, &apiKey, survey.WithValidator(survey.Required)); err != nil {
		return "", HandleSurveyErr(err)
	}
	return apiKey, nil
}

// Provider env map builders — ANTHROPIC_AUTH_TOKEN or ANTHROPIC_API_KEY comes from user input depending on the provider.

func kimiCodeEnv(apiKey string) map[string]string {
	return map[string]string{
		envAnthropicBaseURL: "https://api.kimi.com/coding/",
		envAnthropicAPIKey:  apiKey,
		envEnableToolSearch: "false",
	}
}

func moonshotEnv(apiKey string) map[string]string {
	return map[string]string{
		envAnthropicBaseURL:              "https://api.moonshot.cn/anthropic",
		envAnthropicAuthToken:            apiKey,
		envAnthropicModel:                "kimi-k2.5",
		envAnthropicSmallFastModel:       "kimi-k2.5",
		envAnthropicDefaultOpusModel:     "kimi-k2.5",
		envAnthropicDefaultSonnetModel:   "kimi-k2.5",
		envAnthropicDefaultHaikuModel:    "kimi-k2.5",
		envClaudeCodeSubagentModel:       "kimi-k2.5",
		envClaudeCodeDisableNonessential: "1",
		envEnableToolSearch:              "false",
		envAPITimeoutMS:                  "600000",
	}
}

func glmEnv(apiKey string) map[string]string {
	return map[string]string{
		envAnthropicAuthToken:            apiKey,
		envAnthropicBaseURL:              "https://open.bigmodel.cn/api/anthropic",
		envAnthropicDefaultHaikuModel:    "glm-4.5-air",
		envAnthropicDefaultSonnetModel:   "glm-5-turbo",
		envAnthropicDefaultOpusModel:     "glm-5.1",
		envAPITimeoutMS:                  "3000000",
		envClaudeCodeDisableNonessential: "1",
	}
}

func minimaxEnv(apiKey string) map[string]string {
	return map[string]string{
		envAnthropicBaseURL:              "https://api.minimaxi.com/anthropic",
		envAnthropicAuthToken:            apiKey,
		envAPITimeoutMS:                  "3000000",
		envClaudeCodeDisableNonessential: "1",
		envAnthropicModel:                "MiniMax-M2.7",
		envAnthropicSmallFastModel:       "MiniMax-M2.7",
		envAnthropicDefaultSonnetModel:   "MiniMax-M2.7",
		envAnthropicDefaultOpusModel:     "MiniMax-M2.7",
		envAnthropicDefaultHaikuModel:    "MiniMax-M2.7",
	}
}

func deepseekEnv(apiKey string) map[string]string {
	return map[string]string{
		envAnthropicBaseURL:              "https://api.deepseek.com/anthropic",
		envAnthropicAuthToken:            apiKey,
		envAnthropicModel:                "deepseek-chat",
		envAnthropicSmallFastModel:       "deepseek-chat",
		envClaudeCodeDisableNonessential: "1",
		envAPITimeoutMS:                  "600000",
	}
}

func standardProviderEnv(baseURL, apiKey, model string) map[string]string {
	return map[string]string{
		envAnthropicAuthToken:            apiKey,
		envAnthropicBaseURL:              baseURL,
		envAnthropicModel:                model,
		envClaudeCodeDisableNonessential: "1",
	}
}

func aliyunEnv(apiKey, model string) map[string]string {
	return standardProviderEnv("https://coding.dashscope.aliyuncs.com/apps/anthropic", apiKey, model)
}

func volcengineEnv(apiKey, model string) map[string]string {
	return standardProviderEnv("https://ark.cn-beijing.volces.com/api/coding", apiKey, model)
}

func tencentEnv(apiKey, model string) map[string]string {
	return standardProviderEnv("https://api.lkeap.cloud.tencent.com/coding/anthropic", apiKey, model)
}

func mimoEnv(baseURL, apiKey string) map[string]string {
	return map[string]string{
		envAnthropicBaseURL:              baseURL,
		envAnthropicAuthToken:            apiKey,
		envAnthropicModel:                "mimo-v2.5-pro",
		envAnthropicDefaultSonnetModel:   "mimo-v2.5-pro",
		envAnthropicDefaultOpusModel:     "mimo-v2.5-pro",
		envAnthropicDefaultHaikuModel:    "mimo-v2.5-pro",
		envClaudeCodeDisableNonessential: "1",
		envAPITimeoutMS:                  "3000000",
	}
}

func customEnv(baseURL, apiKey string) map[string]string {
	return map[string]string{
		envAnthropicBaseURL:   baseURL,
		envAnthropicAuthToken: apiKey,
	}
}

// providerEnvKeys lists all environment variable keys that any provider may write.
// These are cleared before writing new provider settings so that stale keys from a
// previous provider do not linger after switching.
var providerEnvKeys = []string{
	envAnthropicAuthToken,
	envAnthropicAPIKey,
	envAnthropicBaseURL,
	envAnthropicModel,
	envAnthropicSmallFastModel,
	envAnthropicDefaultSonnetModel,
	envAnthropicDefaultOpusModel,
	envAnthropicDefaultHaikuModel,
	envClaudeCodeSubagentModel,
	envEnableToolSearch,
	envAPITimeoutMS,
	envClaudeCodeDisableNonessential,
}

// mergeJSONFile reads the JSON file at path (if it exists) into a map,
// calls apply to mutate the map, then writes it back with indentation.
func mergeJSONFile(path string, apply func(map[string]any)) error {
	existing := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			// Non-fatal: log and overwrite the corrupted file.
			fmt.Fprintf(os.Stderr, "warning: %s has invalid JSON, overwriting: %v\n", path, err)
		}
	}
	apply(existing)
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// mergeClaudeSettings reads existing ~/.claude/settings.json (if any),
// removes all known provider env keys, merges the provided env map, and writes back.
func mergeClaudeSettings(path string, env map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	return mergeJSONFile(path, func(m map[string]any) {
		envMap, ok := m["env"].(map[string]any)
		if !ok {
			envMap = make(map[string]any)
		}
		for _, k := range providerEnvKeys {
			delete(envMap, k)
		}
		for k, v := range env {
			envMap[k] = v
		}
		m["env"] = envMap
	})
}

// mergeClaudeJSON reads existing ~/.claude.json (if any),
// sets hasCompletedOnboarding=true, preserves all other keys, and writes back.
func mergeClaudeJSON(path string) error {
	return mergeJSONFile(path, func(m map[string]any) {
		m["hasCompletedOnboarding"] = true
	})
}

// providerSpec describes one interactive provider configuration entry.
type providerSpec struct {
	Name           string        // display name shown in selection menu (matches ProviderXxx const)
	KeyPrompt      func() string // returns the i18n string for prompting the API key
	BaseURLPrompt  func() string // non-nil ⇒ prompt for base URL BEFORE the API key (Mimo / Custom)
	ModelOptions   []string      // non-empty ⇒ show survey.Select for model picker
	ModelDefault   string        // default selection for ModelOptions
	NeedClaudeJSON bool          // true ⇒ also write ~/.claude.json (GLM/MiniMax/Volcengine/Tencent/Mimo)
	BuildEnv       func(apiKey, modelOrBaseURL string) map[string]string
}

var providerSpecs = []providerSpec{
	{
		Name:      ProviderKimiCode,
		KeyPrompt: func() string { return i18n.M.Provider.KeyKimiCode },
		BuildEnv:  func(k, _ string) map[string]string { return kimiCodeEnv(k) },
	},
	{
		Name:      ProviderMoonshot,
		KeyPrompt: func() string { return i18n.M.Provider.KeyMoonshot },
		BuildEnv:  func(k, _ string) map[string]string { return moonshotEnv(k) },
	},
	{
		Name:      ProviderDeepSeek,
		KeyPrompt: func() string { return i18n.M.Provider.KeyDeepSeek },
		BuildEnv:  func(k, _ string) map[string]string { return deepseekEnv(k) },
	},
	{
		Name:           ProviderGLM,
		KeyPrompt:      func() string { return i18n.M.Provider.KeyGLM },
		NeedClaudeJSON: true,
		BuildEnv:       func(k, _ string) map[string]string { return glmEnv(k) },
	},
	{
		Name:           ProviderMiniMax,
		KeyPrompt:      func() string { return i18n.M.Provider.KeyMiniMax },
		NeedClaudeJSON: true,
		BuildEnv:       func(k, _ string) map[string]string { return minimaxEnv(k) },
	},
	{
		Name:         ProviderAliyun,
		KeyPrompt:    func() string { return i18n.M.Provider.KeyAliyun },
		ModelOptions: []string{"qwen3.5-plus", "kimi-k2.5", "glm-5", "MiniMax-M2.5"},
		ModelDefault: "qwen3.5-plus",
		BuildEnv:     aliyunEnv,
	},
	{
		Name:           ProviderVolcengine,
		KeyPrompt:      func() string { return i18n.M.Provider.KeyVolcengine },
		NeedClaudeJSON: true,
		ModelOptions: []string{
			"doubao-seed-2.0-code",
			"doubao-seed-2.0-pro",
			"doubao-seed-2.0-lite",
			"doubao-seed-code",
			"minimax-m2.5",
			"glm-4.7",
			"deepseek-v3.2",
			"kimi-k2.5",
		},
		ModelDefault: "doubao-seed-2.0-code",
		BuildEnv:     volcengineEnv,
	},
	{
		Name:           ProviderTencent,
		KeyPrompt:      func() string { return i18n.M.Provider.KeyTencent },
		NeedClaudeJSON: true,
		ModelOptions: []string{
			"tc-code-latest（auto）",
			"hunyuan-2.0-instruct",
			"hunyuan-2.0-thinking",
			"minimax-m2.5",
			"kimi-k2.5",
			"glm-5",
			"hunyuan-t1",
			"hunyuan-turbos",
		},
		ModelDefault: "tc-code-latest（auto）",
		BuildEnv:     tencentEnv,
	},
	{
		Name:           ProviderMimo,
		KeyPrompt:      func() string { return i18n.M.Provider.KeyMimoToken },
		BaseURLPrompt:  func() string { return i18n.M.Provider.KeyMimoURL },
		NeedClaudeJSON: true,
		BuildEnv:       func(k, baseURL string) map[string]string { return mimoEnv(baseURL, k) },
	},
	{
		Name:          ProviderCustom,
		KeyPrompt:     func() string { return i18n.M.Provider.KeyCustomToken },
		BaseURLPrompt: func() string { return i18n.M.Provider.KeyCustomURL },
		BuildEnv:      func(k, baseURL string) map[string]string { return customEnv(baseURL, k) },
	},
}

// ConfigureProvider runs an interactive survey to select a provider and API key,
// then writes the result to ~/.claude/settings.json (and ~/.claude.json where needed).
// Returns ErrInterrupted if the user cancels with Ctrl+C.
func ConfigureProvider() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeJSONPath := filepath.Join(home, ".claude.json")

	if _, err := os.Stat(settingsPath); err == nil {
		var skip bool
		if err := survey.AskOne(&survey.Confirm{
			Message: i18n.M.Provider.FoundSettings,
			Default: true,
		}, &skip); err != nil {
			return HandleSurveyErr(err)
		}
		if skip {
			return nil
		}
	}

	names := make([]string, len(providerSpecs))
	specByName := make(map[string]*providerSpec, len(providerSpecs))
	for i := range providerSpecs {
		names[i] = providerSpecs[i].Name
		specByName[providerSpecs[i].Name] = &providerSpecs[i]
	}

	var providerName string
	if err := survey.AskOne(&survey.Select{
		Message: i18n.M.Provider.Select,
		Options: names,
	}, &providerName); err != nil {
		return HandleSurveyErr(err)
	}

	spec, ok := specByName[providerName]
	if !ok {
		return fmt.Errorf("unknown provider: %s", providerName)
	}

	// Mimo/Custom prompt for base URL FIRST.
	var baseURL string
	if spec.BaseURLPrompt != nil {
		v, err := promptAPIKey(spec.BaseURLPrompt())
		if err != nil {
			return err
		}
		baseURL = v
	}

	apiKey, err := promptAPIKey(spec.KeyPrompt())
	if err != nil {
		return err
	}

	// Determine the second argument passed to BuildEnv:
	//   - if a model picker is configured, prompt and use the selected model
	//   - else if a base URL was prompted, pass it through
	//   - else pass empty string (single-key providers ignore the second arg)
	secondArg := ""
	if len(spec.ModelOptions) > 0 {
		if err := survey.AskOne(&survey.Select{
			Message: i18n.M.Provider.SelectModel,
			Options: spec.ModelOptions,
			Default: spec.ModelDefault,
		}, &secondArg); err != nil {
			return HandleSurveyErr(err)
		}
	} else if baseURL != "" {
		secondArg = baseURL
	}

	env := spec.BuildEnv(apiKey, secondArg)

	if err := mergeClaudeSettings(settingsPath, env); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}
	fmt.Println(i18n.M.Provider.WrittenSettings)

	if spec.NeedClaudeJSON {
		if err := mergeClaudeJSON(claudeJSONPath); err != nil {
			return fmt.Errorf("write .claude.json: %w", err)
		}
		fmt.Println(i18n.M.Provider.WrittenJSON)
	}
	return nil
}
