package local

const (
	EventMessage      = "message"
	EventWorkerStatus = "worker_status"
	EventTask         = "task"
)

const SenderBee = "bee"

const (
	KindMessage = "message"
	KindHandoff = "handoff"
)

const (
	ActivityIdle    = "idle"
	ActivityQueued  = "queued"
	ActivityRunning = "running"
)

type Sender struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
}

type HandoffInfo struct {
	WorkerID       string `json:"worker_id"`
	TaskID         string `json:"task_id"`
	TaskType       string `json:"task_type"`
	CronExpr       string `json:"cron_expr,omitempty"`
	ScheduledAt    *int64 `json:"scheduled_at,omitempty"`
	ConversationID string `json:"conversation_id"`
}

type ChatMessage struct {
	ID         string       `json:"id"`
	Sender     Sender       `json:"sender"`
	Kind       string       `json:"kind"`
	Content    string       `json:"content"`
	MediaPaths []string     `json:"media_paths,omitempty"`
	TS         int64        `json:"ts"`
	Handoff    *HandoffInfo `json:"handoff,omitempty"`
}

type Event struct {
	Type           string       `json:"type"`
	Seq            uint64       `json:"seq"`
	ConversationID string       `json:"conversation_id,omitempty"`
	Message        *ChatMessage `json:"message,omitempty"`
	WorkerID       string       `json:"worker_id,omitempty"`
	Status         string       `json:"status,omitempty"`
	TaskID         string       `json:"task_id,omitempty"`
	State          string       `json:"state,omitempty"`
	QueuedAhead    int          `json:"queued_ahead,omitempty"`
}

type Activity struct {
	State       string `json:"state"`
	QueuedAhead int    `json:"queued_ahead,omitempty"`
}
