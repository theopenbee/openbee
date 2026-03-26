# Skill Management Design

**Date:** 2026-03-25
**Status:** Draft

## Overview

Introduce skill management capability to openbee, enabling users to create, version, and manage Claude Code skills for global use and per-worker customization. Skills are prompt-based capabilities stored as `SKILL.md` files that extend what Claude can do.

## Background

Claude Code loads skills from two directories:

- **Personal (global):** `~/.claude/skills/<skill-name>/` — available to all projects and workers
- **Project (per-worker):** `{WorkDir}/.claude/skills/<skill-name>/` — applies only to that worker

A worker with a local skill of the same name as a global skill uses the local one (local overrides global).

Currently openbee has no mechanism to create, track, or version skills. The `skills-lock.json` in the project root records some skill sources but doesn't manage local skill lifecycle.

## Goals

- Create and edit skills locally through openbee
- Track multiple versions of each skill; switch or rollback without editing files manually
- Distinguish openbee-managed skills from externally-placed ones
- Expose management via CLI, REST API, and Web UI

## Out of Scope

- Installing skills from remote sources (GitHub, npm, etc.)
- Plugin-scoped skills

---

## Architecture

### Two-Tier Structure

```
[Registry — real files]
~/.openbee/skills/
  <skill-name>/
    v1/
      SKILL.md
      <optional supporting files>
    v2/
      SKILL.md

[Link layer — what Claude Code reads]
~/.claude/skills/
  <skill-name>  ->  ~/.openbee/skills/<skill-name>/v2   (global symlink)

{WorkDir}/.claude/skills/
  <skill-name>  ->  ~/.openbee/skills/<skill-name>/v1   (worker override symlink)
```

Claude Code reads the link layer and is unaware of symlinks. Version switching is an atomic `ln -sfn` operation.

### State File: `.openbee/skills.json`

Tracks all openbee-managed skills and worker-level version overrides.

```json
{
  "version": 1,
  "skills": {
    "brainstorming": {
      "description": "Brainstorming and design discussions",
      "latest_version": "v2",
      "global_version": "v2",
      "versions": {
        "v1": { "created_at": "2026-03-01T10:00:00Z" },
        "v2": { "created_at": "2026-03-25T08:00:00Z" }
      }
    }
  },
  "worker_overrides": {
    "<worker_id>": {
      "brainstorming": "v1"
    }
  }
}
```

**Fields:**

| Field | Description |
|---|---|
| `skills.<name>.latest_version` | Highest version created (not necessarily active globally) |
| `skills.<name>.global_version` | Version the global symlink currently points to |
| `worker_overrides.<id>.<name>` | Worker-specific version; absent means inherit global |

Externally-placed skills (not openbee-managed) are **not** recorded in this file. They are identified dynamically during scan.

---

## Skill Classification

When scanning `~/.claude/skills/` or `{WorkDir}/.claude/skills/`, each entry is classified:

```
Is it a symlink?
  ├── Yes → Target path starts with ~/.openbee/skills/?
  │          ├── Yes → openbee-managed
  │          └── No  → external symlink (unmanaged)
  └── No  → real directory (unmanaged)
```

For worker scans, also note whether a global skill of the same name exists — if so, the worker entry is flagged as "override."

---

## Operations

### Openbee-Managed Skills

| Operation | Description |
|---|---|
| `list` | List all skills with source and active version |
| `create` | Create new skill, write to registry as v1, create global symlink |
| `edit` | Open SKILL.md in editor; saving creates a new version (does not auto-switch) |
| `delete` | Remove from registry and all symlinks |
| `versions` | Show version history for a skill |
| `use` | Switch global or worker symlink to a specified version |

### External Skills (Unmanaged)

| Operation | Description |
|---|---|
| `list` | Appear in listing with `external` label |
| `delete` | Remove the directory or symlink |
| `adopt` | Copy files into registry as v1, replace original with managed symlink |

---

## CLI Interface

```bash
# List skills — global by default; --worker shows worker's effective view
openbee skill list [--worker <id>] [--global]

# Create a new managed skill
openbee skill create <name> [--description "..."]

# Edit skill content (opens $EDITOR; save creates new version)
openbee skill edit <name> [--worker <id>]

# Delete a skill
openbee skill delete <name> [--worker <id>] [--force]

# List version history
openbee skill versions <name>

# Switch active version (global or worker)
openbee skill use <name> <version> [--global | --worker <id>]

# Adopt an externally-placed skill into openbee management
openbee skill adopt <name> [--global | --worker <id>]
```

Default scope (when neither `--global` nor `--worker` is specified) is `--global`.

---

## REST API

### Skill Registry

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/skills` | List all skills (global view) |
| `POST` | `/api/skills` | Create skill |
| `GET` | `/api/skills/:name` | Skill details with version list |
| `DELETE` | `/api/skills/:name` | Delete skill and all symlinks |
| `POST` | `/api/skills/:name/versions` | Save new version (body: SKILL.md content) |
| `PUT` | `/api/skills/:name/global-version` | Switch global active version |
| `POST` | `/api/skills/:name/adopt` | Adopt external global skill |

### Worker Dimension

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/workers/:id/skills` | Worker's effective skill list |
| `PUT` | `/api/workers/:id/skills/:name` | Set worker version override |
| `DELETE` | `/api/workers/:id/skills/:name` | Remove override (revert to global) |
| `POST` | `/api/workers/:id/skills/:name/adopt` | Adopt external worker skill |

---

## Version Lifecycle

### Creating a Version (edit flow)

1. Read current `latest_version` from `skills.json`
2. Present SKILL.md content to user (editor or web form)
3. On save: create `v{N+1}/` in registry, write new content
4. Update `skills.json`: `latest_version = "v{N+1}"`
5. **Do not** auto-update `global_version` — editing is not publishing

This separation allows testing a new version on a specific worker before promoting it globally.

### Switching Versions

```
openbee skill use brainstorming v3 --global
  1. Verify v3/ exists in registry
  2. ln -sfn ~/.openbee/skills/brainstorming/v3 ~/.claude/skills/brainstorming
  3. Update skills.json: global_version = "v3"

openbee skill use brainstorming v2 --worker worker_abc
  1. Verify v2/ exists
  2. ln -sfn ~/.openbee/skills/brainstorming/v2 {WorkDir_abc}/.claude/skills/brainstorming
  3. Update skills.json: worker_overrides.worker_abc.brainstorming = "v2"
```

Rollback is identical — just `use` an older version number.

### Adopt Flow (External → Managed)

```
openbee skill adopt brainstorming --global
  1. Locate ~/.claude/skills/brainstorming
  2. Classify:
     - Symlink → openbee registry: already managed, error out
     - Symlink → elsewhere: follow link, read real files
     - Real directory: read directly
  3. Copy contents to ~/.openbee/skills/brainstorming/v1/
  4. Remove original path; create symlink → v1
  5. Write entry to skills.json
  6. Output: skill 'brainstorming' is now managed at v1
```

---

## Edge Cases

| Scenario | Handling |
|---|---|
| Delete version still referenced by a worker symlink | Refuse deletion; list affected workers. Require `--force` to override (leaves broken symlinks, warns user). |
| Worker deleted with active skill overrides | Worker teardown removes `{WorkDir}/.claude/skills/` symlinks for all openbee-managed skills. |
| External skill shares name with managed skill | Cannot coexist at same scope level. `adopt` resolves the conflict. |
| Adopt on already-managed skill | Detect via symlink target check; return error with current version info. |
| Concurrent writes to `skills.json` | Use file lock (lockfile adjacent to `skills.json`) for all write operations. |
| Windows platform | Symlinks require Developer Mode. Short-term fallback: copy files instead of symlink; flag worker as "copy mode" in `skills.json`. |
| Worker has local skill that overrides global | `list --worker` marks such entries as `[override]`; `delete` on worker scope removes override, falling back to global. |
