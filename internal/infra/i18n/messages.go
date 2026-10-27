package i18n

// Messages holds all user-visible text, keyed by YAML tags.
type Messages struct {
	Runtime RuntimeMessages `yaml:"runtime"`
}

// RuntimeMessages holds server-runtime user-visible text (sent to IM users,
// platform placeholders, etc.) that must respond to the language setting.
type RuntimeMessages struct {
	FailureNotifier FailureNotifierMessages   `yaml:"failure_notifier"`
	RPC             RPCRuntimeMessages        `yaml:"rpc"`
	Department      DepartmentRuntimeMessages `yaml:"department"`
	EngineCommand   EngineCommandMessages     `yaml:"engine_command"`
	ClearCommand    ClearCommandMessages      `yaml:"clear_command"`
	StopCommand     StopCommandMessages       `yaml:"stop_command"`
	StatusCommand   StatusCommandMessages     `yaml:"status_command"`
	ListCommand     ListCommandMessages       `yaml:"list_command"`
}

// EngineCommandMessages holds text sent to IM users by the /engine command handler.
type EngineCommandMessages struct {
	Usage           string `yaml:"usage"`
	SwitchFailed    string `yaml:"switch_failed"`
	DefaultSwitched string `yaml:"default_switched"` // contains %s (engine name)
	WorkerNotFound  string `yaml:"worker_not_found"` // contains %s (worker name)
	WorkerSwitched  string `yaml:"worker_switched"`  // contains %s, %s (worker, engine)
	UnknownEngine   string `yaml:"unknown_engine"`   // contains %s, %s (engine, supported list)
	BusyMessages    string `yaml:"busy_messages"`
	BusyExecutions  string `yaml:"busy_executions"`
	BusyTasks       string `yaml:"busy_tasks"`
}

// ClearCommandMessages holds text sent to IM users by the /clear command handler.
type ClearCommandMessages struct {
	Usage                  string `yaml:"usage"`
	WorkerNotFound         string `yaml:"worker_not_found"` // contains %s (worker name)
	WorkerDuplicate        string `yaml:"worker_duplicate"` // contains %s, %s (name, id list)
	NoContext              string `yaml:"no_context"`
	LookupFailed           string `yaml:"lookup_failed"`
	ConfirmHeader          string `yaml:"confirm_header"`
	ConfirmAgentLine       string `yaml:"confirm_agent_line"`   // contains %s, %s (name, engine)
	ConfirmTasksHeader     string `yaml:"confirm_tasks_header"` // contains %d (task count)
	ConfirmFooter          string `yaml:"confirm_footer"`
	Cleared                string `yaml:"cleared"`                   // contains %s (agent/engine list)
	ClearedWithTasks       string `yaml:"cleared_with_tasks"`        // contains %s, %d (list, cancelled count)
	WorkerCleared          string `yaml:"worker_cleared"`            // contains %s, %s (worker name, engine)
	WorkerClearedWithTasks string `yaml:"worker_cleared_with_tasks"` // contains %s, %s, %d (name, engine, cancelled count)
	WorkerConfirmHeader    string `yaml:"worker_confirm_header"`     // contains %s, %s (name, engine)
	WorkerConfirmFooter    string `yaml:"worker_confirm_footer"`     // contains %s (name)
}

// StopCommandMessages holds text sent to IM users by the /stop command handler.
type StopCommandMessages struct {
	Usage               string `yaml:"usage"`
	Stopped             string `yaml:"stopped"`
	StoppedWithMessages string `yaml:"stopped_with_messages"`
	CancelledMessages   string `yaml:"cancelled_messages"`
	NothingToStop       string `yaml:"nothing_to_stop"`
	LookupFailed        string `yaml:"lookup_failed"`
	WorkerNotFound      string `yaml:"worker_not_found"`       // contains %q (worker name)
	WorkerDuplicate     string `yaml:"worker_duplicate"`       // contains %q, %s (name, id list)
	WorkerStopped       string `yaml:"worker_stopped"`         // contains %s, %d (worker name, cancelled count)
	WorkerNothingToStop string `yaml:"worker_nothing_to_stop"` // contains %s (worker name)
	WorkerStopFailed    string `yaml:"worker_stop_failed"`     // contains %s (worker name)
}

// StatusCommandMessages holds text sent to IM users by the /status command handler.
type StatusCommandMessages struct {
	Usage        string `yaml:"usage"`
	LookupFailed string `yaml:"lookup_failed"`
	Header       string `yaml:"header"`
	SectionBees  string `yaml:"section_bees"`
	SectionTasks string `yaml:"section_tasks"`
	EmptyMarker  string `yaml:"empty_marker"`
	BeeLine      string `yaml:"bee_line"`
	TaskLine     string `yaml:"task_line"`
}

// ListCommandMessages holds text sent to IM users by the /list command handler.
type ListCommandMessages struct {
	Usage         string `yaml:"usage"`
	LookupFailed  string `yaml:"lookup_failed"`
	HeaderAll     string `yaml:"header_all"`
	HeaderSearch  string `yaml:"header_search"`
	EmptyAll      string `yaml:"empty_all"`
	EmptySearch   string `yaml:"empty_search"`
	Line          string `yaml:"line"`
	StatusIdle    string `yaml:"status_idle"`
	StatusWorking string `yaml:"status_working"`
	StatusError   string `yaml:"status_error"`
}

// DepartmentRuntimeMessages holds error messages returned by the department API.
type DepartmentRuntimeMessages struct {
	HasSubDepartments    string `yaml:"has_sub_departments"`
	HasAssociatedWorkers string `yaml:"has_associated_workers"`
	ParentNotFound       string `yaml:"parent_not_found"`
	CircularReference    string `yaml:"circular_reference"`
}

// FailureNotifierMessages holds text sent to IM users when a task fails.
type FailureNotifierMessages struct {
	TaskFailed    string `yaml:"task_failed"`    // prefix e.g. "❌ Task execution failed"
	TaskCancelled string `yaml:"task_cancelled"` // prefix for cancellation notifications
	ParseFailed   string `yaml:"parse_failed"`   // worker-line when message parse failed; contains leading \n
	WorkerLine    string `yaml:"worker_line"`    // worker-line template when worker name is known; contains leading \n and %s
	Failed        string `yaml:"failed"`         // error suffix template; contains %s
}

// RPCRuntimeMessages holds RPC tool runtime text sent back to the bee agent.
type RPCRuntimeMessages struct {
	ClearSessionConfirm      string `yaml:"clear_session_confirm"`       // confirmation prompt; contains %d (worker count)
	ClearSessionTasksConfirm string `yaml:"clear_session_tasks_confirm"` // task confirmation prompt; contains %d (task count)
}
