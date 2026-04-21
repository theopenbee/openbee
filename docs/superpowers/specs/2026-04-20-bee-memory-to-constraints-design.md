# Design: Rename bee_memories to bee_constraints

**Date:** 2026-04-20
**Status:** Approved

## Problem

The `bee_memories` system uses MCP tool names (`save_memory`, `get_memory`, `delete_memory`) with "memory" semantics. When a Bee (Claude agent) executes, Claude's built-in agent memory feature can intercept or interfere with these memory operations, causing unexpected behavior.

The Worker-side equivalent was already renamed from `memory` to `constraints` in commit `3fe1dba` for the same reason. This design applies the same fix to the Bee-side memory system.

## Decision

Pure rename across all layers. No behavioral changes, no semantic changes to the data model, no backward compatibility aliases.

- **Rename type:** Full-stack rename (all layers)
- **Backward compatibility:** Hard cut — old names deprecated, no aliases
- **Data model:** key-value structure (scope + key + value) unchanged

## Architecture

### Database Layer

New migration added to `internal/infra/store/db.go`:

1. Create `bee_constraints` table with identical schema to `bee_memories`
2. Copy all data from `bee_memories` to `bee_constraints`
3. Drop `bee_memories`

SQLite does not support `RENAME TABLE`, so the three-step approach is required.

Schema (unchanged):
```sql
CREATE TABLE bee_constraints (
    id         TEXT PRIMARY KEY,
    scope      TEXT NOT NULL,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(scope, key)
)
```

### Store Layer

File: `internal/infra/store/memory_store.go` → `constraint_store.go`

| Before | After |
|--------|-------|
| `Memory` struct | `Constraint` struct |
| `MemoryStore` type | `ConstraintStore` type |
| `NewMemoryStore()` | `NewConstraintStore()` |
| SQL: `bee_memories` | SQL: `bee_constraints` |

Test file: `memory_store_test.go` → `constraint_store_test.go`, symbols updated.

### MCP Tool Names

File: `internal/infra/utils/toolnames.go`

| Before | After |
|--------|-------|
| `ToolSaveMemory = "save_memory"` | `ToolSaveConstraint = "save_constraint"` |
| `ToolGetMemory = "get_memory"` | `ToolGetConstraint = "get_constraint"` |
| `ToolDeleteMemory = "delete_memory"` | `ToolDeleteConstraint = "delete_constraint"` |

### MCP Tool Handlers

File: `internal/mcp/tools.go`

| Before | After |
|--------|-------|
| `toolSaveMemory()` | `toolSaveConstraint()` |
| `toolGetMemory()` | `toolGetConstraint()` |
| `toolDeleteMemory()` | `toolDeleteConstraint()` |

Tool description text: all "memory" references → "constraint".

### MCP Server

File: `internal/mcp/server.go`

- Field `memoryStore` → `constraintStore`
- Constructor parameter updated

### App Layer

File: `internal/app/app.go`

- Field `memoryStore` → `constraintStore`
- `store.NewMemoryStore(db)` → `store.NewConstraintStore(db)`
- All call sites updated

### CLI

File: `cmd/openbee/ctl_memory.go` → `ctl_constraint.go`

| Before | After |
|--------|-------|
| `openbee ctl memory get` | `openbee ctl constraint get` |
| `openbee ctl memory save` | `openbee ctl constraint save` |
| `openbee ctl memory delete` | `openbee ctl constraint delete` |

Sub-command names (`get`, `save`, `delete`) are unchanged.

### Skill Documentation

Files under `internal/infra/skillinstall/skills/openbee-bee/references/`:

- `memory-management.md` → `constraint-management.md`; all content updated to use "constraint" terminology and new tool names
- `entity-relationships.md`: memory-related descriptions updated

## Files Changed

| Layer | File | Change |
|-------|------|--------|
| DB | `internal/infra/store/db.go` | New migration: create `bee_constraints`, copy data, drop `bee_memories` |
| Store | `internal/infra/store/memory_store.go` | Rename to `constraint_store.go`, update all symbols |
| Store Test | `internal/infra/store/memory_store_test.go` | Rename to `constraint_store_test.go`, update all symbols |
| MCP Tools | `internal/mcp/tools.go` | Rename 3 handler functions and description text |
| MCP Tools Test | `internal/mcp/tools_test.go` | Update all memory tool name references |
| MCP Server | `internal/mcp/server.go` | Field `memoryStore` → `constraintStore` |
| Tool Names | `internal/infra/utils/toolnames.go` | 3 constants and string values updated |
| App | `internal/app/app.go` | Field and call site updates |
| CLI | `cmd/openbee/ctl_memory.go` | Rename to `ctl_constraint.go`, command name updated |
| Skill Doc | `skills/openbee-bee/SKILL.md` | Update memory tool name references |
| Skill Doc | `skills/openbee-bee/references/memory-management.md` | Rename to `constraint-management.md`, content updated |
| Skill Doc | `skills/openbee-bee/references/entity-relationships.md` | Memory descriptions updated |

## Out of Scope

- Database field names (`key`, `value`, `scope`) — unchanged
- Business logic — unchanged
- API endpoint paths — unchanged
- Worker constraints code — already uses `constraint` naming, not touched
