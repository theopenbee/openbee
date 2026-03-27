package i18n

// Messages 包含所有 CLI 用户可见文本。
// 字段名通过 yaml tag 与 YAML 键对应。
type Messages struct {
	Cmd    CmdMessages    `yaml:"cmd"`
	Prompt PromptMessages `yaml:"prompt"`
}

// CmdMessages 对应所有 cobra 命令的 Short/Long 描述。
type CmdMessages struct {
	Root struct {
		Short string `yaml:"short"`
	} `yaml:"root"`
	Config struct {
		Short string `yaml:"short"`
	} `yaml:"config"`
	Server struct {
		Short string `yaml:"short"`
	} `yaml:"server"`
	Stop struct {
		Short string `yaml:"short"`
	} `yaml:"stop"`
	Restart struct {
		Short string `yaml:"short"`
	} `yaml:"restart"`
	Status struct {
		Short string `yaml:"short"`
	} `yaml:"status"`
	Upgrade struct {
		Short string `yaml:"short"`
		Long  string `yaml:"long"`
	} `yaml:"upgrade"`
	Claude struct {
		Short string `yaml:"short"`
	} `yaml:"claude"`
	ClaudeDownload struct {
		Short string `yaml:"short"`
	} `yaml:"claude_download"`
	ClaudeEnv struct {
		Short string `yaml:"short"`
	} `yaml:"claude_env"`
}

// PromptMessages 对应所有 survey 交互提示的 Message 字段。
type PromptMessages struct {
	// Claude setup
	ClaudeNotFound string `yaml:"claude_not_found"`
	ClaudePath     string `yaml:"claude_path"`
	ClaudeTimeout  string `yaml:"claude_timeout"`
	// Platform
	PlatformSelect       string `yaml:"platform_select"`
	FeishuAppID          string `yaml:"feishu_app_id"`
	FeishuAppSecret      string `yaml:"feishu_app_secret"`
	DingtalkClientID     string `yaml:"dingtalk_client_id"`
	DingtalkClientSecret string `yaml:"dingtalk_client_secret"`
	WecomBotID           string `yaml:"wecom_bot_id"`
	WecomSecret          string `yaml:"wecom_secret"`
	TelegramToken        string `yaml:"telegram_token"`
	TelegramAuthCode     string `yaml:"telegram_auth_code"`
	WeixinReacquire      string `yaml:"weixin_reacquire"` // contains %s placeholder
	WeixinBotToken       string `yaml:"weixin_bot_token"`
	WeixinUserID         string `yaml:"weixin_user_id"`
	// Auth
	Username              string `yaml:"username"`
	PasswordChangeConfirm string `yaml:"password_change_confirm"`
	PasswordSetup         string `yaml:"password_setup"`
	Password              string `yaml:"password"`
	JWTRegenConfirm       string `yaml:"jwt_regen_confirm"`
	// Advanced
	AdvancedConfirm      string `yaml:"advanced_confirm"`
	ServerPort           string `yaml:"server_port"`
	ServerHost           string `yaml:"server_host"`
	DebugMode            string `yaml:"debug_mode"`
	DBPath               string `yaml:"db_path"`
	MCPAPIKeySetup       string `yaml:"mcp_api_key_setup"`
	MCPAPIKey            string `yaml:"mcp_api_key"`
	MCPWorkerAPIKeySetup string `yaml:"mcp_worker_api_key_setup"`
	MCPWorkerAPIKey      string `yaml:"mcp_worker_api_key"`
	FeederTimeout        string `yaml:"feeder_timeout"`
	MaxConcurrentBee     string `yaml:"max_concurrent_bee"`
	MessageDebounce      string `yaml:"message_debounce"`
	FFprobePath          string `yaml:"ffprobe_path"`
	FFmpegPath           string `yaml:"ffmpeg_path"`
	// Write
	ConfirmWrite string `yaml:"confirm_write"`
}
