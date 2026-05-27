# Multi-Account Per Platform Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Support N accounts per IM platform (feishu / wecom / weixin / dingtalk / telegram / linear) with full per-account isolation of messages, sessions, tasks, and executions; worker / engine / env / department / auth stay global.

**Architecture:** YAML config moves to platform-keyed lists with mandatory unique `name`. A new `account_name` column propagates through 5 tables. SessionKey gains the account segment. A `sendersByAccount` map keyed by `<platform>:<account>` replaces `sendersByPlatform`. Existing single-bot deployments must migrate their YAML to list form before startup; DB rows are filled with `account_name='default'` via column default.

**Tech Stack:** Go 1.x, SQLite, embedded migrations in `internal/infra/store/db.go`, gin, yaml.v3, standard `testing` package.

**Spec reference:** `docs/superpowers/specs/2026-05-26-multi-account-per-platform-design.md`

---

## File Structure

### New files
- `internal/platform/account.go` — small helper for `<platform>:<account>` key construction and validation
- `internal/platform/account_test.go` — tests for helper

### Modified files
**Config layer**
- `internal/infra/config/config.go` — types become lists; `applyDefaults` per-element; validation added
- `internal/infra/config/config.yaml.tmpl` — example list format
- `internal/infra/config/config_bee_test.go` — list parsing, name uniqueness, legacy-format error
- `config.yaml` — repo-level example updated

**DB layer**
- `internal/infra/store/db.go` — add migration v45
- `internal/infra/store/db_test.go` — test v45 migration
- `internal/infra/store/message_store.go` — accept/persist/filter `account_name`
- `internal/infra/store/outbound_message_store.go` — same
- `internal/infra/store/session_store.go` — same
- `internal/infra/store/task_store.go` — same
- `internal/infra/store/execution_store.go` — same
- corresponding `*_test.go` files

**Domain**
- `internal/platform/interfaces.go` — `InboundMessage.AccountName`
- `internal/platform/feishu/handler.go` — SessionKey + AccountName
- `internal/platform/wecom/handler.go` — same
- `internal/platform/dingtalk/handler.go` — same
- `internal/platform/telegram/handler.go` — same
- `internal/platform/weixin/handler.go` — same
- `internal/platform/linear/handler.go` — same
- `internal/platform/local/*.go` — fixed `"default"` AccountName
- `internal/platform/{feishu,wecom,...}/handler_test.go` — SessionKey assertions
- `internal/domain/msgingest/gateway.go` — `WithAccountBotNames` keyed by `<platform>:<account>`
- `internal/domain/task/failure_notifier.go` — route key uses `<platform>:<account>`

**Wiring**
- `internal/app/app.go` — `buildPlatforms` loops over list; `sendersByAccount`; rewired handlers

**Docs**
- `CHANGELOG.md`
- `README.md` / `README.zh.md` (config example block)

---

## Task 1: Account-key helper

**Files:**
- Create: `internal/platform/account.go`
- Create: `internal/platform/account_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/platform/account_test.go
package platform

import "testing"

func TestAccountKey(t *testing.T) {
    if got := AccountKey("feishu", "marketing-bot"); got != "feishu:marketing-bot" {
        t.Fatalf("got %q", got)
    }
}

func TestValidateAccountName(t *testing.T) {
    good := []string{"default", "marketing-bot", "a", "team_1", "ops-2026"}
    for _, n := range good {
        if err := ValidateAccountName(n); err != nil {
            t.Fatalf("valid name %q rejected: %v", n, err)
        }
    }
    bad := []string{"", "Marketing", "bot.1", "a b", "你好", "-leading", "trailing-"}
    for _, n := range bad {
        if err := ValidateAccountName(n); err == nil {
            t.Fatalf("invalid name %q accepted", n)
        }
    }
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/platform/ -run 'TestAccountKey|TestValidateAccountName' -v`
Expected: FAIL — `AccountKey` / `ValidateAccountName` undefined.

- [ ] **Step 3: Implement the helper**

```go
// internal/platform/account.go
package platform

import (
    "errors"
    "regexp"
)

// AccountKey returns the composite routing key used to address a single
// account on a platform (e.g. "feishu:marketing-bot").
func AccountKey(platformID, accountName string) string {
    return platformID + ":" + accountName
}

var accountNameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]*[a-z0-9])?$`)

// ValidateAccountName enforces the lowercase [a-z0-9_-] alphabet,
// non-empty, no leading/trailing dash or underscore.
func ValidateAccountName(name string) error {
    if name == "" {
        return errors.New("account name must not be empty")
    }
    if !accountNameRE.MatchString(name) {
        return errors.New("account name must match [a-z0-9][a-z0-9_-]*[a-z0-9]")
    }
    return nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/platform/ -run 'TestAccountKey|TestValidateAccountName' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/platform/account.go internal/platform/account_test.go
git commit -m "feat(platform): add AccountKey helper and name validator"
```

---

## Task 2: Config types become lists

**Files:**
- Modify: `internal/infra/config/config.go`
- Modify: `internal/infra/config/config_bee_test.go`

- [ ] **Step 1: Write failing tests for list parsing**

Append to `internal/infra/config/config_bee_test.go`:

```go
func TestLoad_PlatformList(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "config.yaml")
    err := os.WriteFile(p, []byte(`
bee:
  platforms:
    feishu:
      - name: marketing
        enabled: true
        app_id: id1
        app_secret: s1
      - name: support
        enabled: true
        app_id: id2
        app_secret: s2
    wecom: []
    dingtalk: []
    telegram: []
    weixin: []
    linear: []
`), 0o600)
    if err != nil { t.Fatal(err) }
    cfg, err := Load(p)
    if err != nil { t.Fatalf("load: %v", err) }
    if n := len(cfg.Bee.Platforms.Feishu); n != 2 {
        t.Fatalf("feishu len=%d", n)
    }
    if cfg.Bee.Platforms.Feishu[0].Name != "marketing" {
        t.Fatalf("name=%q", cfg.Bee.Platforms.Feishu[0].Name)
    }
}

func TestLoad_LegacyFormatRejected(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "config.yaml")
    err := os.WriteFile(p, []byte(`
bee:
  platforms:
    feishu:
      enabled: true
      app_id: id1
      app_secret: s1
`), 0o600)
    if err != nil { t.Fatal(err) }
    if _, err := Load(p); err == nil {
        t.Fatal("expected error for legacy map format")
    }
}

func TestLoad_DuplicateAccountName(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "config.yaml")
    err := os.WriteFile(p, []byte(`
bee:
  platforms:
    feishu:
      - name: dup
        enabled: true
        app_id: id1
        app_secret: s1
      - name: dup
        enabled: true
        app_id: id2
        app_secret: s2
`), 0o600)
    if err != nil { t.Fatal(err) }
    if _, err := Load(p); err == nil {
        t.Fatal("expected duplicate-name error")
    }
}

func TestLoad_InvalidAccountName(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "config.yaml")
    err := os.WriteFile(p, []byte(`
bee:
  platforms:
    feishu:
      - name: Bad.Name
        enabled: true
        app_id: id1
        app_secret: s1
`), 0o600)
    if err != nil { t.Fatal(err) }
    if _, err := Load(p); err == nil {
        t.Fatal("expected invalid-name error")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/infra/config/ -run 'TestLoad_PlatformList|TestLoad_LegacyFormatRejected|TestLoad_DuplicateAccountName|TestLoad_InvalidAccountName' -v`
Expected: FAIL — types still single struct.

- [ ] **Step 3: Convert struct types to slices + add Name field**

In `internal/infra/config/config.go`:

Replace the `PlatformsConfig` struct:

```go
type PlatformsConfig struct {
    Feishu   []FeishuConfig   `yaml:"feishu"`
    DingTalk []DingTalkConfig `yaml:"dingtalk"`
    WeCom    []WeComConfig    `yaml:"wecom"`
    Telegram []TelegramConfig `yaml:"telegram"`
    Weixin   []WeixinConfig   `yaml:"weixin"`
    Linear   []LinearConfig   `yaml:"linear"`
}
```

Add `Name` (first field) on each of the six platform configs:

```go
type FeishuConfig struct {
    Name         string `yaml:"name"`
    Enabled      bool   `yaml:"enabled"`
    AppID        string `yaml:"app_id"`
    AppSecret    string `yaml:"app_secret"`
    MaxMediaSize int    `yaml:"max_media_size"`
    BotName      string `yaml:"bot_name"`
}
// repeat for DingTalkConfig, WeComConfig, TelegramConfig, WeixinConfig, LinearConfig:
// add `Name string \`yaml:"name"\`` as first field.
```

Replace the body of `BotNames`:

```go
func (p PlatformsConfig) BotNames() []string {
    var names []string
    add := func(s string) { if s != "" { names = append(names, s) } }
    for _, c := range p.Feishu   { add(c.BotName) }
    for _, c := range p.DingTalk { add(c.BotName) }
    for _, c := range p.WeCom    { add(c.BotName) }
    for _, c := range p.Telegram { add(c.BotName) }
    for _, c := range p.Weixin   { add(c.BotName) }
    return names
}
```

- [ ] **Step 4: Implement validation in `applyDefaults`**

Replace per-platform default blocks (currently `cfg.Bee.Platforms.Feishu.MaxMediaSize` etc.) with loops, and add name validation:

```go
import "github.com/theopenbee/openbee/internal/platform" // for ValidateAccountName

// inside applyDefaults, replace the per-platform default lines with:

for i := range cfg.Bee.Platforms.Feishu {
    if cfg.Bee.Platforms.Feishu[i].MaxMediaSize == 0 {
        cfg.Bee.Platforms.Feishu[i].MaxMediaSize = 100 * 1024 * 1024
    }
}
for i := range cfg.Bee.Platforms.WeCom {
    if cfg.Bee.Platforms.WeCom[i].WebSocketURL == "" {
        cfg.Bee.Platforms.WeCom[i].WebSocketURL = "wss://openws.work.weixin.qq.com"
    }
}
for i := range cfg.Bee.Platforms.Telegram {
    if cfg.Bee.Platforms.Telegram[i].MaxMediaSize == 0 {
        cfg.Bee.Platforms.Telegram[i].MaxMediaSize = 50 * 1024 * 1024
    }
}
for i := range cfg.Bee.Platforms.Weixin {
    if cfg.Bee.Platforms.Weixin[i].BaseURL == "" {
        cfg.Bee.Platforms.Weixin[i].BaseURL = "https://ilinkai.weixin.qq.com"
    }
    if cfg.Bee.Platforms.Weixin[i].CDNBaseURL == "" {
        cfg.Bee.Platforms.Weixin[i].CDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
    }
    if cfg.Bee.Platforms.Weixin[i].MaxMediaSize == 0 {
        cfg.Bee.Platforms.Weixin[i].MaxMediaSize = 100 * 1024 * 1024
    }
}
for i := range cfg.Bee.Platforms.Linear {
    if cfg.Bee.Platforms.Linear[i].MaxMediaSize == 0 {
        cfg.Bee.Platforms.Linear[i].MaxMediaSize = 50 * 1024 * 1024
    }
    if cfg.Bee.Platforms.Linear[i].LabelName == "" {
        cfg.Bee.Platforms.Linear[i].LabelName = "openbee"
    }
    if cfg.Bee.Platforms.Linear[i].PollInterval == 0 {
        cfg.Bee.Platforms.Linear[i].PollInterval = 10 * time.Second
    }
}

if err := validatePlatformAccounts(cfg.Bee.Platforms); err != nil {
    return err
}
```

Add the validator at the end of the file:

```go
func validatePlatformAccounts(p PlatformsConfig) error {
    type entry struct{ platform string; names []string }
    type ids interface{ getName() string }

    check := func(platform string, names []string) error {
        seen := make(map[string]bool, len(names))
        for _, n := range names {
            if err := platform_pkg.ValidateAccountName(n); err != nil {
                return fmt.Errorf("%s account name %q: %w", platform, n, err)
            }
            if seen[n] {
                return fmt.Errorf("%s has duplicate account name %q", platform, n)
            }
            seen[n] = true
        }
        return nil
    }

    feishuNames := make([]string, len(p.Feishu));   for i, c := range p.Feishu   { feishuNames[i]   = c.Name }
    dtNames     := make([]string, len(p.DingTalk)); for i, c := range p.DingTalk { dtNames[i]       = c.Name }
    wcNames     := make([]string, len(p.WeCom));    for i, c := range p.WeCom    { wcNames[i]       = c.Name }
    tgNames     := make([]string, len(p.Telegram)); for i, c := range p.Telegram { tgNames[i]       = c.Name }
    wxNames     := make([]string, len(p.Weixin));   for i, c := range p.Weixin   { wxNames[i]       = c.Name }
    lnNames     := make([]string, len(p.Linear));   for i, c := range p.Linear   { lnNames[i]       = c.Name }

    for _, e := range []struct{ platform string; names []string }{
        {"feishu", feishuNames}, {"dingtalk", dtNames}, {"wecom", wcNames},
        {"telegram", tgNames}, {"weixin", wxNames}, {"linear", lnNames},
    } {
        if err := check(e.platform, e.names); err != nil { return err }
    }
    return nil
}
```

Add to imports of `config.go`:

```go
platform_pkg "github.com/theopenbee/openbee/internal/platform"
```

**Note:** if importing `platform` creates an import cycle, copy `ValidateAccountName` and its regex into `config.go` as an unexported helper and drop the import.

- [ ] **Step 5: Reject legacy single-map format**

YAML unmarshalling into `[]FeishuConfig` will return a type error for the legacy map form (e.g., `feishu: {app_id: ...}`). Surface that error explicitly in `Load`:

In `Load`, wrap the unmarshal error:

```go
if err := yaml.Unmarshal(data, &cfg); err != nil {
    return Config{}, fmt.Errorf(`config parse error: %w; note: each platform under bee.platforms must be a YAML list, e.g. "feishu: [{name: default, ...}]"`, err)
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/infra/config/ -v`
Expected: all four new tests PASS. Old tests may have to be touched if they relied on map form — fix those callers to use list literals.

- [ ] **Step 7: Commit**

```bash
git add internal/infra/config/config.go internal/infra/config/config_bee_test.go
git commit -m "feat(config): platforms become per-account lists with name validation"
```

---

## Task 3: Update config templates and example

**Files:**
- Modify: `internal/infra/config/config.yaml.tmpl`
- Modify: `config.yaml`

- [ ] **Step 1: Update the embedded template**

Replace the existing `platforms:` block in `internal/infra/config/config.yaml.tmpl` with:

```yaml
  platforms:
    feishu: []
    dingtalk: []
    wecom: []
    telegram: []
    weixin: []
    linear: []
    # Each platform is a list of accounts; example:
    # feishu:
    #   - name: default
    #     enabled: true
    #     app_id: ""
    #     app_secret: ""
    #     bot_name: ""
    #     max_media_size: 104857600
```

- [ ] **Step 2: Update repo-level example**

In `config.yaml` replace the `platforms:` block with:

```yaml
  platforms:
    feishu:
      - name: default
        enabled: false
        app_id: ""
        app_secret: ""
        max_media_size: 104857600
    dingtalk:
      - name: default
        enabled: false
        client_id: ""
        client_secret: ""
    wecom:
      - name: default
        enabled: false
        bot_id: "YOUR_BOT_ID"
        secret: "YOUR_BOT_SECRET"
    telegram: []
    weixin: []
    linear: []
```

- [ ] **Step 3: Verify build still passes**

Run: `go build ./...`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/infra/config/config.yaml.tmpl config.yaml
git commit -m "docs(config): update example templates for list-based platforms"
```

---

## Task 4: DB migration v45 — add `account_name` columns

**Files:**
- Modify: `internal/infra/store/db.go`
- Modify: `internal/infra/store/db_test.go`

- [ ] **Step 1: Write failing test for migration v45**

Append to `internal/infra/store/db_test.go`:

```go
func TestMigration_AccountNameColumns(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "test.db")
    db, err := InitDB(dbPath)
    if err != nil { t.Fatal(err) }
    defer db.Close()

    tables := []string{
        "bee_platform_messages",
        "bee_outbound_messages",
        "bee_sessions",
        "bee_tasks",
        "bee_worker_executions",
    }
    for _, tbl := range tables {
        var name string
        row := db.QueryRow(`SELECT name FROM pragma_table_info(?) WHERE name='account_name'`, tbl)
        if err := row.Scan(&name); err != nil {
            t.Fatalf("table %s missing account_name: %v", tbl, err)
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/store/ -run TestMigration_AccountNameColumns -v`
Expected: FAIL — column missing.

- [ ] **Step 3: Add migration v45**

In `internal/infra/store/db.go`, append to the `migrations` slice:

```go
{
    version: 45,
    name:    "add_account_name_to_per_account_tables",
    sql: `
        ALTER TABLE bee_platform_messages  ADD COLUMN account_name TEXT NOT NULL DEFAULT 'default';
        ALTER TABLE bee_outbound_messages  ADD COLUMN account_name TEXT NOT NULL DEFAULT 'default';
        ALTER TABLE bee_sessions           ADD COLUMN account_name TEXT NOT NULL DEFAULT 'default';
        ALTER TABLE bee_tasks              ADD COLUMN account_name TEXT NOT NULL DEFAULT 'default';
        ALTER TABLE bee_worker_executions  ADD COLUMN account_name TEXT NOT NULL DEFAULT 'default';
        CREATE INDEX IF NOT EXISTS idx_messages_platform_account     ON bee_platform_messages(platform, account_name);
        CREATE INDEX IF NOT EXISTS idx_outbound_messages_platform_account ON bee_outbound_messages(platform, account_name);
        CREATE INDEX IF NOT EXISTS idx_sessions_platform_account     ON bee_sessions(platform, account_name);
        CREATE INDEX IF NOT EXISTS idx_tasks_platform_account        ON bee_tasks(platform, account_name);
        CREATE INDEX IF NOT EXISTS idx_executions_platform_account   ON bee_worker_executions(platform, account_name);
    `,
},
```

(If any of those 5 tables does **not** currently have a `platform` column, drop the index for that table; the column-add still applies.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/infra/store/ -v`
Expected: PASS for the new test and existing tests.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/db.go internal/infra/store/db_test.go
git commit -m "feat(store): migration v45 adds account_name to per-account tables"
```

---

## Task 5: Thread `account_name` through MessageStore

**Files:**
- Modify: `internal/infra/store/message_store.go`
- Modify: `internal/infra/store/message_store_test.go` (or create if missing)

- [ ] **Step 1: Write failing test**

Add to `internal/infra/store/message_store_test.go` (create file if missing; mirror pattern in `db_test.go` for DB setup):

```go
func TestMessageStore_CreateWithAccount(t *testing.T) {
    db := newTestDB(t) // helper that calls InitDB on a temp file; reuse existing if present
    s := NewMessageStore(db)
    created, err := s.Create(context.Background(), "m1", "feishu:marketing:c:u", "feishu", "marketing", "hi", "hi", "p1", time.Now().UnixMilli())
    if err != nil { t.Fatal(err) }
    if !created { t.Fatal("expected created=true") }

    var account string
    row := db.QueryRow(`SELECT account_name FROM bee_platform_messages WHERE id='m1'`)
    if err := row.Scan(&account); err != nil { t.Fatal(err) }
    if account != "marketing" { t.Fatalf("account=%q", account) }
}
```

If `newTestDB` doesn't exist, add this helper to the test file:

```go
func newTestDB(t *testing.T) *sql.DB {
    t.Helper()
    db, err := InitDB(filepath.Join(t.TempDir(), "test.db"))
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { db.Close() })
    return db
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/store/ -run TestMessageStore_CreateWithAccount -v`
Expected: FAIL — Create signature mismatch.

- [ ] **Step 3: Extend `Create` signature with `account` parameter**

In `message_store.go`, update:

```go
func (s *MessageStore) Create(
    ctx context.Context,
    id, sessionKey, platform, account, content, raw, platformMsgID string,
    messageTime int64,
) (bool, error) {
    // existing logic; add account_name into INSERT column list and value
}
```

Update the INSERT statement to include `account_name` and pass the new parameter. Also include `account_name` in the `MessageFilter`:

```go
type MessageFilter struct {
    Platform    string
    AccountName string
    // ... existing fields ...
}
```

In `ListFiltered`, when `f.AccountName != ""`, append `AND account_name = ?` to the WHERE clause.

- [ ] **Step 4: Fix every existing caller of `MessageStore.Create`**

Grep callers and update each to pass account name. Most callers receive an `InboundMessage` — after Task 7 adds the field, pass `msg.AccountName`. For tests that hard-code messages, pass `"default"`.

Run: `grep -rn 'msgStore.Create\|MessageStore.*Create' --include='*.go' /Users/tengyongzhi/work/bot-workspaces/openbee` to enumerate sites.

- [ ] **Step 5: Run all store tests**

Run: `go test ./internal/infra/store/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/infra/store/message_store.go internal/infra/store/message_store_test.go $(git ls-files -m | grep '\.go$')
git commit -m "feat(store): MessageStore persists and filters account_name"
```

---

## Task 6: Thread `account_name` through OutboundMessageStore

**Files:**
- Modify: `internal/infra/store/outbound_message_store.go`
- Modify: `internal/infra/store/outbound_message_store_test.go` (create if missing)

- [ ] **Step 1: Write failing test**

```go
func TestOutboundMessageStore_PersistsAccount(t *testing.T) {
    db := newTestDB(t)
    s := NewOutboundMessageStore(db)
    err := s.Create(context.Background(), OutboundMessage{
        ID:          "o1",
        Platform:    "feishu",
        AccountName: "support",
        SessionKey:  "feishu:support:c:u",
        Content:     "hello",
    })
    if err != nil { t.Fatal(err) }

    var account string
    if err := db.QueryRow(`SELECT account_name FROM bee_outbound_messages WHERE id='o1'`).Scan(&account); err != nil {
        t.Fatal(err)
    }
    if account != "support" { t.Fatalf("account=%q", account) }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infra/store/ -run TestOutboundMessageStore_PersistsAccount -v`
Expected: FAIL — `AccountName` field missing on `OutboundMessage`.

- [ ] **Step 3: Add `AccountName` field and propagate**

In `outbound_message_store.go`:

```go
type OutboundMessage struct {
    // ... existing fields ...
    AccountName string
}

type OutboundMessageFilter struct {
    // ... existing fields ...
    AccountName string
}
```

Include `account_name` in INSERT statement; include filter clause in `ListFiltered` when set.

- [ ] **Step 4: Update `LoggingPlatformSenderAdapter`** so it stamps account on the persisted row

In `internal/infra/store/outbound_message_store.go` (or wherever `NewLoggingPlatformSenderAdapter` lives), accept an `accountName string` parameter:

```go
func NewLoggingPlatformSenderAdapter(
    inner platform.PlatformSenderAdapter,
    store *OutboundMessageStore,
    platformID, accountName string,
) platform.PlatformSenderAdapter
```

Set `msg.AccountName = accountName` (or alternative: have `OutboundMessage` already carry it from `msg.ReplyTo.AccountName` in callers).

- [ ] **Step 5: Run tests**

Run: `go test ./internal/infra/store/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/infra/store/outbound_message_store.go internal/infra/store/outbound_message_store_test.go
git commit -m "feat(store): OutboundMessage carries account_name; logging adapter stamps it"
```

---

## Task 7: Add `AccountName` to `InboundMessage`

**Files:**
- Modify: `internal/platform/interfaces.go`
- Modify: `internal/platform/context.go` and `_test.go` if relevant

- [ ] **Step 1: Write failing test in `internal/platform/context_test.go`**

Add a test asserting that whatever helper extracts session context preserves an account name (look at existing test pattern):

```go
func TestInboundMessage_AccountName(t *testing.T) {
    msg := InboundMessage{Platform: "feishu", AccountName: "marketing"}
    if msg.AccountName != "marketing" {
        t.Fatalf("want marketing, got %q", msg.AccountName)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/ -run TestInboundMessage_AccountName -v`
Expected: FAIL — `AccountName` undefined.

- [ ] **Step 3: Add the field**

In `internal/platform/interfaces.go`:

```go
type InboundMessage struct {
    Platform          string
    AccountName       string // new: identifies which account on the platform
    SenderID          string
    SessionKey        string
    Content           string
    RawContent        string
    Raw               string
    PlatformMessageID string
    MessageTime       int64
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/platform/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/platform/interfaces.go internal/platform/context_test.go
git commit -m "feat(platform): add AccountName to InboundMessage"
```

---

## Task 8: Feishu handler — emit AccountName + new SessionKey

**Files:**
- Modify: `internal/platform/feishu/handler.go`
- Modify: `internal/platform/feishu/handler_test.go`

- [ ] **Step 1: Write failing test**

Add a test that constructs the handler with `Name: "marketing"` and asserts the parsed `InboundMessage` has `AccountName == "marketing"` and `SessionKey == "feishu:marketing:<chat>:<sender>"`.

```go
func TestFeishu_InboundCarriesAccount(t *testing.T) {
    cfg := config.FeishuConfig{Name: "marketing", AppID: "x", AppSecret: "y"}
    // ... whichever constructor / direct method exposes the parse path ...
    got := buildInboundMessageForTest(cfg, "chatX", "userY", "hello") // helper or inlined parse
    if got.AccountName != "marketing" { t.Fatalf("account=%q", got.AccountName) }
    if got.SessionKey != "feishu:marketing:chatX:userY" {
        t.Fatalf("session=%q", got.SessionKey)
    }
}
```

If `buildInboundMessageForTest` doesn't exist, factor the inbound-message assembly into a small testable function (e.g., `buildInbound(cfg FeishuConfig, chat, sender, content string) InboundMessage`) and exercise it directly.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/platform/feishu/ -v -run TestFeishu_InboundCarriesAccount`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `feishu/handler.go` around line 178 where `SessionKey` is built, change:

```go
SessionKey: PlatformID + ":" + *msg.ChatId + ":" + senderID,
```

to:

```go
SessionKey: PlatformID + ":" + h.cfg.Name + ":" + *msg.ChatId + ":" + senderID,
```

Add `AccountName: h.cfg.Name,` to the same `InboundMessage` literal. Ensure the handler struct keeps a reference to the cfg (or just the name field).

- [ ] **Step 4: Run feishu tests**

Run: `go test ./internal/platform/feishu/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/platform/feishu/handler.go internal/platform/feishu/handler_test.go
git commit -m "feat(feishu): inbound message carries account name; session key prefixed"
```

---

## Task 9: WeCom handler

**Files:**
- Modify: `internal/platform/wecom/handler.go`
- Modify: `internal/platform/wecom/handler_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestWecom_InboundCarriesAccount(t *testing.T) {
    cfg := config.WeComConfig{Name: "ops", BotID: "id", Secret: "s"}
    got := buildInboundForTest(cfg, "chatA", "userB", "hi")
    if got.AccountName != "ops" || got.SessionKey != "wecom:ops:chatA:userB" {
        t.Fatalf("got %+v", got)
    }
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/platform/wecom/ -v -run TestWecom_InboundCarriesAccount`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `wecom/handler.go:289`, change:

```go
SessionKey: PlatformID + ":" + chatID + ":" + senderID,
```

to include `h.cfg.Name`:

```go
SessionKey: PlatformID + ":" + h.cfg.Name + ":" + chatID + ":" + senderID,
```

Add `AccountName: h.cfg.Name`. Keep `cfg` or `name` field on the handler.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/platform/wecom/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/wecom/handler.go internal/platform/wecom/handler_test.go
git commit -m "feat(wecom): inbound message carries account name; session key prefixed"
```

---

## Task 10: DingTalk handler

**Files:**
- Modify: `internal/platform/dingtalk/handler.go`
- Modify: `internal/platform/dingtalk/handler_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestDingTalk_InboundCarriesAccount(t *testing.T) {
    cfg := config.DingTalkConfig{Name: "team1", ClientID: "id", ClientSecret: "s"}
    got := buildInboundForTest(cfg, "convX", "staffY", "hi")
    if got.AccountName != "team1" || got.SessionKey != "dingtalk:team1:convX:staffY" {
        t.Fatalf("got %+v", got)
    }
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/platform/dingtalk/ -v -run TestDingTalk_InboundCarriesAccount`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `dingtalk/handler.go:148`, change SessionKey to include `h.cfg.Name`; add `AccountName: h.cfg.Name`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/platform/dingtalk/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/dingtalk/handler.go internal/platform/dingtalk/handler_test.go
git commit -m "feat(dingtalk): inbound message carries account name; session key prefixed"
```

---

## Task 11: Telegram handler

**Files:**
- Modify: `internal/platform/telegram/handler.go`
- Modify: `internal/platform/telegram/handler_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestTelegram_buildSessionKey_includesAccount(t *testing.T) {
    if got := buildSessionKey("ops", int64(123), int64(456)); got != "telegram:ops:123:456" {
        t.Fatalf("got %q", got)
    }
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/platform/telegram/ -v -run TestTelegram_buildSessionKey_includesAccount`
Expected: FAIL — current signature is `buildSessionKey(int64, int64)`.

- [ ] **Step 3: Implement**

In `telegram/handler.go:76-77` change `buildSessionKey` signature to:

```go
func buildSessionKey(account string, chatID, senderID int64) string {
    return fmt.Sprintf("telegram:%s:%d:%d", account, chatID, senderID)
}
```

Update the call site to pass `h.cfg.Name`. Add `AccountName: h.cfg.Name` to the `InboundMessage` literal.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/platform/telegram/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/telegram/handler.go internal/platform/telegram/handler_test.go
git commit -m "feat(telegram): inbound message carries account name; session key prefixed"
```

---

## Task 12: WeChat (weixin) handler

**Files:**
- Modify: `internal/platform/weixin/handler.go`
- Modify: `internal/platform/weixin/handler_test.go` (create if missing)

- [ ] **Step 1: Write failing test**

```go
func TestWeixin_buildSessionKey_includesAccount(t *testing.T) {
    if got := buildSessionKey("personal", "userZ"); got != "weixin:personal:userZ:userZ" {
        t.Fatalf("got %q", got)
    }
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/platform/weixin/ -v -run TestWeixin_buildSessionKey_includesAccount`
Expected: FAIL.

- [ ] **Step 3: Implement**

Change `buildSessionKey` from `func(userID string) string { return fmt.Sprintf("weixin:%s:%s", userID, userID) }` to:

```go
func buildSessionKey(account, userID string) string {
    return fmt.Sprintf("weixin:%s:%s:%s", account, userID, userID)
}
```

Update call sites and the `InboundMessage` literal to set `AccountName: h.cfg.Name`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/platform/weixin/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/weixin/handler.go internal/platform/weixin/handler_test.go
git commit -m "feat(weixin): inbound message carries account name; session key prefixed"
```

---

## Task 13: Linear handler

**Files:**
- Modify: `internal/platform/linear/handler.go`
- Modify: `internal/platform/linear/handler_test.go` (create if missing)

- [ ] **Step 1: Write failing test**

```go
func TestLinear_buildSessionKey_includesAccount(t *testing.T) {
    if got := buildSessionKey("workspaceA", "TEAM", "ENG-1"); got != "linear:workspaceA:TEAM:ENG-1" {
        t.Fatalf("got %q", got)
    }
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/platform/linear/ -v -run TestLinear_buildSessionKey_includesAccount`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `linear/handler.go:247-248` change to:

```go
func buildSessionKey(account, teamKey, identifier string) string {
    return fmt.Sprintf("%s:%s:%s:%s", PlatformID, account, teamKey, identifier)
}
```

Update call sites. Add `AccountName: h.cfg.Name` to the `InboundMessage` literal.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/platform/linear/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/linear/handler.go internal/platform/linear/handler_test.go
git commit -m "feat(linear): inbound message carries account name; session key prefixed"
```

---

## Task 14: Local platform fixed AccountName

**Files:**
- Modify: `internal/platform/local/sender.go`, `internal/platform/local/receiver.go`, `internal/api/local_chat_handler.go`

- [ ] **Step 1: Write failing test**

Locate the local platform's existing test file and add:

```go
func TestLocal_AccountNameIsDefault(t *testing.T) {
    msg := buildLocalInbound("sessionKey", "hi") // adjust to actual helper
    if msg.AccountName != "default" {
        t.Fatalf("got %q", msg.AccountName)
    }
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/platform/local/ ./internal/api/ -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Wherever the local platform builds an `InboundMessage`, set `AccountName: "default"`. Wherever local SessionKey is constructed (`"local:default"` already), no change is needed beyond ensuring InboundMessage carries the account name.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/platform/local/ ./internal/api/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/local/ internal/api/local_chat_handler.go
git commit -m "feat(local): inbound messages carry AccountName=default"
```

---

## Task 15: SessionStore persists account_name

**Files:**
- Modify: `internal/infra/store/session_store.go`
- Modify: `internal/infra/store/session_store_test.go` (create if missing)

- [ ] **Step 1: Write failing test**

```go
func TestSessionStore_UpsertWithAccount(t *testing.T) {
    db := newTestDB(t)
    s := NewSessionStore(db)
    if err := s.UpsertSessionContext(context.Background(), "feishu:m:c:u", "feishu", "marketing", "agentX", "sX", "claude"); err != nil {
        t.Fatal(err)
    }
    var account string
    if err := db.QueryRow(`SELECT account_name FROM bee_sessions WHERE session_key='feishu:m:c:u'`).Scan(&account); err != nil {
        t.Fatal(err)
    }
    if account != "marketing" { t.Fatalf("account=%q", account) }
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/infra/store/ -run TestSessionStore_UpsertWithAccount -v`
Expected: FAIL — signature mismatch.

- [ ] **Step 3: Implement**

Update `UpsertSessionContext` signature to accept `platform` and `account` (or pull from session key — but explicit args are clearer):

```go
func (s *SessionStore) UpsertSessionContext(
    ctx context.Context,
    sessionKey, platform, account, agentID, sessionID, engine string,
) error
```

Persist `account_name` column. Update callers — primarily in `internal/domain/bee/feeder.go` and `internal/domain/task/*`. They receive `InboundMessage` or stored message rows; pass `msg.AccountName` / row's account_name. Compile errors will pinpoint sites.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/infra/store/ -v && go build ./...`
Expected: PASS / build OK.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/session_store.go internal/infra/store/session_store_test.go $(git ls-files -m | grep '\.go$')
git commit -m "feat(store): SessionStore persists account_name"
```

---

## Task 16: TaskStore persists account_name

**Files:**
- Modify: `internal/infra/store/task_store.go`
- Modify: `internal/infra/store/task_store_test.go`
- Modify: `internal/infra/model/task.go`

- [ ] **Step 1: Write failing test**

In `internal/infra/store/task_store_test.go`:

```go
func TestTaskStore_CreateWithAccount(t *testing.T) {
    db := newTestDB(t)
    s := NewTaskStore(db)
    id, err := s.Create(context.Background(), model.Task{
        MessageID:   "m1",
        WorkerID:    "w1",
        Platform:    "feishu",
        AccountName: "marketing",
        Instruction: "hi",
        Type:        model.TaskTypeImmediate,
        Status:      model.TaskStatusPending,
    })
    if err != nil { t.Fatal(err) }
    var account string
    if err := db.QueryRow(`SELECT account_name FROM bee_tasks WHERE id=?`, id).Scan(&account); err != nil {
        t.Fatal(err)
    }
    if account != "marketing" { t.Fatalf("account=%q", account) }
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/infra/store/ -run TestTaskStore_CreateWithAccount -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Add fields to `model.Task`:

```go
type Task struct {
    // ... existing ...
    Platform    string
    AccountName string
}
```

Add `Platform` only if not already present; check current struct first. Update `Create` to include them in INSERT. Update `List` / `TaskFilter` similarly:

```go
type TaskFilter struct {
    // ... existing ...
    Platform    string
    AccountName string
}
```

- [ ] **Step 4: Update `ClaimedTask`** to carry account_name (it already has `MessagePlatform` — add `MessageAccountName`):

```go
type ClaimedTask struct {
    Task
    MessageSessionKey  string
    MessagePlatform    string
    MessageAccountName string
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/infra/store/ -v && go build ./...`
Expected: PASS / build OK.

- [ ] **Step 6: Commit**

```bash
git add internal/infra/store/task_store.go internal/infra/store/task_store_test.go internal/infra/model/task.go $(git ls-files -m | grep '\.go$')
git commit -m "feat(store): TaskStore persists account_name"
```

---

## Task 17: ExecutionStore persists account_name

**Files:**
- Modify: `internal/infra/store/execution_store.go`
- Modify: `internal/infra/store/execution_store_test.go` (create if missing)
- Modify: `internal/infra/model/execution.go`

- [ ] **Step 1: Write failing test**

```go
func TestExecutionStore_CreateWithAccount(t *testing.T) {
    db := newTestDB(t)
    s := NewExecutionStore(db, t.TempDir())
    exec, err := s.Create("w1", "do", "session-1", "feishu", "marketing", "claude")
    if err != nil { t.Fatal(err) }
    var account string
    if err := db.QueryRow(`SELECT account_name FROM bee_worker_executions WHERE id=?`, exec.ID).Scan(&account); err != nil {
        t.Fatal(err)
    }
    if account != "marketing" { t.Fatalf("account=%q", account) }
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/infra/store/ -run TestExecutionStore_CreateWithAccount -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Update `WorkerExecution` model:

```go
type WorkerExecution struct {
    // ... existing fields ...
    Platform    string `json:"platform,omitempty" db:"platform"`
    AccountName string `json:"account_name,omitempty" db:"account_name"`
}
```

(`Platform` is already present in some places — check; only add what's missing.)

Update `Create` signature to take `platform, account`. Same for `ExecutionFilter`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/infra/store/ -v && go build ./...`
Expected: PASS / build OK.

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/execution_store.go internal/infra/store/execution_store_test.go internal/infra/model/execution.go $(git ls-files -m | grep '\.go$')
git commit -m "feat(store): ExecutionStore persists account_name"
```

---

## Task 18: Rename `sendersByPlatform` → `sendersByAccount`

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/domain/task/failure_notifier.go`
- Modify: `internal/domain/task/failure_notifier_test.go`
- Modify: `internal/domain/command/{engine,clear,stop,status,list}.go` and corresponding `_test.go`
- Modify: `internal/rpc/bee_server.go` (look for the field on `rpc.Server`)

- [ ] **Step 1: Write failing test**

In `internal/domain/task/failure_notifier_test.go` add:

```go
func TestFailureNotifier_RoutesByAccount(t *testing.T) {
    sent := make(map[string]string)
    senders := map[string]platform.PlatformSenderAdapter{
        "feishu:marketing": fakeSender(func(m platform.OutboundMessage) { sent["feishu:marketing"] = m.Content }),
        "feishu:support":   fakeSender(func(m platform.OutboundMessage) { sent["feishu:support"] = m.Content }),
    }
    n := NewPlatformFailureNotifier(/*msgStore*/ nil, senders)
    n.Notify(context.Background(), FailureInfo{
        Platform:    "feishu",
        AccountName: "support",
        SessionKey:  "feishu:support:c:u",
        Reason:      "boom",
    })
    if got := sent["feishu:support"]; got == "" { t.Fatalf("support notifier not invoked") }
    if got := sent["feishu:marketing"]; got != "" { t.Fatalf("wrong account got the notification: %q", got) }
}
```

(`fakeSender` is a small test helper closure adapting `platform.PlatformSenderAdapter`. Define it inline if not already present.)

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/domain/task/ -run TestFailureNotifier_RoutesByAccount -v`
Expected: FAIL — map key still platform-only.

- [ ] **Step 3: Update `PlatformFailureNotifier` to use composite key**

In `failure_notifier.go`, change lookup to:

```go
sender, ok := n.senders[platform.AccountKey(info.Platform, info.AccountName)]
```

Add `AccountName string` to the `FailureInfo` struct if missing, and ensure callers populate it.

- [ ] **Step 4: Update each command handler** (`engine`, `clear`, `stop`, `status`, `list`)

Each receives `sendersByPlatform map[string]platform.PlatformSenderAdapter` constructor param. Rename to `sendersByAccount` and use `platform.AccountKey(msg.Platform, msg.AccountName)` to look up sender when replying. Update the call sites in `internal/app/app.go`.

- [ ] **Step 5: Update `internal/app/app.go` map name and construction site**

Rename local `sendersByPlatform` to `sendersByAccount`. The `LoggingPlatformSenderAdapter` constructor now takes both `platformID` and `accountName`.

- [ ] **Step 6: Run all affected packages**

Run: `go test ./internal/domain/task/ ./internal/domain/command/ ./internal/rpc/ -v && go build ./...`
Expected: PASS / build OK.

- [ ] **Step 7: Commit**

```bash
git add internal/app/app.go internal/domain/task/ internal/domain/command/ internal/rpc/
git commit -m "refactor: route outbound by AccountKey(platform, account)"
```

---

## Task 19: `buildPlatforms` loops over per-platform account lists

**Files:**
- Modify: `internal/app/app.go`

- [ ] **Step 1: Write failing test**

Add an integration-style test in `internal/app/app_test.go` (create if needed):

```go
func TestBuildPlatforms_MultipleFeishuAccounts(t *testing.T) {
    cfg := config.BeeConfig{
        Platforms: config.PlatformsConfig{
            Feishu: []config.FeishuConfig{
                {Name: "a", Enabled: true, AppID: "x", AppSecret: "y"},
                {Name: "b", Enabled: true, AppID: "x2", AppSecret: "y2"},
            },
        },
        Media: config.MediaConfig{},
    }
    platforms, err := buildPlatforms(cfg.Platforms.Feishu, cfg.Platforms.DingTalk, cfg.Platforms.WeCom,
        cfg.Platforms.Telegram, cfg.Platforms.Weixin, cfg.Platforms.Linear, cfg.Media)
    if err != nil { t.Fatal(err) }
    if len(platforms) != 2 {
        t.Fatalf("len=%d", len(platforms))
    }
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/app/ -run TestBuildPlatforms_MultipleFeishuAccounts -v`
Expected: FAIL.

- [ ] **Step 3: Refactor `buildPlatforms`**

Replace each `if fc.Enabled { result = append(result, feishu.NewPlatform(fc, mediaSvc)) }` block with a loop over the slice, e.g.:

```go
for _, fc := range fcList {
    if !fc.Enabled { continue }
    platform.RegisterExtractor(feishu.PlatformID, feishu.ExtractContext) // idempotent
    result = append(result, feishu.NewPlatform(fc, mediaSvc))
}
// repeat for DingTalk, WeCom, Telegram, Weixin, Linear
```

Change the signature so each parameter is the slice type from the new `PlatformsConfig`.

- [ ] **Step 4: Refactor the `sendersByAccount` population**

Inside `BuildApp`, after `buildPlatforms` returns, populate the map with `(p.ID(), p.AccountName())` keys. To get the account name, either:

- Have `platform.Platform` expose `AccountName() string`, or
- Return a `[]struct{ ID, AccountName string; ... }` from `buildPlatforms`.

Recommended: add `AccountName() string` to the `Platform` interface:

```go
// internal/platform/interfaces.go
type Platform interface {
    ID() string
    AccountName() string
    Receiver() PlatformReceiverAdapter
    Sender() PlatformSenderAdapter
}
```

Each platform's `NewPlatform` stores `cfg.Name` and returns it from `AccountName()`. Update wiring:

```go
for _, p := range platforms {
    key := platform.AccountKey(p.ID(), p.AccountName())
    sendersByAccount[key] = store.NewLoggingPlatformSenderAdapter(p.Sender(), s.outboundMsgStore, p.ID(), p.AccountName())
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/app/ ./internal/platform/... -v && go build ./...`
Expected: PASS / build OK.

- [ ] **Step 6: Commit**

```bash
git add internal/app/app.go internal/platform/interfaces.go internal/platform/
git commit -m "refactor(app): construct one platform per account; expose AccountName()"
```

---

## Task 20: Bot-name @mention stripping per account

**Files:**
- Modify: `internal/domain/msgingest/gateway.go`
- Modify: `internal/domain/msgingest/gateway_test.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Write failing test**

```go
func TestStripBotMention_PerAccount(t *testing.T) {
    g := New(/*msgStore*/ nil, time.Millisecond, /*chain*/ nil,
        WithAccountBotNames(map[string]string{
            "feishu:marketing": "营销小蜜",
            "feishu:support":   "客服小蜜",
        }))
    in := platform.InboundMessage{
        Platform: "feishu", AccountName: "marketing",
        Content: "@营销小蜜 hello", RawContent: "@营销小蜜 hello",
    }
    stripped := g.stripBotMention(in)
    if !strings.HasPrefix(stripped, "hello") { t.Fatalf("not stripped: %q", stripped) }
}
```

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/domain/msgingest/ -run TestStripBotMention_PerAccount -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Rename `WithPlatformBotNames` → `WithAccountBotNames` taking `map[string]string` keyed by `AccountKey`. Update `stripBotMention(content, platform string)` signature to `stripBotMention(msg InboundMessage) string` (or `stripBotMention(content, platform, account string)`). Update the call to use `platform.AccountKey(msg.Platform, msg.AccountName)` for lookup.

In `internal/app/app.go`, replace the bot-name map literal with a loop over each platform's list:

```go
accountBotNames := map[string]string{}
for _, fc := range cfg.Bee.Platforms.Feishu {
    if fc.BotName != "" { accountBotNames[platform.AccountKey(feishu.PlatformID, fc.Name)] = fc.BotName }
}
for _, dc := range cfg.Bee.Platforms.DingTalk { /* same */ }
for _, wc := range cfg.Bee.Platforms.WeCom    { /* same */ }
for _, tc := range cfg.Bee.Platforms.Telegram { /* same */ }
for _, xc := range cfg.Bee.Platforms.Weixin   { /* same */ }
// Linear: no @mentions
ingest := msgingest.New(s.msgStore, cfg.Bee.MessageDebounce, cmdChain,
    msgingest.WithAccountBotNames(accountBotNames))
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/msgingest/ ./internal/app/ -v && go build ./...`
Expected: PASS / build OK.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/msgingest/ internal/app/app.go
git commit -m "refactor(msgingest): strip @mention using per-account bot names"
```

---

## Task 21: Feeder / scheduler / dispatcher carry account through

**Files:**
- Modify: `internal/domain/bee/feeder.go`
- Modify: `internal/domain/task/scheduler.go`
- Modify: `internal/domain/task/dispatcher.go`
- Modify: their tests

- [ ] **Step 1: Write failing test**

In `internal/domain/task/dispatcher_test.go` extend an existing test to assert account propagation from `ClaimedTask` to the dispatch payload:

```go
func TestDispatcher_PropagatesAccount(t *testing.T) {
    // build a ClaimedTask with MessageAccountName="marketing"
    // assert the resulting outbound metadata / failure notifier sees account "marketing"
}
```

(Adapt to the existing test style; the key assertion is that `dispatch.AccountName == "marketing"` if such a field exists, or that any outbound message produced carries that account.)

- [ ] **Step 2: Run test, verify fail**

Run: `go test ./internal/domain/task/ -v`
Expected: FAIL.

- [ ] **Step 3: Implement propagation**

For every spot that constructs an `OutboundMessage` in domain code, populate `AccountName` from the source. Common patterns:

- `OutboundMessage{ ReplyTo: inboundMsg, ... }` — read `inboundMsg.AccountName` into a new `OutboundMessage.AccountName` field at construction time
- `task.DispatchTask` add `AccountName string` field; populate from `ClaimedTask.MessageAccountName`
- `bee.Feeder` reads `account_name` from message rows (now via Task 5) and threads through

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/... -v && go build ./...`
Expected: PASS / build OK.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/bee/ internal/domain/task/
git commit -m "refactor(domain): propagate account_name from inbound to outbound"
```

---

## Task 22: Command handlers `+list`, `+status` keep global worker view

**Files:**
- Modify: `internal/domain/command/list.go`, `status.go` and tests

This task is largely a **verification** — the spec confirms `+list` / `+status` show all workers globally. The change is only to confirm:
- They do NOT filter workers by `msg.AccountName`
- They DO route their reply back via `platform.AccountKey(msg.Platform, msg.AccountName)` (Task 18 already covered the routing)

- [ ] **Step 1: Write reinforcing test**

```go
func TestListCommand_ReturnsAllWorkersRegardlessOfAccount(t *testing.T) {
    // Set up two accounts: feishu:a, feishu:b; one worker total (global)
    // Issue +list from feishu:a; assert reply includes the worker
    // Issue +list from feishu:b; assert reply includes the same worker
}
```

- [ ] **Step 2: Run test, verify it either passes (no filter applied) or fails (filter present)**

Run: `go test ./internal/domain/command/ -run TestListCommand_ReturnsAllWorkers -v`
Expected: PASS if Task 18 wiring is correct; FAIL if any inadvertent account filter slipped in.

- [ ] **Step 3: If failing, remove account filter**

Delete any `WHERE account_name = ?` clause that was added to the worker list query (the worker table does not have that column; if any code tried to filter, drop it).

- [ ] **Step 4: Commit**

```bash
git add internal/domain/command/
git commit -m "test(command): +list returns global worker view across accounts"
```

---

## Task 23: End-to-end integration test — two accounts, one worker

**Files:**
- Create: `internal/app/multi_account_integration_test.go`

- [ ] **Step 1: Write failing test**

```go
//go:build integration

package app

import (
    "context"
    "testing"
    // ... imports ...
)

func TestMultiAccount_FeishuTwoBots_IsolatedSessions(t *testing.T) {
    // 1. Build cfg with feishu = [{name: a}, {name: b}]
    // 2. Spin up two mock receivers — one per account
    // 3. Send the same Content from each through ingest
    // 4. Assert two distinct session rows exist:
    //      session_key like "feishu:a:chat:user", account_name="a"
    //      session_key like "feishu:b:chat:user", account_name="b"
    // 5. Create a single worker
    // 6. Trigger that worker from both accounts; assert two execution rows,
    //    each with the appropriate account_name
    // 7. Assert outbound replies routed through different senders
}
```

- [ ] **Step 2: Run test, verify fail (until everything wired)**

Run: `go test -tags integration ./internal/app/ -run TestMultiAccount -v`
Expected: FAIL at first; PASS once all prior tasks done.

- [ ] **Step 3: Stabilize and pass**

Iterate as needed.

- [ ] **Step 4: Commit**

```bash
git add internal/app/multi_account_integration_test.go
git commit -m "test(integration): multi-account isolation end-to-end"
```

---

## Task 24: Documentation

**Files:**
- Modify: `CHANGELOG.md`
- Modify: `README.md`
- Modify: `README.zh.md`
- Modify: `docs/platform.md` (if it documents config shape)

- [ ] **Step 1: Update CHANGELOG**

Add the next-version entry in English (per user memory rule):

```markdown
## [Unreleased]

### Breaking
- Platform config under `bee.platforms` is now a list per platform. Each entry must include a unique `name`. Existing single-bot configs must be migrated to list form before startup. See `docs/superpowers/specs/2026-05-26-multi-account-per-platform-design.md` for migration guidance.

### Added
- Support N accounts per IM platform (feishu, dingtalk, wecom, weixin, telegram, linear). Sessions, messages, tasks, and executions are isolated per account; workers remain global.
```

- [ ] **Step 2: Update README config example**

Replace the single-platform example with a list example. Mirror in `README.zh.md`.

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md README.md README.zh.md docs/platform.md
git commit -m "docs: announce multi-account per platform support"
```

---

## Task 25: Final verification

- [ ] **Step 1: Full build + test**

```bash
go build ./...
go test ./...
```

Expected: all green.

- [ ] **Step 2: Run with sample config**

Manually start the server with a `config.yaml` containing two feishu accounts (or stubbed) and exercise: send a message via each receiver mock; verify two session rows with distinct `account_name` are created; verify replies go to the correct sender.

- [ ] **Step 3: Final commit if any tweaks**

```bash
git add -A
git commit -m "chore: post-feature cleanup"
```

---

## Self-Review Notes

- Spec section "资源边界" — covered by Tasks 4 (migration), 5-7, 15-17 (stores).
- Spec section "配置 Schema" — Tasks 2, 3.
- Spec section "标识传播 / SessionKey" — Tasks 7-13.
- Spec section "装配" — Tasks 18, 19.
- Spec section "命令可见范围" — Task 22.
- Spec section "迁移" — Tasks 2 (legacy YAML rejection), 4 (DB default).
- Spec section "测试策略" — embedded in every task + Task 23 integration test.
- Local platform (no per-account) — Task 14 (fixed default).

Open items intentionally left for human judgement during implementation:
- Whether `platform_pkg` import cycle exists in `config.go`; fallback is to duplicate the small regex (see Task 2 note).
- Exact field name for `account` in handler structs — use `h.cfg.Name` if cfg is already stored, otherwise add a `name string` field.
