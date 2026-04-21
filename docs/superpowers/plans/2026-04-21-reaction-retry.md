# Reaction Retry Mechanism Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add exponential-backoff retry (max 5 attempts) to Feishu and DingTalk reaction add/recall operations so transient network failures no longer silently drop the "processing" indicator.

**Architecture:** A shared `RetryWithBackoff` utility function is created in `internal/utils/retry.go`. Both `feishu/handler.go` and `dingtalk/handler.go` wrap their reaction API calls with this function. Retries run inside existing goroutines so message replies are never blocked.

**Tech Stack:** Go stdlib (`context`, `time`), `go.uber.org/zap` (logging), existing Lark SDK and DingTalk HTTP client.

---

## File Map

| Action | File | Purpose |
|--------|------|---------|
| Create | `internal/utils/retry.go` | `RetryWithBackoff` utility function |
| Create | `internal/utils/retry_test.go` | Unit tests for RetryWithBackoff |
| Modify | `internal/platform/feishu/handler.go` lines 138-156 | Wrap addReaction with retry |
| Modify | `internal/platform/feishu/handler.go` lines 472-481 | Wrap recallReaction with retry |
| Modify | `internal/platform/dingtalk/handler.go` lines 984-986 | Wrap addThinkingEmoji with retry |
| Modify | `internal/platform/dingtalk/handler.go` lines 989-991 | Wrap recallThinkingEmoji with retry |

---

## Task 1: Shared RetryWithBackoff utility

**Files:**
- Create: `internal/utils/retry.go`
- Create: `internal/utils/retry_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/utils/retry_test.go`:

```go
package utils_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/theopenbee/openbee/internal/utils"
)

var errFake = errors.New("fake error")

func TestRetryWithBackoff_SuccessOnFirst(t *testing.T) {
	calls := 0
	err := utils.RetryWithBackoff(context.Background(), func() error {
		calls++
		return nil
	}, 5, time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetryWithBackoff_SuccessOnThird(t *testing.T) {
	calls := 0
	err := utils.RetryWithBackoff(context.Background(), func() error {
		calls++
		if calls < 3 {
			return errFake
		}
		return nil
	}, 5, time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryWithBackoff_AllFail(t *testing.T) {
	calls := 0
	err := utils.RetryWithBackoff(context.Background(), func() error {
		calls++
		return errFake
	}, 5, time.Millisecond)
	if !errors.Is(err, errFake) {
		t.Fatalf("expected errFake, got %v", err)
	}
	if calls != 5 {
		t.Fatalf("expected 5 calls, got %d", calls)
	}
}

func TestRetryWithBackoff_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := utils.RetryWithBackoff(ctx, func() error {
		calls++
		cancel() // cancel after first failure
		return errFake
	}, 5, time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call before cancel, got %d", calls)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/tengyongzhi/work/bot-workspaces/openbee
go test ./internal/utils/... -v
```

Expected: compilation error — package does not exist yet.

- [ ] **Step 3: Create the retry utility**

Create `internal/utils/retry.go`:

```go
package utils

import (
	"context"
	"time"
)

// RetryWithBackoff retries fn with exponential backoff up to maxRetries times.
// The first attempt is immediate. On failure, it waits baseDelay before the
// second attempt, doubling the delay each round. No wait after the final attempt.
// Returns the last error if all attempts fail, or ctx.Err() if cancelled.
func RetryWithBackoff(ctx context.Context, fn func() error, maxRetries int, baseDelay time.Duration) error {
	var err error
	delay := baseDelay
	for i := 0; i < maxRetries; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i < maxRetries-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}
	}
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/utils/... -v
```

Expected output (all PASS):
```
--- PASS: TestRetryWithBackoff_SuccessOnFirst
--- PASS: TestRetryWithBackoff_SuccessOnThird
--- PASS: TestRetryWithBackoff_AllFail
--- PASS: TestRetryWithBackoff_ContextCancelled
PASS
```

- [ ] **Step 5: Commit**

```bash
git add internal/utils/retry.go internal/utils/retry_test.go
git commit -m "feat: add RetryWithBackoff utility with exponential backoff"
```

---

## Task 2: Feishu addReaction retry

**Files:**
- Modify: `internal/platform/feishu/handler.go` lines 134-157

- [ ] **Step 1: Replace addReaction goroutine body with retry**

In `internal/platform/feishu/handler.go`, replace the goroutine body (lines 134-157) that starts with `go func() {` for the addReaction section.

Current code (lines 134-157):
```go
go func() {
    defer time.AfterFunc(10*time.Minute, func() {
        r.pendingReactions.Delete(*msg.MessageId)
    })
    resp, err := r.larkClient.Im.MessageReaction.Create(ctx,
        larkim.NewCreateMessageReactionReqBuilder().
            MessageId(*msg.MessageId).
            Body(larkim.NewCreateMessageReactionReqBodyBuilder().
                ReactionType(larkim.NewEmojiBuilder().
                    EmojiType("Typing").
                    Build()).
                Build()).
            Build())
    if err != nil || !resp.Success() {
        log.Error("add reaction error", zap.Error(err), zap.Any("resp", resp))
        close(reactionCh)
        return
    }
    if resp.Data != nil && resp.Data.ReactionId != nil {
        reactionCh <- *resp.Data.ReactionId
    } else {
        close(reactionCh)
    }
}()
```

Replace with:
```go
go func() {
    defer time.AfterFunc(10*time.Minute, func() {
        r.pendingReactions.Delete(*msg.MessageId)
    })
    req := larkim.NewCreateMessageReactionReqBuilder().
        MessageId(*msg.MessageId).
        Body(larkim.NewCreateMessageReactionReqBodyBuilder().
            ReactionType(larkim.NewEmojiBuilder().
                EmojiType("Typing").
                Build()).
            Build()).
        Build()
    err := utils.RetryWithBackoff(ctx, func() error {
        resp, e := r.larkClient.Im.MessageReaction.Create(ctx, req)
        if e != nil {
            return e
        }
        if !resp.Success() {
            return fmt.Errorf("add reaction: %v", resp.CodeError)
        }
        if resp.Data != nil && resp.Data.ReactionId != nil {
            reactionCh <- *resp.Data.ReactionId
        } else {
            close(reactionCh)
        }
        return nil
    }, 5, 500*time.Millisecond)
    if err != nil {
        log.Error("add reaction failed after retries", zap.Error(err))
        close(reactionCh)
    }
}()
```

Make sure `"github.com/theopenbee/openbee/internal/utils"` is in the import block. The package already imports `"fmt"` — verify it's present or add it.

- [ ] **Step 2: Verify the build compiles**

```bash
go build ./internal/platform/feishu/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/platform/feishu/handler.go
git commit -m "feat(feishu): retry addReaction with exponential backoff"
```

---

## Task 3: Feishu recallReaction retry

**Files:**
- Modify: `internal/platform/feishu/handler.go` lines 472-481

- [ ] **Step 1: Replace the recall Delete call with retry**

In `internal/platform/feishu/handler.go`, inside `FeishuSender.Send`, find the `recallCtx` block (lines ~472-481):

Current code:
```go
recallCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
resp, err := s.larkClient.Im.MessageReaction.Delete(recallCtx,
    larkim.NewDeleteMessageReactionReqBuilder().
        MessageId(messageID).
        ReactionId(reactionID).
        Build())
cancel()
if err != nil || !resp.Success() {
    log.Warn("recall reaction error", zap.Error(err), zap.Any("resp", resp))
}
```

Replace with:
```go
recallCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
req := larkim.NewDeleteMessageReactionReqBuilder().
    MessageId(messageID).
    ReactionId(reactionID).
    Build()
if err := utils.RetryWithBackoff(recallCtx, func() error {
    resp, e := s.larkClient.Im.MessageReaction.Delete(recallCtx, req)
    if e != nil {
        return e
    }
    if !resp.Success() {
        return fmt.Errorf("recall reaction: %v", resp.CodeError)
    }
    return nil
}, 5, 500*time.Millisecond); err != nil {
    log.Warn("recall reaction failed after retries", zap.Error(err))
}
```

Note: The timeout is extended to 30s to accommodate up to 5 retry attempts (max ~7.5s wait) plus network round-trips. The `cancel()` call moves to `defer` since it's in a goroutine.

- [ ] **Step 2: Verify the build compiles**

```bash
go build ./internal/platform/feishu/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/platform/feishu/handler.go
git commit -m "feat(feishu): retry recallReaction with exponential backoff"
```

---

## Task 4: DingTalk addThinkingEmoji and recallThinkingEmoji retry

**Files:**
- Modify: `internal/platform/dingtalk/handler.go` lines 951-991

- [ ] **Step 1: Refactor doEmojiRequest to return error**

Currently `doEmojiRequest` (line 951) returns nothing. To support retry, it must return `error`.

Find the function signature and body at lines 951-981:

```go
func doEmojiRequest(ctx context.Context, cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel, url string, timeout time.Duration, action string) {
```

Replace the entire `doEmojiRequest` function with:

```go
func doEmojiRequest(ctx context.Context, cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel, url string, timeout time.Duration, action string) error {
	token, err := getAccessToken(cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return fmt.Errorf("get access token for emoji %s: %w", action, err)
	}

	payload := buildEmojiPayload(cfg, data)

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create emoji %s request: %w", action, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s emoji reaction: %w", action, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("emoji %s returned non-200: %d", action, resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 2: Update addThinkingEmoji and recallThinkingEmoji to use retry**

Replace the existing `addThinkingEmoji` and `recallThinkingEmoji` functions (lines 983-991):

```go
func addThinkingEmoji(ctx context.Context, cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel) {
	retryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := utils.RetryWithBackoff(retryCtx, func() error {
		return doEmojiRequest(ctx, cfg, data, "https://api.dingtalk.com/v1.0/robot/emotion/reply", 5*time.Second, "reply")
	}, 5, 500*time.Millisecond); err != nil {
		log.Error("add emoji failed after retries", zap.Error(err))
	}
}

func recallThinkingEmoji(ctx context.Context, cfg config.DingTalkConfig, data *chatbot.BotCallbackDataModel) {
	retryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := utils.RetryWithBackoff(retryCtx, func() error {
		return doEmojiRequest(ctx, cfg, data, "https://api.dingtalk.com/v1.0/robot/emotion/recall", 3*time.Second, "recall")
	}, 5, 500*time.Millisecond); err != nil {
		log.Warn("recall emoji failed after retries", zap.Error(err))
	}
}
```

Add `"github.com/theopenbee/openbee/internal/utils"` to the import block.

- [ ] **Step 3: Verify the build compiles**

```bash
go build ./internal/platform/dingtalk/...
```

Expected: no errors.

- [ ] **Step 4: Run DingTalk handler tests**

```bash
go test ./internal/platform/dingtalk/... -v
```

Expected: all existing tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/dingtalk/handler.go
git commit -m "feat(dingtalk): retry emoji add/recall with exponential backoff"
```

---

## Task 5: Full build and test verification

- [ ] **Step 1: Build entire project**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 2: Run all tests**

```bash
go test ./... -timeout 120s
```

Expected: all tests PASS.

- [ ] **Step 3: Update CHANGELOG**

Add to the `[Unreleased]` section of `CHANGELOG.md`:

```markdown
### Changed
- Feishu and DingTalk reaction add/recall now retry up to 5 times with exponential backoff (500ms base delay) on network failures, improving resilience to transient connectivity issues.
```

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md
git commit -m "chore: update changelog for reaction retry mechanism"
```
