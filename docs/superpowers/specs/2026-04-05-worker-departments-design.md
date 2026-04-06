# Worker Departments Design

**Date:** 2026-04-05
**Status:** Approved

## Summary

Add department management for workers. Departments support multi-level hierarchy, each department has at most one parent, and a worker can belong to multiple departments. This feature is for management only: display, organization, filtering, and statistics. It does not change task assignment, permissions, or runtime behavior.

## Goals

- Add a department tree with unlimited depth
- Allow a worker to belong to multiple departments
- Support worker filtering by a department subtree
- Let users manage department membership when creating or editing a worker
- Keep the feature compatible with current API, CLI, MCP, and web worker flows

## Non-Goals

- Department-based task routing or automatic worker selection
- Department-based permissions or visibility control
- Primary department semantics
- Multi-parent department graphs

## Requirements

| Dimension | Decision |
|-----------|----------|
| Department structure | Strict tree |
| Parent relation | Each department has at most one parent |
| Worker membership | Many-to-many |
| Primary department | None |
| Name uniqueness | Unique among siblings only |
| Department name validation | Trimmed, non-empty, must not contain `/` |
| Worker filtering | Selecting a parent department includes all descendants |
| Department deletion | Reject when child departments or worker memberships still exist |
| Scope | Management only |

## Architecture

### Data Model

Use a mixed model:

- `parent_id` is the source of truth for hierarchy
- `path` and `depth` are derived fields stored for query speed and UI convenience

This keeps writes explicit and safe while allowing fast subtree filtering without requiring recursive queries on every list request.

### New Table: `bee_departments`

```sql
CREATE TABLE bee_departments (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    parent_id   TEXT REFERENCES bee_departments(id),
    path        TEXT NOT NULL,
    depth       INTEGER NOT NULL,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
)
```

Indexes and constraints:

- Unique sibling name:

```sql
CREATE UNIQUE INDEX idx_departments_parent_name
ON bee_departments(COALESCE(parent_id, ''), name)
```

- Path prefix lookup:

```sql
CREATE INDEX idx_departments_path ON bee_departments(path)
```

- Parent lookup:

```sql
CREATE INDEX idx_departments_parent_id ON bee_departments(parent_id)
```

Path format:

- Root: `/engineering`
- Child: `/engineering/backend`
- Grandchild: `/engineering/backend/platform`

`path` is built from department names, not IDs, because it is also returned to the UI for readable display and breadcrumb-like context. A rename or move updates `path` for the department and all descendants in one transaction.

### New Table: `bee_worker_departments`

```sql
CREATE TABLE bee_worker_departments (
    worker_id      TEXT NOT NULL REFERENCES bee_workers(id) ON DELETE CASCADE,
    department_id  TEXT NOT NULL REFERENCES bee_departments(id) ON DELETE CASCADE,
    created_at     INTEGER NOT NULL,
    PRIMARY KEY (worker_id, department_id)
)
```

Indexes:

```sql
CREATE INDEX idx_worker_departments_department_id
ON bee_worker_departments(department_id)
```

### Worker Response Shape

Extend the worker DTO to include department membership:

```json
{
  "id": "worker-id",
  "name": "Backend Agent",
  "departments": [
    {
      "id": "dept-backend",
      "name": "Backend",
      "parent_id": "dept-engineering",
      "path": "/engineering/backend",
      "depth": 1
    }
  ]
}
```

Worker detail returns the full department array. Worker list also returns department summaries so the web app does not need an extra fetch per row.

## Storage and Domain Rules

### Create Department

1. Validate that `parent_id` exists when provided
2. Validate that `name` is trimmed, non-empty, and does not contain `/`
3. Validate sibling name uniqueness under the chosen parent
4. Compute `path` and `depth`
5. Insert department row

### Update Department

Allowed mutable fields:

- `name`
- `parent_id`

Rules:

- Apply the same name validation rules as create
- Reject if `parent_id` points to the department itself
- Reject if `parent_id` points to any descendant
- Reject if the target parent already has a child with the same name
- Recompute `path` and `depth` for the department and all descendants in one transaction

### Delete Department

Reject deletion when either condition is true:

- The department still has direct child departments
- The department still has worker memberships

This intentionally favors data safety over convenience. Deletion is only allowed after the tree and memberships are manually cleaned up.

### Create or Update Worker

Accept `department_ids` on worker create and update.

Rules:

- Every department ID must exist
- Duplicate department IDs are ignored by primary key semantics
- Update uses replace semantics for department membership:
  - omitted field: keep existing memberships unchanged
  - present field: replace all memberships with the provided set
  - present field with `[]`: clear all memberships

All worker updates and membership writes occur in one transaction.

## API Design

### New Department Endpoints

| Method | Path | Behavior |
|--------|------|----------|
| `POST` | `/departments` | Create a department |
| `GET` | `/departments` | Return the department tree |
| `GET` | `/departments/:id` | Return one department with summary counts |
| `PUT` | `/departments/:id` | Rename or move a department |
| `DELETE` | `/departments/:id` | Delete an empty leaf department |

### `GET /departments` Response

Return a tree shape:

```json
[
  {
    "id": "dept-engineering",
    "name": "Engineering",
    "parent_id": null,
    "path": "/engineering",
    "depth": 0,
    "worker_count_direct": 1,
    "worker_count_recursive": 3,
    "children": []
  }
]
```

`worker_count_direct` counts distinct workers directly attached to the department. `worker_count_recursive` counts distinct workers in the subtree.

### Worker Endpoint Extensions

#### `POST /workers`

Add optional `department_ids: string[]`.

#### `PUT /workers/:id`

Add optional `department_ids: string[]`.

#### `GET /workers`

Add optional query parameter:

- `department_id`

When `department_id` is present, the API returns workers linked to that department or any descendant department.

#### `GET /workers/:id`

Return the worker plus its departments.

## Query Strategy

Subtree filtering uses stored `path`:

1. Load the selected department path
2. Find departments where:
   - `path = selected_path`
   - or `path LIKE selected_path || '/%'`
3. Join `bee_worker_departments`
4. Return distinct workers

`parent_id` remains the authoritative hierarchy field. `path` is a denormalized index-friendly helper.

## CLI and MCP

### CLI

Add:

- `openbee ctl department list`
- `openbee ctl department get <id>`
- `openbee ctl department create`
- `openbee ctl department update <id>`
- `openbee ctl department delete <id>`

Extend worker commands:

- `openbee ctl worker create --department-id <id> [--department-id <id> ...]`
- `openbee ctl worker update <id> --department-id <id> [--department-id <id> ...]`
- `openbee ctl worker update <id> --clear-departments`

Repeated `--department-id` flags map to `department_ids`. `--clear-departments` explicitly replaces membership with an empty set.

### MCP

Add tools:

- `list_departments`
- `get_department`
- `create_department`
- `update_department`
- `delete_department`

Extend worker tools:

- `list_workers` accepts optional `department_id`
- `create_worker` accepts optional `department_ids`
- `update_worker` accepts optional `department_ids`
- `get_worker` returns `departments`

## Web UX

### Workers Page

Add a department filter above the table.

Behavior:

- Default value: all workers
- Selecting a department filters by the entire subtree
- The filter control reads from the department tree API
- The table shows worker department chips or a short summary such as the first two names plus overflow count

### Create Worker

Extend the existing create worker sheet with a department multi-select tree.

Behavior:

- Optional field
- Multiple departments can be selected
- Selected items are shown clearly before submit

### Worker Detail

Add a department section to the detail page with inline editing.

Behavior:

- Show current department memberships as chips or list items
- Support editing membership with the same tree selector used in create flow
- Keep description and memory editing behavior unchanged

### Department Management Entry

Do not add a dedicated top-level route in this scope.

Instead, add a lightweight management entry near the worker page filter. It can open a drawer or dialog for:

- create department
- rename department
- move department
- delete department

This keeps the feature inside the existing worker management surface without introducing a separate navigation concept.

## Validation and Errors

| Case | Status | Error |
|------|--------|-------|
| Parent department does not exist | `400` | invalid parent |
| Department name empty after trim or contains `/` | `400` | invalid department name |
| Sibling name conflict | `400` | duplicate sibling name |
| Department moved under itself or descendant | `400` | invalid hierarchy move |
| Delete department with child departments | `409` | department not empty |
| Delete department with worker memberships | `409` | department not empty |
| Worker update references unknown department | `400` | invalid department_ids |
| Department not found | `404` | department not found |
| Worker not found | `404` | worker not found |

## Migration Plan

Add new migrations after the current latest schema version:

1. Create `bee_departments`
2. Create indexes for parent lookup, path lookup, and sibling uniqueness
3. Create `bee_worker_departments`
4. Create index on `department_id`

No existing worker, task, execution, or session data is modified.

## Files Expected to Change

| Area | Likely Files |
|------|--------------|
| DB schema | `internal/infra/store/db.go` |
| Models | `internal/infra/model/worker.go`, new department model file |
| Stores | new department store, worker store membership loading and filtering |
| API | `internal/api/router.go`, `internal/api/worker_handler.go`, new department handler |
| Worker domain | `internal/domain/worker/manager.go` for create/update flow |
| MCP | `internal/ai/mcp/tools.go`, `internal/infra/utils/toolnames.go` |
| CLI | `cmd/openbee/ctl_worker.go`, new `cmd/openbee/ctl_department.go` |
| Web types/api/hooks | `web/src/lib/types.ts`, `web/src/lib/api.ts`, `web/src/hooks/use-workers.ts`, new department hooks |
| Web pages | `web/src/pages/workers.tsx`, `web/src/pages/worker-detail.tsx` |
| i18n | locale files for worker and department labels |

## Testing

### Store Tests

- Create root and nested departments
- Enforce sibling uniqueness
- Reject self-parent and descendant-parent moves
- Recompute descendant `path` and `depth` after rename or move
- Reject delete when children exist
- Reject delete when worker memberships exist
- Support worker multi-membership
- Filter workers by department subtree correctly

### API Tests

- Department CRUD success paths
- Worker create/update with `department_ids`
- Worker list filtered by `department_id`
- Correct HTTP status codes for validation errors

### MCP and CLI Tests

- New department tool coverage
- Worker tool compatibility with department fields
- CLI repeated `--department-id` handling

### Web Tests

- Worker create form submits department selections
- Worker detail edits and reloads memberships
- Worker list department subtree filter updates results
- Empty states for no departments and no matches

## Implementation Notes

- Keep the hierarchy logic centralized in the department store or a small dedicated service instead of spreading `path` updates across handlers
- Keep worker membership loading batched for list endpoints to avoid N+1 queries
- Reuse the same department selector component for worker create and worker detail edit
- Escape `LIKE` wildcard characters when using `path` prefix matching
