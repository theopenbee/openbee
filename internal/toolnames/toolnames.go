// Package toolnames defines MCP tool name constants as the single source of truth.
package toolnames

const (
	ListWorkers     = "list_workers"
	GetWorker       = "get_worker"
	CreateWorker    = "create_worker"
	UpdateWorker    = "update_worker"
	DeleteWorker    = "delete_worker"
	CreateTask      = "create_task"
	ListTasks       = "list_tasks"
	CancelTask      = "cancel_task"
	MarkTaskSuccess = "mark_task_success"
	SendMessage     = "send_message"
	ClearSession    = "clear_session"
)
