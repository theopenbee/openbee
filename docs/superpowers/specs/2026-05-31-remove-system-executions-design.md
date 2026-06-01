# Remove `openbee ctl system executions` (full-stack)

## Background

The branch `feat/remove-execution-subcommand` previously removed the
top-level `ctl execution` subcommand, the `list_executions` MCP tool, and
the `read:executions` scope. The `system executions` subcommand (which
listed *bee-owned* execution history — `worker_executions` rows with
`worker_id IS NULL`) was left in place. This spec removes that remaining
entry point and its supporting code paths, keeping the codebase
symmetric: no CLI or MCP surface exposes bee execution history anymore.

## Scope

Remove the `openbee ctl system executions` command end-to-end:

- CLI subcommand registration and flag binding.
- `list_bee_executions` MCP tool (dispatch + tool-name constant).
- `ExecutionStore.ListBeeExecutions` store method.
- Associated unit tests.
- Skill, CLI reference, and i18n documentation that references the
  command or the bee self-reflection workflow it powered.
- Changelog entry (English).

`openbee ctl system overview` is unaffected and continues to expose
`recent_executions` in its response.

## Out of scope

- Database schema. `worker_executions` rows with `worker_id IS NULL`
  remain. No migration is needed.
- Replacement self-reflection workflow. The bee-skill section that
  pointed at `system executions` is simply removed; no substitute
  command or guidance is added.
- Any change to worker-scoped execution tracking.

## Changes

### CLI

`cmd/openbee/ctl_system.go`

- Remove `ctlSystemExecutionsCmd` and the `executionsLimit` package
  variable.
- Drop the `AddCommand(... ctlSystemExecutionsCmd)` argument so only
  `ctlSystemOverviewCmd` is registered.
- File should retain only the `system` parent command and the
  `overview` subcommand.

### RPC tool

`internal/rpc/tools.go`

- Remove `toolListBeeExecutions` (currently at ~line 833).
- Remove the `case utils.ListBeeExecutions:` branch from `CallTool`'s
  dispatch switch.
- If a tool-schema list / registry block in this file enumerates
  `list_bee_executions`, remove that entry as well.

`internal/infra/utils/toolnames.go`

- Remove the `ListBeeExecutions = "list_bee_executions"` constant.

### Store

`internal/infra/store/execution_store.go`

- Remove the `ListBeeExecutions(limit int)` method (~line 472).
- If the surrounding `ExecutionStore` interface (or any mock) declares
  it, remove that declaration as well.

### Tests

- `internal/infra/store/execution_store_test.go`: remove
  `TestExecutionStore_ListBeeExecutions`.
- `internal/rpc/tools_test.go`: remove `TestCallTool_ListBeeExecutions`.
- Verify no other test references the removed symbols.

### Documentation / skill

`internal/infra/skillinstall/skills/openbee-bee/references/system-status.md`

- Remove the `openbee ctl system executions [--limit <n>]` line from
  the command block.
- Remove the `When doing self-reflection, use system executions...`
  bullet from the **Usage Scenarios** list (per chosen option (b):
  delete the self-reflection guidance entirely; do not substitute
  `system overview` as a replacement).

`internal/infra/skillinstall/skills/openbee-bee/references/cli-reference.md`

- Remove the `openbee ctl system executions [--limit <count>]` entry
  and any prose referencing it.

### i18n

`internal/infra/i18n/locales/en.yaml` and `zh.yaml`

- `ctl_system.short`: change from "View system status and executions"
  (and its zh equivalent) to a phrase that mentions only system status
  / overview. English wording: `"View system status overview"`.

### Changelog

- Append an English entry following the format used in commit
  `db11a42` ("docs: update changelog for execution subcommand
  removal"). One line summarising the removal of the
  `system executions` command, the `list_bee_executions` MCP tool, and
  the bee self-reflection skill guidance.

## Compatibility

No backwards-compatibility shim. Consistent with prior removals on this
branch, the command and tool are deleted outright.

## Verification

1. `go build ./...` succeeds.
2. `go test ./...` succeeds.
3. `rg 'list_bee_executions|ListBeeExecutions'` returns no matches.
4. `rg 'system executions'` returns no matches under `internal/` or
   `cmd/` (changelog/spec/plan files may still reference it
   historically, which is expected).
5. `openbee ctl system --help` lists only `overview`.
6. `openbee ctl system overview` works unchanged.

## Risks

- **Loss of bee self-reflection history retrieval.** Accepted by the
  user. The previously documented workflow is removed without a
  replacement; bees no longer have a first-class command to enumerate
  their own historical executions. `system overview`'s
  `recent_executions` field continues to surface the most recent
  entries as a side effect, but the skill no longer instructs bees to
  use it for reflection.
- **Stale skill installations.** Already-installed `openbee-bee` skill
  copies on user machines retain the old guidance until reinstalled.
  Out of scope to migrate.
