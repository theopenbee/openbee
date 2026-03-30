// Package toolnames defines MCP tool name constants as the single source of truth.
package toolnames

const (
	ListWorkers         = "list_workers"
	GetWorker           = "get_worker"
	CreateWorker        = "create_worker"
	UpdateWorker        = "update_worker"
	DeleteWorker        = "delete_worker"
	CreateTask          = "create_task"
	ListTasks           = "list_tasks"
	CancelTask          = "cancel_task"
	SendMessage         = "send_message"
	ClearSession        = "clear_session"
	GetWorkerStatus     = "get_worker_status"
	GetSystemOverview   = "get_system_overview"
	ListBeeExecutions   = "list_bee_executions"
	SaveMemory          = "save_memory"
	GetMemory           = "get_memory"
	DeleteMemory        = "delete_memory"
	ListSessionContexts = "list_session_contexts"
	ClearWorkerSession  = "clear_worker_session"
)
