export type WorkerStatus = "idle" | "working" | "error"

export interface Worker {
  id: string
  name: string
  description: string
  constraints: string
  work_dir: string
  engine: Engine
  engine_args?: Record<string, string>
  permission_scopes?: string
  status: WorkerStatus
  created_at: number
  updated_at: number
}

export type ConversationId = "bee" | `worker:${string}`

export interface ChatSender {
  type: "user" | "bee" | "worker" | "system"
  id?: string
}

export interface HandoffInfo {
  worker_id: string
  task_id: string
  task_type: "immediate" | "countdown" | "scheduled"
  cron_expr?: string
  scheduled_at?: number
  conversation_id: ConversationId
}

export type ChatMessageKind = "message" | "handoff" | "reset"

export interface ChatMessage {
  id: string
  sender: ChatSender
  kind: ChatMessageKind
  handoff?: HandoffInfo
  content: string
  media_paths?: string[]
  ts: number
  pending?: boolean
}

export interface ConversationActivity {
  state: "idle" | "queued" | "running"
  queued_ahead?: number
}

export interface ConversationSummary {
  id: ConversationId
  kind: "bee" | "worker"
  worker_id?: string
  last_message?: {
    sender: ChatSender
    kind: ChatMessageKind
    preview: string
    media_paths?: string[]
    handoff_worker_id?: string
    ts: number
  }
  unread: number
  activity: ConversationActivity
}

export interface ActiveTask {
  id: string
  instruction: string
  type: string
}

export interface ConversationMessagesResponse {
  messages: ChatMessage[]
  has_more: boolean
}

export type EnvScope = "global" | "bee" | "worker"

// Matches the backend constant defaultBeeID in internal/domain/bee/bee_process.go.
export const DEFAULT_BEE_ID = "default"

// Mirror model.SystemConfigKey* in Go — keep in sync.
export const SYSTEM_CONFIG_KEY_DEFAULT_ENGINE = "default_engine"
export const SYSTEM_CONFIG_KEY_ENGINE_ARGS_GLOBAL = "engine_args_global"
export const SYSTEM_CONFIG_KEY_ENGINE_ARGS_BEE = "engine_args_bee"

export type Engine = string

export interface EngineChoice {
  id: Engine
  name: string
}

export function newWorkerEngine(systemDefault: string | undefined, enabled: readonly Engine[]): Engine {
  return enabled.find((engine) => engine === systemDefault) ?? enabled[0] ?? ""
}

export function availableEngine(engine: Engine, enabled: readonly Engine[]): Engine {
  return enabled.includes(engine) ? engine : (enabled[0] ?? engine)
}

export interface EnvConfig {
  id: string
  scope: EnvScope
  scope_id: string | null
  key: string
  masked: string
  created_at: number
  updated_at: number
}

export interface EngineInfo {
  id: Engine
  name: LocalizedText
}

export interface AppConfig {
  language: string
  enabled_engines: Engine[]
  engines: EngineInfo[]
}

export interface SetupStatus {
  initialized: boolean
}

export interface TokenResponse {
  access_token: string
  refresh_token: string
  expires_in: number
}

export type LocalizedText = Record<string, string>

export type PluginType = "channel" | "engine"

export type PluginState = "not_configured" | "disabled" | "starting" | "running" | "ready" | "retrying" | "failed"

export type PluginFieldType = "string" | "secret" | "bool" | "int" | "list"

export interface PluginConfigField {
  key: string
  type: PluginFieldType
  required?: boolean
  default?: unknown
  random_default?: boolean
  label?: LocalizedText
  help?: LocalizedText
}

export interface PluginStatus {
  id: string
  type: PluginType
  package?: string
  source: string
  version?: string
  path: string
  name: LocalizedText
  description?: LocalizedText
  enabled: boolean
  bot_name: string
  state: PluginState
  error?: string
  has_setup: boolean
}

export interface PluginDetail extends PluginStatus {
  config: Record<string, unknown>
  fields: PluginConfigField[]
  setup_label?: LocalizedText
}

export interface CatalogVocab {
  id: string
  name: LocalizedText
}

export interface CatalogPlugin {
  category: string
  id: string
  package: string
  version: string
  official: boolean
  name: LocalizedText
  description?: LocalizedText
  protocol_version: number
  license: string
  repository: string
  maintainers: string[]
  tags: string[]
  min_openbee_version?: string
  deprecated?: LocalizedText
  published_at?: string
  installed: boolean
  installed_version?: string
  update_available: boolean
  installable: boolean
}

export interface PluginCatalog {
  schema_version: number
  categories: CatalogVocab[]
  tags: CatalogVocab[]
  plugins: CatalogPlugin[]
}

export type PluginOperationStatus = "queued" | "running" | "succeeded" | "failed"

export interface PluginOperation {
  id: string
  kind: "install" | "uninstall"
  spec?: string
  plugin_id?: string
  status: PluginOperationStatus
  error?: string
  created_at: string
  finished_at?: string
}

export interface PluginSetupEvent {
  type: "line" | "done"
  text?: string
  ok?: boolean
  error?: string
}

export interface PluginSettingsUpdate {
  bot_name?: string
  config: Record<string, unknown>
}
