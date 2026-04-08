# Department Management CLI Design

**Date:** 2026-04-08  
**Status:** Approved

## Overview

Add `openbee ctl department` subcommands for full department CRUD, and extend `openbee ctl worker` with department-related parameters. Departments already exist in the data model and REST API; this spec covers the CLI and MCP tool layer that is missing.

## Design Decisions

| Question | Decision |
|----------|----------|
| Department hierarchy | Supported; `department list` returns tree structure |
| Worker-department association in CLI | Via flags on existing `worker` commands (no separate association subcommands) |
| Department identifier in CLI | Auto-detect: try ID first, fall back to name match |
| Worker list filtering scope | Recursive by default (includes child departments); disable with `--no-recursive` |
| Multiple departments syntax | Comma-separated string: `--department 研发部,前端组` |
| Architecture | New MCP tools for department CRUD + extend existing worker MCP tools |

## Architecture

All `ctl` commands follow the existing pattern:

```
CLI → ctlRun(toolName, args) → HTTP POST /mcp/bee/call → MCP Tool → Store
```

No new calling patterns are introduced. Name-to-ID resolution happens in the MCP tool layer (not the store layer), keeping store interfaces clean.

## New CLI Commands: `openbee ctl department`

```bash
# List all departments (tree structure)
openbee ctl department list

# Get a single department by ID or name
openbee ctl department get <id|name>

# Create a department
openbee ctl department create --name <name> [--parent <parent-id|name>] [--sort-order <n>]

# Update a department (patch semantics — only changed fields are sent)
openbee ctl department update <id|name> [--name <name>] [--parent <parent-id|name>] [--sort-order <n>]

# Delete a department (fails if it has children or associated workers)
openbee ctl department delete <id|name>
```

## Changes to `openbee ctl worker`

### `worker list` — new flags

```bash
openbee ctl worker list [--department <id|name>] [--no-recursive]
```

- `--department`: filter workers by department (ID or name auto-detected)
- `--no-recursive`: only meaningful when `--department` is also provided; when set, only returns workers directly in the specified department, not in child departments; default behavior is recursive

### `worker create` — new flag

```bash
openbee ctl worker create --name <name> [--department <dept1,dept2,...>] [...]
```

- `--department`: comma-separated list of department IDs or names; worker is associated with these departments at creation time

### `worker update` — new flag

```bash
openbee ctl worker update <id> [--department <dept1,dept2,...>] [...]
```

- `--department`: comma-separated list; **replaces all** existing department associations for the worker. Pass an empty string (`--department ""`) to clear all associations. If `--department` is not provided at all, existing associations are left unchanged (patch semantics via `cmd.Flags().Changed()`).

### `worker get` / `worker list` output

Both commands now include a `departments` field in their output, showing the departments each worker belongs to.

## New MCP Tools

### `create_department`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Department name |
| `parent_id` | string | no | Parent department (ID or name) |
| `sort_order` | int | no | Display sort order |

### `list_departments`

No parameters. Returns the full department tree.

### `get_department`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Department ID or name |

### `update_department`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Department ID or name |
| `name` | string | no | New name |
| `parent_id` | string | no | New parent (ID or name) |
| `sort_order` | int | no | New sort order |

Patch semantics: only fields explicitly provided are updated. Note: moving a department to root level (removing its parent) is not supported in this version — `--parent` can only assign a new parent, not clear one.

### `delete_department`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Department ID or name |

Returns an error if the department has child departments or associated workers.

## Extensions to Existing Worker MCP Tools

### `create_worker` — new parameter

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `department_ids` | string | no | Comma-separated department IDs or names |

### `update_worker` — new parameter

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `department_ids` | string | no | Comma-separated department IDs or names; replaces all existing associations. Empty string clears all. |

### `list_workers` — new parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `department_id` | string | no | Filter by department (ID or name) |
| `recursive` | bool | no | Include child department workers (default: true) |

## Implementation Details

### Name-to-ID Resolution

Implemented in the MCP tool handler, not in the store:

1. If the value looks like a UUID, query by ID directly.
2. If no match found by ID (or value is not a UUID), query all departments and match by name.
3. Return an error if no match found.

This keeps `DepartmentStore` and `WorkerStore` interfaces ID-only.

### Recursive Worker Query for `list_workers`

When `department_id` is provided and `recursive=true`:

1. Fetch the full department tree from `DepartmentStore.BuildTree()`.
2. DFS from the target department node to collect all descendant department IDs.
3. Query `bee_worker_departments` for all workers in this ID set.
4. Batch-fetch the worker records.

When `recursive=false`, query only `bee_worker_departments` for the single department ID.

### Error Handling

- Department name not found: return descriptive error `department "X" not found`
- Ambiguous name (multiple departments with same name under different parents): return all matches with their full ancestor paths (e.g., `研发部 > 前端组`) and instruct the user to use an ID instead
- Delete with children: return error `cannot delete department with child departments`
- Delete with workers: return error `cannot delete department with associated workers`
- Circular parent reference: detected in store layer (already implemented), return error

## Files to Create or Modify

| File | Change |
|------|--------|
| `cmd/openbee/ctl_department.go` | New file — department CLI commands |
| `cmd/openbee/ctl_worker.go` | Add `--department` flag to `list`, `create`, `update` |
| `internal/ai/mcp/tools.go` | Add 5 department MCP tools; extend 3 worker tools |
| `internal/infra/utils/toolnames.go` | Add 5 new tool name constants |
