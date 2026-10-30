package api

import (
	"net/url"
	"strings"
	"time"
)

type LocalizedText map[string]string

type EngineInfo struct {
	ID   string        `json:"id"`
	Name LocalizedText `json:"name"`
}

type Config struct {
	Language       string       `json:"language"`
	EnabledEngines []string     `json:"enabled_engines"`
	Engines        []EngineInfo `json:"engines"`
}

type SetupStatus struct {
	Initialized bool `json:"initialized"`
}

type SetupRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name,omitempty"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type Worker struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Constraints      string            `json:"constraints"`
	WorkDir          string            `json:"work_dir"`
	Engine           string            `json:"engine"`
	EngineArgs       map[string]string `json:"engine_args"`
	Status           string            `json:"status"`
	PermissionScopes string            `json:"permission_scopes"`
	CreatedAt        int64             `json:"created_at"`
	UpdatedAt        int64             `json:"updated_at"`
}

type CreateWorkerRequest struct {
	Name             string            `json:"name"`
	Engine           string            `json:"engine,omitempty"`
	Description      string            `json:"description"`
	Constraints      string            `json:"constraints,omitempty"`
	WorkDir          string            `json:"work_dir,omitempty"`
	PermissionScopes string            `json:"permission_scopes,omitempty"`
	EngineArgs       map[string]string `json:"engine_args,omitempty"`
}

type UpdateWorkerRequest struct {
	Name             *string            `json:"name,omitempty"`
	Description      *string            `json:"description,omitempty"`
	Constraints      *string            `json:"constraints,omitempty"`
	WorkDir          *string            `json:"work_dir,omitempty"`
	PermissionScopes *string            `json:"permission_scopes,omitempty"`
	Engine           *string            `json:"engine,omitempty"`
	EngineArgs       *map[string]string `json:"engine_args,omitempty"`
}

type RandomName struct {
	Name      string `json:"name,omitempty"`
	Exhausted bool   `json:"exhausted,omitempty"`
}

type ChatSender struct {
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
	Sender     ChatSender   `json:"sender"`
	Kind       string       `json:"kind"`
	Content    string       `json:"content"`
	MediaPaths []string     `json:"media_paths,omitempty"`
	TS         int64        `json:"ts"`
	Handoff    *HandoffInfo `json:"handoff,omitempty"`
}

type ConversationActivity struct {
	State       string `json:"state"`
	QueuedAhead int    `json:"queued_ahead,omitempty"`
}

type LastMessage struct {
	Sender          ChatSender `json:"sender"`
	Kind            string     `json:"kind"`
	Preview         string     `json:"preview"`
	MediaPaths      []string   `json:"media_paths,omitempty"`
	HandoffWorkerID string     `json:"handoff_worker_id,omitempty"`
	TS              int64      `json:"ts"`
}

type ConversationSummary struct {
	ID          string               `json:"id"`
	Kind        string               `json:"kind"`
	WorkerID    string               `json:"worker_id,omitempty"`
	LastMessage *LastMessage         `json:"last_message,omitempty"`
	Unread      int                  `json:"unread"`
	Activity    ConversationActivity `json:"activity"`
}

type MessagesPage struct {
	Messages []ChatMessage `json:"messages"`
	HasMore  bool          `json:"has_more"`
}

type SendMessageRequest struct {
	ID         string   `json:"id,omitempty"`
	Content    string   `json:"content"`
	MediaPaths []string `json:"media_paths,omitempty"`
}

type SendMessageResponse struct {
	ID string `json:"id"`
	TS int64  `json:"ts"`
}

type ActiveTask struct {
	ID          string `json:"id"`
	Instruction string `json:"instruction"`
	Type        string `json:"type"`
}

type ChatEvent struct {
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

type EnvConfig struct {
	ID        string  `json:"id"`
	Scope     string  `json:"scope"`
	ScopeID   *string `json:"scope_id"`
	Key       string  `json:"key"`
	Masked    string  `json:"masked"`
	CreatedAt int64   `json:"created_at"`
	UpdatedAt int64   `json:"updated_at"`
}

type CreateEnvRequest struct {
	Scope   string `json:"scope"`
	ScopeID string `json:"scope_id,omitempty"`
	Key     string `json:"key"`
	Value   string `json:"value"`
}

type PluginConfigField struct {
	Key           string        `json:"key"`
	Type          string        `json:"type"`
	Required      bool          `json:"required,omitempty"`
	Default       any           `json:"default,omitempty"`
	RandomDefault bool          `json:"random_default,omitempty"`
	Label         LocalizedText `json:"label,omitempty"`
	Help          LocalizedText `json:"help,omitempty"`
}

type PluginStatus struct {
	ID          string        `json:"id"`
	Package     string        `json:"package,omitempty"`
	Source      string        `json:"source"`
	Version     string        `json:"version,omitempty"`
	Path        string        `json:"path"`
	Name        LocalizedText `json:"name"`
	Description LocalizedText `json:"description,omitempty"`
	Enabled     bool          `json:"enabled"`
	BotName     string        `json:"bot_name"`
	State       string        `json:"state"`
	Error       string        `json:"error,omitempty"`
	HasSetup    bool          `json:"has_setup"`
}

type PluginDetail struct {
	PluginStatus
	Config     map[string]any      `json:"config"`
	Fields     []PluginConfigField `json:"fields"`
	SetupLabel LocalizedText       `json:"setup_label,omitempty"`
}

type CatalogTag struct {
	ID   string        `json:"id"`
	Name LocalizedText `json:"name"`
}

type CatalogPlugin struct {
	Category          string        `json:"category"`
	ID                string        `json:"id"`
	Package           string        `json:"package"`
	Version           string        `json:"version"`
	Official          bool          `json:"official"`
	Name              LocalizedText `json:"name"`
	Description       LocalizedText `json:"description,omitempty"`
	ProtocolVersion   int           `json:"protocol_version"`
	License           string        `json:"license"`
	Repository        string        `json:"repository"`
	Maintainers       []string      `json:"maintainers"`
	Tags              []string      `json:"tags"`
	MinOpenbeeVersion string        `json:"min_openbee_version,omitempty"`
	Deprecated        LocalizedText `json:"deprecated,omitempty"`
	PublishedAt       string        `json:"published_at,omitempty"`
	Integrity         string        `json:"integrity"`
	Tarball           string        `json:"tarball"`
	Installed         bool          `json:"installed"`
	InstalledVersion  string        `json:"installed_version,omitempty"`
	UpdateAvailable   bool          `json:"update_available"`
	Installable       bool          `json:"installable"`
}

type PluginCatalog struct {
	SchemaVersion int             `json:"schema_version"`
	Categories    []CatalogTag    `json:"categories"`
	Tags          []CatalogTag    `json:"tags"`
	Plugins       []CatalogPlugin `json:"plugins"`
}

type PluginOperation struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Spec       string     `json:"spec,omitempty"`
	PluginID   string     `json:"plugin_id,omitempty"`
	Status     string     `json:"status"`
	Error      string     `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type PluginSettingsUpdate struct {
	Config  map[string]any `json:"config"`
	BotName *string        `json:"bot_name,omitempty"`
}

type SetupEvent struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	OK    bool   `json:"ok,omitempty"`
	Error string `json:"error,omitempty"`
}

func escapeSegment(s string) string {
	return strings.ReplaceAll(url.PathEscape(s), ":", "%3A")
}
