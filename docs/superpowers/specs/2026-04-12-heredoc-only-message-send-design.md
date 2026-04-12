# Design: Enforce Heredoc-Only for `openbee ctl message send --content`

**Date:** 2026-04-12
**Status:** Implemented

---

## Background

The `openbee ctl message send` command supports two ways to pass text content:

- **Old approach (direct arg):** `--content "text"` — passes content as a shell argument
- **New approach (stdin/heredoc):** `--content - << 'EOF' ... EOF` — reads content from stdin

The new stdin approach was added to prevent shell expansion of special characters (backticks, `$(...)`, code blocks, etc.) that are common in AI-generated messages.

## Problem

The previous skill documentation presented both approaches side by side and instructed the AI model to choose between them at runtime based on whether the content contained "special characters." This created two problems:

1. **Decision risk**: AI models are unreliable at classifying whether generated content is "safe" for direct shell argument passing. Messages that appear safe may contain special characters that cause silent errors or unintended shell expansion.
2. **Cognitive overhead**: Every `message send` call required a branch decision that had no upside — the heredoc approach is safe for all content, simple or complex.

## Decision

Enforce heredoc as the **only** documented and required method for passing `--content`. The old `--content "..."` direct-arg approach is removed from all skill documentation.

**Rationale:**
- Heredoc is universally safe regardless of content type
- Eliminates the runtime decision entirely
- The verbosity cost (two extra lines) is negligible for AI-generated commands
- The old approach will be removed at the code level in a future change, making this consistent

## Changes Made

### `internal/infra/skillinstall/skills/openbee-worker/SKILL.md`

1. **Communication Hard Gate**: Replaced two-path example (direct arg + heredoc conditional) with heredoc-only example
2. **Task Notification Spec inline example**: Updated to heredoc
3. **Notification Examples**: All `--content "..."` replaced with `--content - << 'EOF' ... EOF`; combined `--content` + `--media-path` cases use `--content - --media-path <path> << 'EOF'` on one command
4. **CLI Reference message subcommand**: Examples updated to heredoc-only
5. **Deleted**: "⚠️ Sending Content with Special Characters" section (no longer needed — there is no longer a conditional; heredoc is always used)

### `internal/infra/skillinstall/skills/openbee-bee/SKILL.md`

1. **CLI Reference message subcommand**: All `--content "..."` examples replaced with heredoc; combined text+media cases use single command with both flags

### `~/.claude/skills/openbee-worker/SKILL.md`

Synced with project source (identical content).

## Out of Scope

- Removal of `--content <string>` parameter from the CLI binary itself (tracked separately)
- `openbee-bee/SKILL.md` notification spec sections (bee skill does not have the same Hard Gate structure)
