package rpc

import (
	"context"
	"strings"

	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

type apiMethod struct {
	name string
	decl string
}

const apiTypes = `type Page<T> = { items: T[]; total: number; page: number; page_size: number };
type WorkerBrief = { id: string; name: string; description: string; engine: string };
type Worker = WorkerBrief & { constraints: string; work_dir: string; engine_args: string; status: "idle" | "working" | "error"; permission_scopes: string; created_at: number; updated_at: number };
type Execution = { id: string; task_id?: string; worker_id?: string; session_id: string; engine?: string; trigger_input?: string; status: "pending" | "running" | "completed" | "failed"; result?: string; started_at?: number; completed_at?: number };
type Task = { ID: string; MessageID: string; SessionKey: string; Platform: string; WorkerID: string; Instruction: string; Type: "immediate" | "countdown" | "scheduled"; Status: "pending" | "running" | "completed" | "failed" | "cancelled"; ScheduledAt: number | null; CronExpr: string; NextRunAt: number | null; CreatedAt: number; UpdatedAt: number; executions: Execution[] };
type Message = { id: string; session_key: string; platform: string; content: string; status: "received" | "feeding" | "bee_processed" | "merged" | "failed"; received_at: number };
type OutboundMessage = { id: string; session_key: string; platform: string; content: string; status: "sent" | "failed"; source_type: "bee" | "worker" | "system"; source_id: string; inbound_msg_id: string; error: string; sent_at: number; kind: string };
type Constraint = { key: string; value: string; updated_at: number };
type ActiveTasksConfirmation = { requires_confirmation: true; reason: "active_tasks"; running_tasks: { task_id: string; instruction: string; status: string }[]; message: string };`

var apiMethods = []apiMethod{
	{utils.SendMessage, `/** Send text and/or one file to the user. Pass message_id to reply to that message, or session_key to post a new message to the conversation. */
send_message(args: { message_id?: string; session_key?: string; content?: string; media_path?: string }): Promise<{ status: "sent" }>;`},
	{utils.ListWorkers, `/** List workers. name matches part of the name, case-insensitive; id matches exactly. page_size defaults to 50, max 200. */
list_workers(args?: { name?: string; id?: string; page?: number; page_size?: number }): Promise<Page<WorkerBrief>>;`},
	{utils.GetWorker, `get_worker(args: { worker_id: string }): Promise<Worker>;`},
	{utils.GetWorkerStatus, `get_worker_status(args: { worker_id: string }): Promise<{ worker_id: string; name: string; status: string; pending_tasks_count: number; current_execution: { id: string; task_id: string; instruction: string; started_at: number } | null }>;`},
	{utils.CreateWorker, `/** engine: one of the enabled engines, empty for the server default. permission_scopes: comma-separated, from read:workers, read:tasks, read:messages. engine_args: extra CLI flags per engine, e.g. { claude: "--model claude-sonnet-4-6" }. */
create_worker(args: { name: string; description?: string; constraints?: string; work_dir?: string; engine?: string; permission_scopes?: string; engine_args?: Record<string, string> }): Promise<Worker>;`},
	{utils.UpdateWorker, `/** Only the fields you pass change. permission_scopes replaces all scopes ("" clears them). In engine_args an empty string removes that engine's flags and {} clears all. */
update_worker(args: { worker_id: string; name?: string; description?: string; constraints?: string; work_dir?: string; engine?: string; permission_scopes?: string; engine_args?: Record<string, string> }): Promise<Worker>;`},
	{utils.DeleteWorker, `delete_worker(args: { worker_id: string; delete_work_dir?: boolean }): Promise<{ status: "deleted" }>;`},
	{utils.ListTasks, `/** Needs at least one of task_id, message_id, session_key, worker_id; task_id cannot be combined with the others. page_size defaults to 50, max 100. Each task carries its newest executions: 10 by default, execution_limit sets another count, and execution_limit 0 with task_id returns all of them. */
list_tasks(args: { task_id?: string; message_id?: string; session_key?: string; worker_id?: string; status?: string; type?: "immediate" | "countdown" | "scheduled"; page?: number; page_size?: number; execution_limit?: number }): Promise<Page<Task>>;`},
	{utils.CreateTask, `/** Assign a task to a worker. message_id is the user message the task answers. countdown needs scheduled_at (Unix ms, at least 5s ahead); scheduled needs cron_expr (5-field cron). */
create_task(args: { message_id: string; worker_id: string; instruction: string; type: "immediate" | "countdown" | "scheduled"; scheduled_at?: number; cron_expr?: string }): Promise<{ task_id: string; status: string; reason?: string; conversation_id?: string }>;`},
	{utils.CancelTask, `cancel_task(args: { task_id: string }): Promise<{ task_id: string; status: "cancelled" }>;`},
	{utils.ListMessages, `/** Inbound user messages. Times are Unix ms. page_size defaults to 50, max 100. */
list_messages(args?: { session_key?: string; platform?: string; status?: Message["status"]; received_at_from?: number; received_at_to?: number; page?: number; page_size?: number }): Promise<Page<Message>>;`},
	{utils.ListOutboundMessages, `/** Messages openbee sent. Times are Unix ms. page_size defaults to 50, max 100. */
list_outbound_messages(args?: { session_key?: string; platform?: string; status?: "sent" | "failed"; source_type?: "bee" | "worker" | "system"; source_id?: string; sent_at_from?: number; sent_at_to?: number; page?: number; page_size?: number }): Promise<Page<OutboundMessage>>;`},
	{utils.ListSessionContexts, `/** Agents (bee and workers) holding context in a session. */
list_session_contexts(args: { session_key: string }): Promise<{ AgentID: string; AgentType: "bee" | "worker"; Engine: string; Name: string; UpdatedAt: number }[] | null>;`},
	{utils.ClearSession, `/** Clear every agent's context in a session. Without force it may return a confirmation request instead of clearing: reason "multiple_workers" or "active_tasks". */
clear_session(args: { session_key: string; force?: boolean }): Promise<{ cleared: true; cancelled_tasks: number } | { cleared: false; reason: "no_context" } | { requires_confirmation: true; reason: "multiple_workers"; worker_count: number; linked_workers: { worker_id: string; name: string }[]; message: string } | ActiveTasksConfirmation>;`},
	{utils.ClearWorkerSession, `/** Clear one worker's context in a session. Without force, running tasks make it return a confirmation request instead. */
clear_worker_session(args: { session_key: string; worker_id: string; force?: boolean }): Promise<{ cleared: true; cancelled_tasks: number; worker_id: string; worker_name: string } | (ActiveTasksConfirmation & { worker_id: string; worker_name: string })>;`},
	{utils.GetConstraint, `/** scope is "global" or a session_key. With key returns one constraint or { status: "not_found" }; without key lists up to 50. */
get_constraint(args: { scope: string; key?: string }): Promise<Constraint | { status: "not_found" } | Constraint[] | null>;`},
	{utils.SaveConstraint, `save_constraint(args: { scope: string; key: string; value: string }): Promise<{ status: "saved" }>;`},
	{utils.DeleteConstraint, `delete_constraint(args: { scope: string; key: string }): Promise<{ status: "deleted" }>;`},
	{utils.GetSystemOverview, `/** Worker counts by status and task counts by status. */
get_system_overview(): Promise<{ workers: { total: number; idle: number; working: number; error: number }; tasks: Record<string, number> }>;`},
}

type caller struct {
	workerID string
	scopes   []string
}

func (c caller) isWorker() bool {
	return c.workerID != ""
}

func (c caller) context(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, CtxWorkerIDKey, c.workerID)
	return context.WithValue(ctx, CtxScopesKey, c.scopes)
}

func (c caller) methods() []apiMethod {
	if !c.isWorker() {
		return apiMethods
	}
	out := make([]apiMethod, 0, len(apiMethods))
	for _, m := range apiMethods {
		if _, ok := auth.WorkerToolScope(m.name); ok {
			out = append(out, m)
		}
	}
	return out
}

func (c caller) methodNames() []string {
	methods := c.methods()
	names := make([]string, len(methods))
	for i, m := range methods {
		names[i] = m.name
	}
	return names
}

func (c caller) declaration() string {
	var b strings.Builder
	b.WriteString(apiTypes)
	b.WriteString("\n\ndeclare const openbee: {\n")
	for _, m := range c.methods() {
		for _, line := range strings.Split(m.decl, "\n") {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("};")
	return b.String()
}
