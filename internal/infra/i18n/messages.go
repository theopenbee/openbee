package i18n

// Messages holds all user-visible text, keyed by YAML tags.
type Messages struct {
	Cmd      CmdMessages      `yaml:"cmd"`
	Prompt   PromptMessages   `yaml:"prompt"`
	Flag     FlagMessages     `yaml:"flag"`
	Output   OutputMessages   `yaml:"output"`
	Provider ProviderMessages `yaml:"provider"`
	Validate ValidateMessages `yaml:"validate"`
	Runtime  RuntimeMessages  `yaml:"runtime"`
}

// CmdEntry holds the Short and optional Long description for a cobra command.
type CmdEntry struct {
	Short string `yaml:"short"`
	Long  string `yaml:"long"`
}

// CmdMessages maps to all cobra command Short/Long descriptions.
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
	Ctl            CmdEntry `yaml:"ctl"`
	CtlWorker      CmdEntry `yaml:"ctl_worker"`
	CtlTask        CmdEntry `yaml:"ctl_task"`
	CtlMemory      CmdEntry `yaml:"ctl_memory"`
	CtlSession     CmdEntry `yaml:"ctl_session"`
	CtlSystem      CmdEntry `yaml:"ctl_system"`
	CtlMessage     CmdEntry `yaml:"ctl_message"`
}

// PromptMessages maps to all survey prompt Message fields.
type PromptMessages struct {
	// Engine selection
	EngineSelect       string `yaml:"engine_select"`
	OptionEngineClaude string `yaml:"option_engine_claude"`
	OptionEngineCodex  string `yaml:"option_engine_codex"`
	OptionEnginePi     string `yaml:"option_engine_pi"`
	// Claude setup
	ClaudeNotFound string `yaml:"claude_not_found"`
	ClaudePath     string `yaml:"claude_path"`
	ClaudeTimeout  string `yaml:"claude_timeout"`
	// Codex setup
	CodexPath    string `yaml:"codex_path"`
	CodexTimeout string `yaml:"codex_timeout"`
	// Pi setup
	PiPath    string `yaml:"pi_path"`
	PiTimeout string `yaml:"pi_timeout"`
	// Platform
	PlatformSelect   string `yaml:"platform_select"`
	PlatformFeishu   string `yaml:"platform_feishu"`
	PlatformDingTalk string `yaml:"platform_dingtalk"`
	PlatformWeCom    string `yaml:"platform_wecom"`
	PlatformTelegram string `yaml:"platform_telegram"`
	PlatformWeixin   string `yaml:"platform_weixin"`
	// Survey hint text
	MultiSelectHint      string `yaml:"multiselect_hint"`
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
	JWTRegenConfirm          string `yaml:"jwt_regen_confirm"`
	MCPTokenRegenConfirm     string `yaml:"mcp_token_regen_confirm"`
	// Advanced
	AdvancedConfirm     string `yaml:"advanced_confirm"`
	ServerPort          string `yaml:"server_port"`
	ServerHost          string `yaml:"server_host"`
	DebugMode           string `yaml:"debug_mode"`
	DBPath              string `yaml:"db_path"`
	FeederTimeout       string `yaml:"feeder_timeout"`
	MaxConcurrentBee    string `yaml:"max_concurrent_bee"`
	MessageDebounce     string `yaml:"message_debounce"`
	FFprobePath         string `yaml:"ffprobe_path"`
	FFmpegPath          string `yaml:"ffmpeg_path"`
	// Write
	ConfirmWrite string `yaml:"confirm_write"`
	// Survey options (used in both Options slice and switch cases)
	OptionEnterManually     string `yaml:"option_enter_manually"`
	OptionGenerateRandom    string `yaml:"option_generate_random"`
	OptionEnterPathManually string `yaml:"option_enter_path_manually"`
	OptionDownloadClaude    string `yaml:"option_download_claude"`
	// Telegram help text
	TelegramTokenHelp    string `yaml:"telegram_token_help"`
	TelegramAuthCodeHelp string `yaml:"telegram_auth_code_help"`
	// Generic cancel
	Cancelled string `yaml:"cancelled"`
}

// FlagMessages maps to all cobra flag Usage descriptions.
type FlagMessages struct {
	ConfigPath             string `yaml:"config_path"`
	ServerDaemon           string `yaml:"server_daemon"`
	ConfigOutput           string `yaml:"config_output"`
	BackupPassword         string `yaml:"backup_password"`
	RestorePassword        string `yaml:"restore_password"`
	RestoreForce           string `yaml:"restore_force"`
	UpgradeCheck           string `yaml:"upgrade_check"`
	UpgradeCDNURL          string `yaml:"upgrade_cdn_url"`
	UpgradeCN              string `yaml:"upgrade_cn"`
	ClaudeDownloadForce    string `yaml:"claude_download_force"`
	ClaudeDownloadCDNURL   string `yaml:"claude_download_cdn_url"`
	ClaudeDownloadCN       string `yaml:"claude_download_cn"`
}

// ProviderMessages maps to all user-visible text in claude/provider.go.
type ProviderMessages struct {
	FoundSettings   string `yaml:"found_settings"`
	Select          string `yaml:"select"`
	SelectModel     string `yaml:"select_model"`
	KeyKimiCode     string `yaml:"key_kimicode"`
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

// ValidateMessages maps to interactive input validation error messages.
type ValidateMessages struct {
	PortInteger     string `yaml:"port_integer"`
	PositiveInteger string `yaml:"positive_integer"`
	FileNotFound    string `yaml:"file_not_found"` // contains %s
	PathIsDir       string `yaml:"path_is_dir"`    // contains %s
	FileNotExec     string `yaml:"file_not_exec"`  // contains %s
}

// OutputMessages maps to all command runtime output text.
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

// StopOutput maps to stop command runtime output.
type StopOutput struct {
	NotRunning string `yaml:"not_running"`
	Stale      string `yaml:"stale"`
	ForeignPID string `yaml:"foreign_pid"` // contains %d
	Stopping   string `yaml:"stopping"`    // contains %d
	Stopped    string `yaml:"stopped"`
}

// StatusOutput maps to status command runtime output.
type StatusOutput struct {
	NotRunning      string `yaml:"not_running"`
	NotRunningStale string `yaml:"not_running_stale"`
	Running         string `yaml:"running"` // contains %d, %s
}

// UpgradeOutput maps to upgrade command runtime output.
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
	UsingCDN        string `yaml:"using_cdn"` // contains %s
}

// BackupOutput maps to backup command runtime output.
type BackupOutput struct {
	Created string `yaml:"created"` // contains %s
}

// RestoreOutput maps to restore command runtime output.
type RestoreOutput struct {
	Complete string `yaml:"complete"`
}

// ConfigOutput maps to config and config_claude runtime output.
type ConfigOutput struct {
	FoundExisting           string `yaml:"found_existing"`           // contains %s
	SectionEngine           string `yaml:"section_engine"`
	SectionPlatform         string `yaml:"section_platform"`
	SectionAuth             string `yaml:"section_auth"`
	SectionAdvanced         string `yaml:"section_advanced"`
	SectionWrite            string `yaml:"section_write"`
	OutputFile              string `yaml:"output_file"`              // contains %s
	WriteCancelled          string `yaml:"write_cancelled"`
	Written                 string `yaml:"written"`                  // contains %s
	JWTRegenerated          string `yaml:"jwt_regenerated"`
	JWTGenerated            string `yaml:"jwt_generated"`
	MCPTokenSecretGenerated    string `yaml:"mcp_token_secret_generated"`    // contains %s
	MCPTokenSecretRegenerated  string `yaml:"mcp_token_secret_regenerated"`
	PasswordGenerated       string `yaml:"password_generated"`       // contains %s
	WeixinQRLogin           string `yaml:"weixin_qr_login"`
	FetchingQR              string `yaml:"fetching_qr"`
	QRFailed                string `yaml:"qr_failed"`                // contains %v
	QRFallback              string `yaml:"qr_fallback"`
	WeixinSuccess           string `yaml:"weixin_success"`
	ClaudeFound             string `yaml:"claude_found"`             // contains %s
	ClaudeDownloadFailed    string `yaml:"claude_download_failed"`   // contains %v
	ClaudeManualEntry       string `yaml:"claude_manual_entry"`
	CodexFound              string `yaml:"codex_found"`              // contains %s
	CodexManualEntry        string `yaml:"codex_manual_entry"`
	PiFound                 string `yaml:"pi_found"`                 // contains %s
	PiManualEntry           string `yaml:"pi_manual_entry"`
	SkillInstalled          string `yaml:"skill_installed"`          // contains %s
	SkillUpdated            string `yaml:"skill_updated"`            // contains %s
	SkillUpToDate           string `yaml:"skill_up_to_date"`         // contains %s
	SkillsInstallWarning    string `yaml:"skills_install_warning"`   // contains %v
}

// ClaudeOutput maps to claude subcommand runtime output.
type ClaudeOutput struct {
	AlreadyInstalled string `yaml:"already_installed"` // contains %s
	UseForce         string `yaml:"use_force"`
	InstalledAt      string `yaml:"installed_at"` // contains %s
	UsingCDN         string `yaml:"using_cdn"`    // contains %s
}

// WeixinOutput maps to Weixin QR-code login flow output.
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

// DaemonOutput maps to daemon-related runtime output.
type DaemonOutput struct {
	Started        string `yaml:"started"`         // contains %d
	AlreadyRunning string `yaml:"already_running"` // contains %d
}

// RuntimeMessages holds server-runtime user-visible text (sent to IM users,
// platform placeholders, etc.) that must respond to the language setting.
type RuntimeMessages struct {
	FailureNotifier FailureNotifierMessages `yaml:"failure_notifier"`
	Feishu          FeishuRuntimeMessages   `yaml:"feishu"`
	WeCom           WeComRuntimeMessages    `yaml:"wecom"`
	MCP             MCPRuntimeMessages      `yaml:"mcp"`
}

// FailureNotifierMessages holds text sent to IM users when a task fails.
type FailureNotifierMessages struct {
	TaskFailed  string `yaml:"task_failed"`  // prefix e.g. "❌ Task execution failed"
	ParseFailed string `yaml:"parse_failed"` // worker-line when message parse failed; contains leading \n
	WorkerLine  string `yaml:"worker_line"`  // worker-line template when worker name is known; contains leading \n and %s
	Failed      string `yaml:"failed"`       // error suffix template; contains %s
}

// FeishuRuntimeMessages holds Feishu platform runtime text.
type FeishuRuntimeMessages struct {
	RichTextFallback string `yaml:"rich_text_fallback"` // placeholder when rich-text cannot be parsed
}

// WeComRuntimeMessages holds WeCom platform runtime text.
type WeComRuntimeMessages struct {
	FileSent string `yaml:"file_sent"` // notification after a file is sent to WeCom
}

// MCPRuntimeMessages holds MCP tool runtime text sent back to the bee agent.
type MCPRuntimeMessages struct {
	ClearSessionConfirm      string `yaml:"clear_session_confirm"`       // confirmation prompt; contains %d (worker count)
	ClearSessionTasksConfirm string `yaml:"clear_session_tasks_confirm"` // task confirmation prompt; contains %d (task count)
}
