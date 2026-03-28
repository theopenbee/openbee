package i18n

// Messages 包含所有 CLI 用户可见文本。
// 字段名通过 yaml tag 与 YAML 键对应。
type Messages struct {
	Cmd      CmdMessages      `yaml:"cmd"`
	Prompt   PromptMessages   `yaml:"prompt"`
	Flag     FlagMessages     `yaml:"flag"`
	Output   OutputMessages   `yaml:"output"`
	Provider ProviderMessages `yaml:"provider"`
	Validate ValidateMessages `yaml:"validate"`
}

// CmdEntry holds the Short and optional Long description for a cobra command.
type CmdEntry struct {
	Short string `yaml:"short"`
	Long  string `yaml:"long"`
}

// CmdMessages 对应所有 cobra 命令的 Short/Long 描述。
type CmdMessages struct {
	Root           CmdEntry `yaml:"root"`
	Config         CmdEntry `yaml:"config"`
	Server         CmdEntry `yaml:"server"`
	Stop           CmdEntry `yaml:"stop"`
	Restart        CmdEntry `yaml:"restart"`
	Status         CmdEntry `yaml:"status"`
	Upgrade        CmdEntry `yaml:"upgrade"`
	Backup         CmdEntry `yaml:"backup"`
	Restore        CmdEntry `yaml:"restore"`
	Claude         CmdEntry `yaml:"claude"`
	ClaudeDownload CmdEntry `yaml:"claude_download"`
	ClaudeEnv      CmdEntry `yaml:"claude_env"`
}

// PromptMessages 对应所有 survey 交互提示的 Message 字段。
type PromptMessages struct {
	// Claude setup
	ClaudeNotFound string `yaml:"claude_not_found"`
	ClaudePath     string `yaml:"claude_path"`
	ClaudeTimeout  string `yaml:"claude_timeout"`
	// Platform
	PlatformSelect   string `yaml:"platform_select"`
	PlatformFeishu   string `yaml:"platform_feishu"`
	PlatformDingTalk string `yaml:"platform_dingtalk"`
	PlatformWeCom    string `yaml:"platform_wecom"`
	PlatformTelegram string `yaml:"platform_telegram"`
	PlatformWeixin   string `yaml:"platform_weixin"`
	// Survey hint text
	MultiSelectHint string `yaml:"multiselect_hint"`
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
	// Survey options (used in both Options slice and switch cases)
	OptionEnterManually     string `yaml:"option_enter_manually"`
	OptionGenerateRandom    string `yaml:"option_generate_random"`
	OptionEnterPathManually string `yaml:"option_enter_path_manually"`
	OptionDownloadClaude    string `yaml:"option_download_claude"`
	// Telegram Help text
	TelegramTokenHelp    string `yaml:"telegram_token_help"`
	TelegramAuthCodeHelp string `yaml:"telegram_auth_code_help"`
	// Generic cancel
	Cancelled string `yaml:"cancelled"`
}

// FlagMessages 对应所有 cobra flag 的 Usage 说明。
type FlagMessages struct {
	ConfigPath          string `yaml:"config_path"`
	ServerDaemon        string `yaml:"server_daemon"`
	ConfigOutput        string `yaml:"config_output"`
	BackupPassword      string `yaml:"backup_password"`
	RestorePassword     string `yaml:"restore_password"`
	RestoreForce        string `yaml:"restore_force"`
	UpgradeCheck        string `yaml:"upgrade_check"`
	ClaudeDownloadForce string `yaml:"claude_download_force"`
}

// ProviderMessages 对应 claude/provider.go 中所有用户可见文本。
type ProviderMessages struct {
	FoundSettings   string `yaml:"found_settings"`
	Select          string `yaml:"select"`
	SelectModel     string `yaml:"select_model"`
	KeyMoonshot     string `yaml:"key_moonshot"`
	KeyDeepSeek     string `yaml:"key_deepseek"`
	KeyGLM          string `yaml:"key_glm"`
	KeyMiniMax      string `yaml:"key_minimax"`
	KeyAliyun       string `yaml:"key_aliyun"`
	KeyVolcengine   string `yaml:"key_volcengine"`
	KeyTencent      string `yaml:"key_tencent"`
	KeyCustomURL    string `yaml:"key_custom_url"`
	KeyCustomToken  string `yaml:"key_custom_token"`
	WrittenSettings string `yaml:"written_settings"`
	WrittenJSON     string `yaml:"written_json"`
}

// ValidateMessages 对应交互式输入的校验错误提示。
type ValidateMessages struct {
	PortInteger     string `yaml:"port_integer"`
	PositiveInteger string `yaml:"positive_integer"`
	FileNotFound    string `yaml:"file_not_found"` // contains %s
	PathIsDir       string `yaml:"path_is_dir"`    // contains %s
	FileNotExec     string `yaml:"file_not_exec"`  // contains %s
}

// OutputMessages 对应所有命令的运行时输出文本。
type OutputMessages struct {
	Stop    StopOutput    `yaml:"stop"`
	Status  StatusOutput  `yaml:"status"`
	Upgrade UpgradeOutput `yaml:"upgrade"`
	Backup  BackupOutput  `yaml:"backup"`
	Restore RestoreOutput `yaml:"restore"`
	Config  ConfigOutput  `yaml:"config"`
	Claude  ClaudeOutput  `yaml:"claude"`
	Weixin  WeixinOutput  `yaml:"weixin"`
	Daemon  DaemonOutput  `yaml:"daemon"`
}

// StopOutput 对应 stop 命令的运行时输出。
type StopOutput struct {
	NotRunning string `yaml:"not_running"`
	Stale      string `yaml:"stale"`
	ForeignPID string `yaml:"foreign_pid"` // contains %d
	Stopping   string `yaml:"stopping"`    // contains %d
	Stopped    string `yaml:"stopped"`
}

// StatusOutput 对应 status 命令的运行时输出。
type StatusOutput struct {
	NotRunning      string `yaml:"not_running"`
	NotRunningStale string `yaml:"not_running_stale"`
	Running         string `yaml:"running"` // contains %d, %s
}

// UpgradeOutput 对应 upgrade 命令的运行时输出。
type UpgradeOutput struct {
	CurrentVersion  string `yaml:"current_version"`  // contains %s
	Checking        string `yaml:"checking"`
	LatestVersion   string `yaml:"latest_version"`   // contains %s
	UpToDate        string `yaml:"up_to_date"`
	NewVersion      string `yaml:"new_version"`      // contains %s
	RunCmd          string `yaml:"run_cmd"`
	Downloading     string `yaml:"downloading"`      // contains %s
	ChecksumWarning string `yaml:"checksum_warning"` // contains %v
	Verifying       string `yaml:"verifying"`
	Verified        string `yaml:"verified"`
	BinaryAt        string `yaml:"binary_at"` // contains %s
	Success         string `yaml:"success"`   // contains %s
}

// BackupOutput 对应 backup 命令的运行时输出。
type BackupOutput struct {
	Created string `yaml:"created"` // contains %s
}

// RestoreOutput 对应 restore 命令的运行时输出。
type RestoreOutput struct {
	Complete string `yaml:"complete"`
}

// ConfigOutput 对应 config 及 config_claude 的运行时输出。
type ConfigOutput struct {
	FoundExisting        string `yaml:"found_existing"`         // contains %s
	SectionClaude        string `yaml:"section_claude"`
	SectionPlatform      string `yaml:"section_platform"`
	SectionAuth          string `yaml:"section_auth"`
	SectionAdvanced      string `yaml:"section_advanced"`
	SectionWrite         string `yaml:"section_write"`
	OutputFile           string `yaml:"output_file"`            // contains %s
	WriteCancelled       string `yaml:"write_cancelled"`
	Written              string `yaml:"written"`                // contains %s
	JWTRegenerated       string `yaml:"jwt_regenerated"`
	JWTGenerated         string `yaml:"jwt_generated"`
	MCPKeyGenerated      string `yaml:"mcp_key_generated"`      // contains %s
	WorkerKeyGenerated   string `yaml:"worker_key_generated"`   // contains %s
	PasswordGenerated    string `yaml:"password_generated"`     // contains %s
	WeixinQRLogin        string `yaml:"weixin_qr_login"`
	FetchingQR           string `yaml:"fetching_qr"`
	QRFailed             string `yaml:"qr_failed"`              // contains %v
	QRFallback           string `yaml:"qr_fallback"`
	WeixinSuccess        string `yaml:"weixin_success"`
	ClaudeFound          string `yaml:"claude_found"`           // contains %s
	ClaudeDownloadFailed string `yaml:"claude_download_failed"` // contains %v
	ClaudeManualEntry    string `yaml:"claude_manual_entry"`
}

// ClaudeOutput 对应 claude 子命令的运行时输出。
type ClaudeOutput struct {
	AlreadyInstalled string `yaml:"already_installed"` // contains %s
	UseForce         string `yaml:"use_force"`
	InstalledAt      string `yaml:"installed_at"` // contains %s
}

// WeixinOutput 对应微信扫码登录流程的运行时输出。
type WeixinOutput struct {
	ScanQR       string `yaml:"scan_qr"`
	Waiting      string `yaml:"waiting"`
	PollFailed   string `yaml:"poll_failed"`   // contains %d, %v
	PollInvalid  string `yaml:"poll_invalid"`  // contains %d, %v
	Scanned      string `yaml:"scanned"`
	StillWaiting string `yaml:"still_waiting"`
	QRExpired    string `yaml:"qr_expired"`
	QRTimeout    string `yaml:"qr_timeout"`
}

// DaemonOutput 对应 daemon 相关的运行时输出。
type DaemonOutput struct {
	Started        string `yaml:"started"`         // contains %d
	AlreadyRunning string `yaml:"already_running"` // contains %d
}
