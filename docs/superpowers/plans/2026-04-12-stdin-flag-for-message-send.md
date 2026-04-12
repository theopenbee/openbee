# stdin Flag for Message Send Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `--content` with a `--stdin` bool flag in `openbee ctl message send`, removing the legacy `\n`-literal replacement and the dual-mode string/sentinel design.

**Architecture:** Single-file Go change in `cmd/openbee/ctl_message.go` plus mechanical find-replace in two SKILL.md docs. No new types or interfaces needed.

**Tech Stack:** Go (cobra flags, `io.ReadAll`), Markdown

---

## File Map

| Action | File |
|--------|------|
| Modify | `cmd/openbee/ctl_message.go` |
| Modify | `internal/infra/skillinstall/skills/openbee-worker/SKILL.md` |
| Modify | `internal/infra/skillinstall/skills/openbee-bee/SKILL.md` |

---

### Task 1: Update `ctl_message.go` — replace `--content` with `--stdin`

**Files:**
- Modify: `cmd/openbee/ctl_message.go`

- [ ] **Step 1: Read the current file**

Read `cmd/openbee/ctl_message.go` to confirm the current state before editing.

- [ ] **Step 2: Rewrite the file**

Replace the entire file content with:

```go
package main

import (
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/infra/utils"
)

var ctlMessageCmd = &cobra.Command{Use: "message", Short: ""}

var (
	msgSendMessageID string
	msgSendStdin     bool
	msgSendMediaPath string
)

var ctlMessageSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to the user on the originating platform",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := map[string]any{"message_id": msgSendMessageID}
		if msgSendStdin {
			b, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			if len(b) > 0 {
				a["content"] = string(b)
			}
		}
		if msgSendMediaPath != "" {
			a["media_path"] = msgSendMediaPath
		}
		return ctlRun(utils.SendMessage, a)
	},
}

func init() {
	ctlMessageSendCmd.Flags().StringVar(&msgSendMessageID, "message-id", "", "ID of the originating platform message (required)")
	ctlMessageSendCmd.Flags().BoolVar(&msgSendStdin, "stdin", false, "Read text content from stdin (use with heredoc)")
	ctlMessageSendCmd.Flags().StringVar(&msgSendMediaPath, "media-path", "", "Local file path to upload and send as media")
	ctlMessageSendCmd.MarkFlagRequired("message-id")

	ctlMessageCmd.AddCommand(ctlMessageSendCmd)
	ctlCmd.AddCommand(ctlMessageCmd)
}
```

- [ ] **Step 3: Build to verify**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee && go build ./cmd/openbee/...
```

Expected: no output, exit 0.

- [ ] **Step 4: Commit**

```bash
git add cmd/openbee/ctl_message.go
git commit -m "feat: replace --content with --stdin in openbee ctl message send

Remove the dual-mode --content flag (string value / '-' sentinel) and
legacy \\n-literal replacement. --stdin is a bool flag that reads text
from stdin, intended for use with heredoc.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Update `openbee-worker/SKILL.md`

**Files:**
- Modify: `internal/infra/skillinstall/skills/openbee-worker/SKILL.md`

- [ ] **Step 1: Apply replacements**

Make the following targeted edits (each is a distinct `old_string` → `new_string`):

**Edit 1** — Communication Hard Gate example (line 29):
```
old: openbee ctl message send --message-id <id> --content - << 'EOF'
new: openbee ctl message send --message-id <id> --stdin << 'EOF'
```
Note: this pattern appears multiple times; use `replace_all: true`.

**Edit 2** — Task Notification Spec header example (line 62):
```
old: openbee ctl message send --message-id <message_id> --content - << 'EOF'
new: openbee ctl message send --message-id <message_id> --stdin << 'EOF'
```

**Edit 3** — Combined text + image example (line 101):
```
old: openbee ctl message send --message-id <id> --content - --media-path /tmp/result.png << 'EOF'
new: openbee ctl message send --message-id <id> --stdin --media-path /tmp/result.png << 'EOF'
```

**Edit 4** — Combined text + pdf example (line 106):
```
old: openbee ctl message send --message-id <id> --content - --media-path /tmp/report.pdf << 'EOF'
new: openbee ctl message send --message-id <id> --stdin --media-path /tmp/report.pdf << 'EOF'
```

**Edit 5** — CLI reference signature (line 125):
```
old: openbee ctl message send --message-id <id> [--content -] [--media-path <file path>]
new: openbee ctl message send --message-id <id> [--stdin] [--media-path <file path>]
```

**Edit 6** — CLI reference comment (line 128):
```
old: # --content and --media-path can be used independently or together (text first, then media)
new: # --stdin and --media-path can be used independently or together (text first, then media)
```

**Edit 7** — CLI reference combined example (line 139):
```
old: openbee ctl message send --message-id <id> --content - --media-path /tmp/output.csv << 'EOF'
new: openbee ctl message send --message-id <id> --stdin --media-path /tmp/output.csv << 'EOF'
```

- [ ] **Step 2: Verify no `--content` remains**

```bash
grep -n "content -" internal/infra/skillinstall/skills/openbee-worker/SKILL.md
```

Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/skillinstall/skills/openbee-worker/SKILL.md
git commit -m "docs(skill): update openbee-worker to use --stdin instead of --content -

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Update `openbee-bee/SKILL.md`

**Files:**
- Modify: `internal/infra/skillinstall/skills/openbee-bee/SKILL.md`

- [ ] **Step 1: Apply replacements**

**Edit 1** — CLI reference signature (line 300):
```
old: openbee ctl message send --message-id <id> [--content -] [--media-path <file path>]
new: openbee ctl message send --message-id <id> [--stdin] [--media-path <file path>]
```

**Edit 2** — All `--content -` in scenario examples (lines 305, 310, 315, 320); use `replace_all: true`:
```
old: openbee ctl message send --message-id <id> --content - << 'EOF'
new: openbee ctl message send --message-id <id> --stdin << 'EOF'
```

**Edit 3** — Combined text + media scenarios (lines 310, 315):
```
old: openbee ctl message send --message-id <id> --content - --media-path /tmp/overview.png << 'EOF'
new: openbee ctl message send --message-id <id> --stdin --media-path /tmp/overview.png << 'EOF'
```

```
old: openbee ctl message send --message-id <id> --content - --media-path /tmp/tasks.csv << 'EOF'
new: openbee ctl message send --message-id <id> --stdin --media-path /tmp/tasks.csv << 'EOF'
```

- [ ] **Step 2: Verify no `--content` remains in message subcommand section**

```bash
grep -n "content -" internal/infra/skillinstall/skills/openbee-bee/SKILL.md
```

Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add internal/infra/skillinstall/skills/openbee-bee/SKILL.md
git commit -m "docs(skill): update openbee-bee to use --stdin instead of --content -

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```
