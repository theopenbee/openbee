# Design: Replace `--content` with `--stdin` in `openbee ctl message send`

**Date:** 2026-04-12
**Status:** Approved

---

## Background

`openbee ctl message send` currently accepts text content via `--content`:

- Direct string: `--content "text"` — susceptible to shell expansion
- Stdin/heredoc: `--content -` — reads from stdin, safe for all content

A previous change enforced heredoc-only in skill documentation. This design completes the migration by removing `--content` from the CLI binary itself and replacing it with a dedicated `--stdin` bool flag.

## Decision

Replace `--content` with `--stdin` (a boolean flag). Reading text content from stdin is now the only supported method.

**Rationale:**
- `--stdin` is semantically clear: it means "read text from stdin"
- Eliminates the dual-mode `--content` flag (string value vs `-` sentinel)
- Removes the legacy `\n`-literal-to-newline replacement, which only served the direct-string path
- No backward compatibility required: the primary consumer (AI skills) has already migrated to heredoc via doc updates

## New Interface

```bash
# Text only
openbee ctl message send --message-id <id> --stdin << 'EOF'
message content
EOF

# Text + media
openbee ctl message send --message-id <id> --stdin --media-path /tmp/img.png << 'EOF'
image caption
EOF

# Media only (--stdin omitted)
openbee ctl message send --message-id <id> --media-path /tmp/img.png
```

CLI signature:
```
openbee ctl message send --message-id <id> [--stdin] [--media-path <file path>]
```

## Changes

### `cmd/openbee/ctl_message.go`

- Remove `msgSendContent string` variable
- Remove `--content` flag registration
- Add `msgSendStdin bool` variable
- Add `--stdin` bool flag: `"Read text content from stdin (use with heredoc)"`
- Replace stdin-read logic: trigger on `msgSendStdin` instead of `msgSendContent == "-"`
- Remove `strings.ReplaceAll(msgSendContent, \`\n\`, "\n")` — no longer needed
- Remove `"io"` and `"strings"` imports if no longer used

### `internal/infra/skillinstall/skills/openbee-worker/SKILL.md`

- Replace all `--content -` occurrences with `--stdin`
- Replace `--content - --media-path` with `--stdin --media-path`
- Update CLI reference signature

### `internal/infra/skillinstall/skills/openbee-bee/SKILL.md`

- Same replacements as above

## Out of Scope

- Local skill sync at `~/.claude/skills/openbee-worker/SKILL.md`
- Any other `ctl` subcommands
