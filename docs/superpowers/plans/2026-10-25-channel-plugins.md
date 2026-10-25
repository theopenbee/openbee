# Channel Plugins Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let chat channels run as external subprocess plugins speaking JSON-RPC over stdio, with a Node SDK and a Discord reference plugin, after first making the Go runtime channel-agnostic.

**Architecture:** Step 0 collapses per-channel wiring in `app.go`, `msgingest`, `worker` and `rpc` into one generic channel list. Step 1 adds `internal/plugin` (manifest, registry, installer) and `internal/platform/pluginchannel` (package `pluginchannel`, a proxy `platform.Platform` that supervises a child process and speaks NDJSON JSON-RPC 2.0). Plugin channel config lives under `bee.platforms.<id>` in `config.yaml`, captured by a yaml inline map.

**Tech Stack:** Go 1.25 (cobra, survey/v2, yaml.v3, zap, testify), Node ≥ 20 + TypeScript (SDK), discord.js v14 (reference plugin), pnpm.

**Spec:** `docs/superpowers/specs/2026-10-24-channel-plugins-design.md`

## Global Constraints

- No code comments except ones the toolchain needs (`//go:build`, `//go:embed`, `// eslint-disable-next-line`, shebangs).
- All text in code is English; user-facing CLI text goes through `internal/infra/i18n` (`locales/en.yaml` + `locales/zh.yaml`).
- `CHANGELOG.md` entries: at most 10 words, English.
- Existing `config.yaml` files must load and behave exactly as before.
- A broken, missing or crashing plugin never blocks openbee startup and never affects other channels.
- Protocol version is the integer `1`; manifest file name is `openbee.plugin.json`.
- Plugin id regex `^[a-z][a-z0-9_-]{1,31}$`; reserved ids: `feishu`, `dingtalk`, `wecom`, `telegram`, `weixin`, `linear`, `local`.
- Reserved config keys owned by the host: `enabled`, `bot_name`.
- Session key for plugin channels: `<id>:<chat_id>:<session_scope>`; `session_scope` defaults to `sender_id`; `chat_id` must not contain `:`.
- Stored dedup id for plugin messages: `<id>:<message_id>`.
- Timeouts: `initialize` 30s; `send` = caller ctx capped at 120s; `shutdown` 5s; stdin-close-to-kill grace 2s.
- Restart backoff: 1s doubling to 60s cap, reset after 10 minutes of uptime.
- Max protocol line: 8 MiB.
- Plugin data root: `~/.openbee/plugins` (`plugins.json`, `node_modules/`, `data/<id>/`). Media dir passed to plugins: `~/.openbee/media/inbound`.

## Review Focus

- Re-running `openbee config` after a plugin was uninstalled or deselected must write its section back unchanged (except `enabled`). Test: Task 10 `TestRenderConfig_PreservesUnknownPluginSection`.
- A plugin (or one of its dependencies) printing non-JSON to stdout must not break the connection. Tests: Task 5 `TestConn_IgnoresGarbageLines`; Task 11 `console.log is redirected to stderr`.
- openbee killed with SIGKILL must not leave a plugin connected to the bot. Tests: Task 7 `TestSupervisor_PluginExitsWhenStdinCloses`; Task 11 `exits when stdin closes`.
- `enabled: true` for a plugin that is not installed, or whose required field is missing, must log and skip only that channel. Test: Task 8 `TestBuildChannels_SkipsBrokenSections`.
- Agent replies longer than the platform's message limit and attachments over the size limit. Tests: Task 12 `chunkText splits at 2000`, `oversized attachment becomes placeholder`; Task 6 `TestToInbound_AttachmentOutsideMediaDir`.

---

## Step 0 — runtime decoupling

### Task 1: Generic channel wiring and bot names

**Files:**
- Create: `internal/platform/channel.go`
- Modify: `internal/infra/config/config.go` (`PlatformsConfig.BotNames`, new `BotNamesByPlatform`)
- Modify: `internal/app/app.go` (`buildPlatforms`, platform loops, `WithPlatformBotNames` call)
- Test: `internal/infra/config/config_bee_test.go`

**Interfaces:**
- Produces:
  - `type platform.Channel struct { Platform platform.Platform; ExtractContext func(raw string) string }`
  - `func (p config.PlatformsConfig) BotNamesByPlatform() map[string]string` — keys are platform ids, values non-empty bot names, regardless of `enabled`.
  - `func buildPlatforms(bc config.BeeConfig, mediaSvc *media.Service, tasks *store.TaskStore) ([]platform.Channel, error)` in `app.go`.

- [ ] **Step 1: Write the failing test** in `config_bee_test.go`

```go
func TestPlatformsConfig_BotNamesByPlatform(t *testing.T) {
	p := PlatformsConfig{}
	p.Feishu.BotName = "FeishuBot"
	p.Telegram.BotName = "TgBot"
	p.Telegram.Enabled = false
	got := p.BotNamesByPlatform()
	assert.Equal(t, map[string]string{"feishu": "FeishuBot", "telegram": "TgBot"}, got)
	assert.ElementsMatch(t, []string{"FeishuBot", "TgBot"}, p.BotNames())
}
```

- [ ] **Step 2: Run** `go test ./internal/infra/config -run TestPlatformsConfig_BotNamesByPlatform` — Expected: FAIL (`BotNamesByPlatform` undefined).

- [ ] **Step 3: Implement** `BotNamesByPlatform` covering feishu, dingtalk, wecom, telegram, weixin (Linear has no bot name); use literal ids `"feishu"`, `"dingtalk"`, `"wecom"`, `"telegram"`, `"weixin"` (config must not import platform packages). Reimplement `BotNames()` as the values of `BotNamesByPlatform()`.

- [ ] **Step 4: Run** the test — Expected: PASS.

- [ ] **Step 5: Refactor `app.go`**
  - Add `platform.Channel` in `internal/platform/channel.go`.
  - Hoist `mediaSvc := media.NewService()` into `BuildApp`; `buildPlatforms` takes `cfg.Bee`, `mediaSvc`, `s.taskStore` and returns `[]platform.Channel` (feishu/dingtalk/wecom carry their `ExtractContext`; others nil).
  - One loop over channels: `if ch.ExtractContext != nil { platform.RegisterExtractor(id, ch.ExtractContext) }`, wrap sender with `store.NewLoggingPlatformSenderAdapter`, append receiver runner.
  - Replace the hand-written bot-name map with `msgingest.WithPlatformBotNames(cfg.Bee.Platforms.BotNamesByPlatform())`.
  - Remove imports of platform packages that become unused.

- [ ] **Step 6: Verify** `go build ./... && go vet ./internal/... && go test ./...` — Expected: all PASS; `grep -n 'feishu\.\|dingtalk\.\|wecom\.\|telegram\.\|weixin\.\|linear\.' internal/app/app.go` only matches inside `buildPlatforms`.

- [ ] **Step 7: Commit** `git commit -m "refactor(app): wire channels through a generic list"`

### Task 2: Replace the Linear send special case with `CombinedSender`

**Files:**
- Modify: `internal/platform/interfaces.go`
- Modify: `internal/platform/linear/handler.go` (`LinearSender`)
- Modify: `internal/infra/store/logging_sender_adapter.go`
- Modify: `internal/rpc/tools.go` (`toolSendMessage`)
- Test: `internal/rpc/tools_test.go`, `internal/infra/store/logging_sender_adapter_test.go` (create if absent)

**Interfaces:**
- Produces:
  - `type platform.CombinedSender interface { SendsTextWithMedia() bool }`
  - `func platform.SendsTextWithMedia(s platform.PlatformSenderAdapter) bool` — true iff `s` implements `CombinedSender` and returns true.
  - `(*store.LoggingPlatformSenderAdapter).SendsTextWithMedia() bool` delegates to `platform.SendsTextWithMedia(inner)`.

- [ ] **Step 1: Write failing tests**
  - In `tools_test.go`, add `combinedMockSender` (embeds `mockSender`, `SendsTextWithMedia() bool { return true }`). Change case `LinearContentAndMediaSentTogether` to register `combinedMockSender` for `"linear"`. Add case `NonCombinedSenderSplitsTextAndMedia`: feishu message, `content:"a"`, `media_path:"/tmp/x.png"` → `require.Len(t, sent, 2)`, `sent[0].Content == "a"`, `sent[1].MediaPath == "/tmp/x.png"`. Add case `CombinedSenderOnAnyPlatform`: platform `"discord"` with `combinedMockSender` → `require.Len(t, sent, 1)`.
  - `TestLoggingSenderAdapter_SendsTextWithMedia`: wrapping a combined sender → true; wrapping a plain sender → false.

- [ ] **Step 2: Run** `go test ./internal/rpc ./internal/infra/store` — Expected: FAIL (`CombinedSenderOnAnyPlatform` sends twice; adapter method undefined).

- [ ] **Step 3: Implement.** `LinearSender` gets `SendsTextWithMedia() bool { return true }`; `toolSendMessage` uses `platform.SendsTextWithMedia(sender)` instead of comparing `base.ReplyTo.Platform == linear.PlatformID`; drop the `linear` import from `rpc`.

- [ ] **Step 4: Run** `go test ./...` — Expected: PASS; `grep -rn 'platform/linear' internal/rpc` → no matches.

- [ ] **Step 5: Commit** `git commit -m "refactor(rpc): let senders opt into combined text and media"`

---

## Step 1 — plugin core

### Task 3: Manifest parsing and plugin config resolution

**Files:**
- Create: `internal/plugin/manifest.go`, `internal/plugin/config.go`
- Test: `internal/plugin/manifest_test.go`, `internal/plugin/config_test.go`

**Interfaces:**
- Produces:
  - `const ManifestFile = "openbee.plugin.json"`, `const ProtocolVersion = 1`
  - `var ReservedIDs = []string{"feishu", "dingtalk", "wecom", "telegram", "weixin", "linear", "local"}`
  - `type LocalizedText map[string]string` with `UnmarshalJSON` accepting a JSON string (stored under `"en"`) or an object; `func (t LocalizedText) Get(lang string) string` → `t[lang]`, else `t["en"]`, else `""`.
  - `type FieldType string` with `FieldString="string"`, `FieldSecret="secret"`, `FieldBool="bool"`, `FieldInt="int"`, `FieldList="list"`.
  - `type ConfigField struct { Key string; Type FieldType; Required bool; Default any; Label LocalizedText; Help LocalizedText }` (json tags snake_case).
  - `type Manifest struct { ProtocolVersion int; ID string; Name LocalizedText; Description LocalizedText; Command []string; Config []ConfigField; Version string; Dir string }` (`Dir` not serialized).
  - `func LoadManifest(dir string) (Manifest, error)` — reads `dir/openbee.plugin.json`, sets `Dir` to the absolute dir, fills `Version` from `dir/package.json` `version` when present, then `Validate()`.
  - `func (m Manifest) Validate() error`
  - `func (m Manifest) DisplayName(lang string) string` — `Name.Get(lang)`, falling back to `ID`.
  - `func ValidateID(id string) error` — regex + reserved check.
  - `func ResolveConfig(m Manifest, section map[string]any) (map[string]any, error)` — drops `enabled`/`bot_name`, applies defaults, enforces `required` (missing or empty string/list), coerces types (`int` accepts int/int64/float64 with integral value; `list` accepts `[]any` of strings and returns `[]string`), passes unknown keys through unchanged.

- [ ] **Step 1: Write failing tests**
  - `TestLoadManifest_Valid`: temp dir with manifest + `package.json` `{"version":"0.3.1"}` → `ID=="discord"`, `Version=="0.3.1"`, `Name.Get("zh")=="Discord"`, `Dir` absolute.
  - `TestLocalizedText_StringOrObject`: `"Bot token"` → `Get("zh")=="Bot token"`; `{"en":"A","zh":"甲"}` → `Get("zh")=="甲"`, `Get("fr")=="A"`.
  - `TestManifestValidate` table: wrong `protocol_version` (2) → error mentions `protocol_version`; id `"Discord"` → error; id `"telegram"` → error mentions `reserved`; empty `command` → error; field key `"enabled"` or `"bot_name"` → error; duplicate field keys → error; unknown field type `"float"` → error.
  - `TestResolveConfig` table: defaults applied (`max_media_size` default `10485760` appears as `int`); required secret missing → error naming `token`; `list` from `[]any{"1","2"}` → `[]string{"1","2"}`; `int` given `"abc"` → error; `enabled`/`bot_name` removed; unknown key `extra:"x"` kept.

- [ ] **Step 2: Run** `go test ./internal/plugin` — Expected: FAIL (package missing).
- [ ] **Step 3: Implement** the produced API.
- [ ] **Step 4: Run** `go test ./internal/plugin` — Expected: PASS.
- [ ] **Step 5: Commit** `git commit -m "feat(plugin): parse channel plugin manifests"`

### Task 4: Plugin registry and installer

**Files:**
- Create: `internal/plugin/registry.go`, `internal/plugin/installer.go`
- Test: `internal/plugin/registry_test.go`, `internal/plugin/installer_test.go`

**Interfaces:**
- Consumes: `LoadManifest`, `ValidateID` (Task 3).
- Produces:
  - `func DefaultDir() string` → `~/.openbee/plugins`
  - `type Entry struct { ID string; Source string; Package string; Path string; Version string }` — `Source` is `"npm"` or `"path"`; `Package` set for npm.
  - `type Registry struct{ dir string }`; `func OpenRegistry(dir string) *Registry`; `func (r *Registry) Dir() string`; `List() ([]Entry, error)` sorted by ID; `Get(id string) (Entry, bool, error)`; `Put(e Entry) error`; `Remove(id string) error`; `DataDir(id string) string` → `dir/data/<id>` (created on demand by the caller).
  - File `dir/plugins.json`: `{"plugins":{"<id>":{...entry}}}`, written atomically (temp file + rename), mode 0600, dir mode 0700.
  - `type Runner func(ctx context.Context, dir string, name string, args ...string) ([]byte, error)`; `func ExecRunner(...)` real implementation using `exec.CommandContext` with combined output.
  - `type Installer struct { Registry *Registry; Run Runner }`
  - `func (i *Installer) Install(ctx context.Context, spec string) (Entry, Manifest, error)`
  - `func (i *Installer) Remove(ctx context.Context, id string) (Entry, error)`
  - `var ErrNpmNotFound = errors.New("npm not found on PATH")`; `var ErrNotInstalled = errors.New("plugin not installed")`

- [ ] **Step 1: Write failing tests**
  - `TestRegistry_PutGetListRemove`: put two entries → `List` sorted, file mode `0600`, `Get` round-trips, `Remove` then `Get` → not found.
  - `TestInstaller_LocalPath`: spec is a temp dir with a valid manifest → entry `{Source:"path", Path:<abs>}`; runner never called.
  - `TestInstaller_Npm`: fake runner records calls; for `npm pack --dry-run --json <spec>` returns `[{"name":"@x/openbee-discord","version":"0.1.0"}]`; for `npm install --save <spec>` writes a manifest into `dir/node_modules/@x/openbee-discord/`. Assert call order (`pack` then `install`), `dir/package.json` created with `"private": true`, entry `{Source:"npm", Package:"@x/openbee-discord", Path:dir/node_modules/@x/openbee-discord}`.
  - `TestInstaller_RejectsConflicts`: manifest id `telegram` → error mentions `reserved`; id already registered from a different path → error mentions `already installed`; same path re-install → succeeds (upgrade).
  - `TestInstaller_RemoveNpm`: runner receives `npm uninstall --save @x/openbee-discord`; entry gone. Remove unknown id → `errors.Is(err, ErrNotInstalled)`.
  - `TestInstaller_NpmMissing`: runner returns `exec.ErrNotFound` → `errors.Is(err, ErrNpmNotFound)`.

- [ ] **Step 2: Run** `go test ./internal/plugin -run 'Registry|Installer'` — Expected: FAIL.
- [ ] **Step 3: Implement.** Spec is a local path iff `os.Stat(spec)` succeeds and is a dir. On manifest validation failure after an npm install, uninstall the package again before returning the error.
- [ ] **Step 4: Run** `go test ./internal/plugin` — Expected: PASS.
- [ ] **Step 5: Commit** `git commit -m "feat(plugin): add plugin registry and installer"`

### Task 5: NDJSON JSON-RPC connection

**Files:**
- Create: `internal/platform/pluginchannel/rpc.go`
- Test: `internal/platform/pluginchannel/rpc_test.go`

**Interfaces:**
- Produces:
  - `type RPCError struct { Code int; Message string }` implementing `error`; `const CodeConfigInvalid = -32001`, `CodeMethodNotFound = -32601`.
  - `type NotificationHandler func(method string, params json.RawMessage)`
  - `func NewConn(r io.Reader, w io.Writer, onNotify NotificationHandler, log *logger.Logger) *Conn`
  - `func (c *Conn) Run() error` — read loop; returns nil on EOF; on exit fails all pending calls with `ErrConnClosed`.
  - `func (c *Conn) Call(ctx context.Context, method string, params, result any) error` — on ctx done sends notification `$/cancelRequest` `{"id": <id>}` and returns `ctx.Err()`.
  - `func (c *Conn) Notify(method string, params any) error`
  - `var ErrConnClosed = errors.New("plugin connection closed")`
  - Incoming requests (messages with `id` and `method`) are answered with `CodeMethodNotFound`.

- [ ] **Step 1: Write failing tests** (use `io.Pipe` pairs; a test peer goroutine reads/writes lines)
  - `TestConn_CallRoundTrip`: peer answers `{"jsonrpc":"2.0","id":1,"result":{"ok":true}}` → result decoded.
  - `TestConn_CallError`: peer answers error `-32001 "bad token"` → `errors.As(err, &rpcErr)` with `Code == CodeConfigInvalid`.
  - `TestConn_CancelSendsCancelRequest`: ctx cancelled before reply → `Call` returns `context.Canceled`; peer receives `{"method":"$/cancelRequest","params":{"id":1}}`.
  - `TestConn_IgnoresGarbageLines`: peer writes `hello world\n` then a valid response → call succeeds.
  - `TestConn_DispatchesNotifications`: peer writes `{"jsonrpc":"2.0","method":"inbound","params":{"chat_id":"1"}}` → handler called with method `inbound`.
  - `TestConn_OversizedLineSkipped`: a 9 MiB line followed by a valid response → call succeeds.
  - `TestConn_EOFFailsPending`: peer closes → `Call` returns `ErrConnClosed`.
  - `TestConn_RejectsPeerRequests`: peer sends `{"id":7,"method":"x"}` → peer receives error response id 7 code `-32601`.

- [ ] **Step 2: Run** `go test ./internal/platform/pluginchannel -run TestConn` — Expected: FAIL.
- [ ] **Step 3: Implement.** Use `bufio.Reader` + manual line accumulation so oversized lines can be discarded without aborting the loop (not `bufio.Scanner`). Writes are serialized with a mutex; ids are increasing int64.
- [ ] **Step 4: Run** `go test -race ./internal/platform/pluginchannel -run TestConn` — Expected: PASS.
- [ ] **Step 5: Commit** `git commit -m "feat(plugin): add NDJSON JSON-RPC connection"`

### Task 6: Protocol types, session keys, raw envelope, inbound conversion

**Files:**
- Create: `internal/platform/pluginchannel/protocol.go`, `internal/platform/pluginchannel/session.go`, `internal/platform/pluginchannel/envelope.go`, `internal/platform/pluginchannel/inbound.go`
- Modify: `internal/infra/media/service.go` (add `InboundDir()`)
- Test: `internal/platform/pluginchannel/session_test.go`, `envelope_test.go`, `inbound_test.go`

**Interfaces:**
- Produces (json tags exactly as the spec §6.6, snake_case, `omitempty` on optional fields):
  - `InitializeParams{ProtocolVersion int; OpenbeeVersion, ChannelID, Language, BotName string; Config map[string]any; DataDir, MediaDir string}`, `InitializeResult{ProtocolVersion int}`
  - `SendParams{SessionKey, ChatID, SessionScope string; Proactive bool; ReplyTo *ReplyTo; Content string; Media *Media}`, `ReplyTo{MessageID, Raw string}`, `Media{Path, Name string}`, `SendResult{PlatformMessageID string}`
  - `InboundParams{ChatID, SenderID, SessionScope, MessageID, Text string; Attachments []Attachment; Raw string; Context json.RawMessage; Time int64}`, `Attachment{Type, Path, Name string}`
  - `LogParams{Level, Message string; Fields map[string]any}`
  - `func BuildSessionKey(channelID, chatID, scope string) string`
  - `func SplitSessionKey(channelID, key string) (chatID, scope string, err error)` — scope may itself contain `:`.
  - `func EncodeEnvelope(raw, messageID string, ctx json.RawMessage) string` → `{"v":1,...}`
  - `func DecodeEnvelope(s string) (raw, messageID string, ctx json.RawMessage)` — invalid/empty input returns zero values.
  - `func ExtractContextFor(channelID string) func(raw string) string` — `platform.MarshalContext(channelID, ctx)` when ctx present, else `""`.
  - `func ToInbound(channelID string, mediaSvc *media.Service, p InboundParams) (platform.InboundMessage, error)`
  - `func (s *media.Service) InboundDir() string`

- [ ] **Step 1: Write failing tests**
  - `TestSessionKey_RoundTrip`: `BuildSessionKey("discord","123","456")=="discord:123:456"`; `SplitSessionKey("discord","discord:123:t9:456")` → `("123","t9:456",nil)`; wrong prefix → error.
  - `TestEnvelope_RoundTrip` and `TestEnvelope_DecodeGarbage` (`"not json"` → zero values).
  - `TestExtractContextFor`: envelope with ctx `{"guild_id":"9"}` → `{"discord":{"guild_id":"9"}}`; envelope without ctx → `""`.
  - `TestToInbound_Basic`: `{chat_id:"1", sender_id:"2", message_id:"m", text:"hi", raw:"r", time:5}` → `Platform=="discord"`, `SessionKey=="discord:1:2"`, `SenderID=="2"`, `PlatformMessageID=="discord:m"`, `Content=="hi"`, `RawContent=="hi"`, `MessageTime==5`, decoded envelope raw `"r"`/message id `"m"`.
  - `TestToInbound_SessionScope`: `session_scope:"issue-7"` → `SessionKey=="discord:1:issue-7"`.
  - `TestToInbound_Validation` table: empty `chat_id`, `chat_id` with `:`, empty `sender_id`, empty text and no attachments → error.
  - `TestToInbound_Attachments`: file inside `InboundDir()` with `type:"image"`, `name:"a.png"` → `Content == "hi\n" + mediaSvc.BuildPlaceholder("image", path, "a.png")`.
  - `TestToInbound_AttachmentOutsideMediaDir`: path `/etc/passwd` → content contains `BuildPlaceholder("document", "", "passwd")` and no `/etc/passwd`; unknown `type` normalizes to `document`.
  - `MessageTime == 0` when `time` absent (ingest fills server time).

- [ ] **Step 2: Run** `go test ./internal/platform/pluginchannel` — Expected: FAIL.
- [ ] **Step 3: Implement.** Attachment path check: `filepath.Rel(InboundDir, filepath.Clean(path))` must not start with `..` and path must be absolute. Multiple placeholders are joined with `\n`; with empty text, content is just the placeholders.
- [ ] **Step 4: Run** `go test ./internal/platform/pluginchannel ./internal/infra/media` — Expected: PASS.
- [ ] **Step 5: Commit** `git commit -m "feat(plugin): map plugin protocol messages to platform messages"`

### Task 7: Process supervisor

**Files:**
- Create: `internal/platform/pluginchannel/supervisor.go`
- Test: `internal/platform/pluginchannel/supervisor_test.go`, `internal/platform/pluginchannel/helper_test.go` (fake plugin)

**Interfaces:**
- Consumes: `NewConn`, `Call`, `RPCError`, `CodeConfigInvalid` (Task 5); `InitializeParams`, `InitializeResult`, `LogParams` (Task 6); `ai.ConfigureCmd`, `ai.NewCmdProcess` (existing).
- Produces:
  - `type Spec struct { ChannelID string; Dir string; Command []string; Env []string; Init InitializeParams }`
  - `type Supervisor struct{...}`; `func NewSupervisor(spec Spec, onNotify NotificationHandler) *Supervisor`
  - `func (s *Supervisor) Run(ctx context.Context)` — blocks until ctx is done and the child is stopped, or until a permanent failure.
  - `func (s *Supervisor) Call(ctx context.Context, method string, params, result any) error` — `ErrNotRunning` when no initialized connection.
  - `func (s *Supervisor) Done() <-chan struct{}` — closed when `Run` returns.
  - `var ErrNotRunning = errors.New("plugin channel is not running")` (wrap with channel id: `fmt.Errorf("channel %s: %w", id, ErrNotRunning)`).
  - Unexported knobs for tests: `initialBackoff`, `maxBackoff`, `stableAfter`, `shutdownTimeout`, `killGrace`.

Behavior (spec §6.5):
- Resolve `Command[0]`: contains a path separator or starts with `.` → `filepath.Join(Dir, Command[0])`; else `exec.LookPath`. Not found → permanent failure.
- `cmd.Dir = Dir`; env = `os.Environ()` + `Env`; `ai.ConfigureCmd(cmd)`; stdout → `Conn`; stderr lines → `logger.With(zap.String("component", "plugin:"+id)).Info(line)`.
- `log` notifications are written to that logger at their level; other notifications go to `onNotify`.
- `initialize` (30s). `RPCError` with `CodeConfigInvalid` or result protocol version ≠ 1 → permanent failure (log error, no restart).
- Unexpected exit → restart after backoff; backoff resets when the previous run lasted ≥ `stableAfter`.
- ctx done → `shutdown` call (5s), close stdin, wait `killGrace` (2s), then `ai.NewCmdProcess(cmd).Stop()`.

- [ ] **Step 1: Write the fake plugin** in `helper_test.go`: `TestMain` runs `fakePluginMain()` and exits when env `OPENBEE_FAKE_PLUGIN` is set. Modes via `FAKE_PLUGIN_MODE`: `ok` (answers initialize, echoes `send` back as an `inbound` notification, answers shutdown, exits on stdin EOF), `config_invalid`, `bad_protocol` (returns `protocol_version: 2`), `crash_after_init` (exits 1 right after replying), `ignore_shutdown` (never exits on its own), `count_starts` (appends a line to file `FAKE_PLUGIN_COUNTER` each start, then behaves like `crash_after_init`). Spec command for tests: `[]string{os.Args[0], "-test.run=^$"}`.

- [ ] **Step 2: Write failing tests**
  - `TestSupervisor_InitializeAndCall`: mode `ok` → `Call(ctx,"send",...)` succeeds and `onNotify` receives `inbound`.
  - `TestSupervisor_ConfigInvalidIsPermanent`: mode `config_invalid` → `Run` returns by itself within 2s; `Call` → `ErrNotRunning`.
  - `TestSupervisor_ProtocolMismatchIsPermanent`: mode `bad_protocol` → same as above.
  - `TestSupervisor_MissingCommandIsPermanent`: command `["definitely-not-a-binary-xyz"]` → `Run` returns.
  - `TestSupervisor_RestartsWithBackoff`: mode `count_starts`, `initialBackoff=10ms`, `maxBackoff=40ms` → counter file has ≥ 3 lines within 1s.
  - `TestSupervisor_CallWhileDown`: before initialize completes / after crash → `errors.Is(err, ErrNotRunning)`.
  - `TestSupervisor_ShutdownKillsStubbornPlugin` (unix only via `//go:build !windows` file or `runtime.GOOS` skip): mode `ignore_shutdown`, `shutdownTimeout=100ms`, `killGrace=100ms` → after cancel, `Done()` closes within 1s and the child pid no longer exists (`syscall.Kill(pid, 0)` returns `ESRCH`).
  - `TestSupervisor_PluginExitsWhenStdinCloses`: mode `ok`; close the supervisor's stdin writer directly (test hook) → child exits with code 0.

- [ ] **Step 3: Run** `go test -race ./internal/platform/pluginchannel -run TestSupervisor` — Expected: FAIL.
- [ ] **Step 4: Implement** `supervisor.go`.
- [ ] **Step 5: Run** `go test -race ./internal/platform/pluginchannel` — Expected: PASS (run 3 times with `-count=3` to catch flakes).
- [ ] **Step 6: Commit** `git commit -m "feat(plugin): supervise channel plugin processes"`

### Task 8: Proxy platform, config capture, app wiring, graceful shutdown

**Files:**
- Create: `internal/platform/pluginchannel/platform.go`, `internal/platform/pluginchannel/build.go`
- Modify: `internal/infra/config/config.go` (`PlatformsConfig.Plugins`, `BotNamesByPlatform`)
- Modify: `internal/app/app.go` (`BuildApp`, `App`, `App.Run`)
- Test: `internal/platform/pluginchannel/platform_test.go`, `internal/platform/pluginchannel/build_test.go`, `internal/infra/config/config_bee_test.go`

**Interfaces:**
- Consumes: Tasks 3–7; `platform.Channel` (Task 1); `platform.CombinedSender` (Task 2); `buildinfo.Version`.
- Produces:
  - `PlatformsConfig.Plugins map[string]map[string]any` with tag `yaml:",inline"`.
  - `BotNamesByPlatform` also includes `Plugins[id]["bot_name"]` when it is a non-empty string (enabled or not).
  - `type Platform struct{...}`; `func New(spec Spec, mediaSvc *media.Service) *Platform`; methods `ID() string`, `Receiver() platform.PlatformReceiverAdapter`, `Sender() platform.PlatformSenderAdapter`, `Done() <-chan struct{}`.
    - Receiver `Start(ctx, dispatch)` runs the supervisor; `inbound` notifications go through `ToInbound` then `dispatch`; invalid ones are logged at warn and dropped.
    - Sender `Send(ctx, msg)`: session key = `msg.ReplyTo.SessionKey` or `msg.SessionKey`; `SplitSessionKey`; reply raw/message id from `DecodeEnvelope(msg.ReplyTo.Raw)` (omit `reply_to` when both empty); `Media{Path: msg.MediaPath, Name: filepath.Base}`; ctx capped with `context.WithTimeout(ctx, 120*time.Second)`. Sender implements `SendsTextWithMedia() bool { return true }`.
  - `type BuildOptions struct { Registry *plugin.Registry; MediaSvc *media.Service; Language string }`
  - `func BuildChannels(sections map[string]map[string]any, opts BuildOptions) []BuiltChannel` where `type BuiltChannel struct { Channel platform.Channel; Done <-chan struct{} }`, sorted by id. A section is enabled iff `section["enabled"] == true`.
  - `App` gains `waitOnShutdown []<-chan struct{}`; `App.Run` waits for them in parallel with `server.Shutdown`, bounded by the existing 15s `shutdownCtx`, before `db.Close()`.

- [ ] **Step 1: Write failing tests**
  - `TestPlatformsConfig_InlinePlugins` (config): YAML with `feishu` + `discord: {enabled: true, bot_name: DBot, token: x}` → `Feishu` typed fields intact, `Plugins["discord"]["token"]=="x"`, `Plugins` has no `feishu` key, `BotNamesByPlatform()["discord"]=="DBot"`; a disabled plugin section's bot name is included too.
  - `TestBuildChannels_SkipsBrokenSections`: registry with valid plugin `good`; sections `good` (enabled, required field present), `missing` (enabled, not installed), `incomplete` (installed, required field absent), `off` (enabled:false) → returns exactly one channel with `ID()=="good"`; no panic.
  - `TestPlatform_SendMapsOutbound`: proxy using fake plugin mode `ok` with echo of the `send` params; reply message whose `ReplyTo.Raw` is an envelope (`raw:"r1"`, message id `"m1"`), session key `good:c1:u1`, content + media → plugin received `chat_id:"c1"`, `session_scope:"u1"`, `reply_to:{message_id:"m1",raw:"r1"}`, `content`, `media.name == base(path)`; exactly one `send` call.
  - `TestPlatform_InboundDispatch`: fake plugin emits inbound `{chat_id:"c1",sender_id:"u1",text:"hi",message_id:"m9"}` → dispatched `InboundMessage` has `SessionKey=="good:c1:u1"`, `PlatformMessageID=="good:m9"`.
  - `TestPlatform_SendWhenDown` → error wraps `ErrNotRunning`.

- [ ] **Step 2: Run** `go test ./internal/platform/pluginchannel ./internal/infra/config` — Expected: FAIL.
- [ ] **Step 3: Implement** `platform.go`, `build.go`, config changes. In `BuildChannels`, for each enabled section: `Registry.Get` → `LoadManifest(entry.Path)` → `ResolveConfig` → create `Registry.DataDir(id)` (0700) → `Spec{ChannelID:id, Dir:manifest.Dir, Command:manifest.Command, Init: InitializeParams{ProtocolVersion:1, OpenbeeVersion: buildinfo.Version, ChannelID:id, Language, BotName: section bot_name, Config: resolved, DataDir, MediaDir: mediaSvc.InboundDir()}}`. Each failure: `logger.Error` with channel id and reason, then continue.
- [ ] **Step 4: Wire `app.go`**: after built-in channels, `pluginchannel.BuildChannels(cfg.Bee.Platforms.Plugins, ...)` with `plugin.OpenRegistry(plugin.DefaultDir())`; append channels to the generic loop (`ExtractContext` = `ExtractContextFor(id)`); collect `Done` channels into `App.waitOnShutdown`. Update `App.Run`.
- [ ] **Step 5: Run** `go build ./... && go test -race ./...` — Expected: PASS.
- [ ] **Step 6: Commit** `git commit -m "feat(plugin): run channel plugins as platforms"`

---

## Step 1 — CLI

### Task 9: `openbee plugin` commands

**Files:**
- Create: `cmd/openbee/internal/cli/plugincmd/plugincmd.go`
- Modify: `cmd/openbee/internal/cli/root.go` (register command)
- Modify: `internal/infra/i18n/messages.go`, `internal/infra/i18n/locales/en.yaml`, `internal/infra/i18n/locales/zh.yaml`
- Test: `cmd/openbee/internal/cli/plugincmd/plugincmd_test.go`, `internal/infra/i18n/i18n_test.go` (existing key-completeness test must pass)

**Interfaces:**
- Consumes: `plugin.Installer`, `plugin.Registry`, `plugin.ExecRunner`, `plugin.DefaultDir`, `Manifest.DisplayName` (Tasks 3–4); `config.Load`, `config.GetLang`.
- Produces:
  - `func NewCommand() *cobra.Command` with `install <spec|path>`, `list`, `remove <id>`; persistent flag `-c/--config` default `config.yaml`.
  - `func newCommand(deps deps) *cobra.Command` with `type deps struct { Installer *plugin.Installer; Out io.Writer }` for tests.
  - i18n: `CmdMessages.Plugin CmdEntry` (`yaml:"plugin"`, subs `install`, `list`, `remove`); `OutputMessages.Plugin PluginOutput` (`yaml:"plugin"`) with keys `installed` (`%s %s`), `trust_warning`, `next_steps`, `list_header`, `list_empty`, `removed` (`%s`), `still_enabled` (`%s`).

- [ ] **Step 1: Write failing tests** (fake `Runner`, temp `HOME`-independent registry dir, temp config file)
  - `TestPluginInstall_LocalPath`: output contains plugin id, version, trust warning and next-steps text.
  - `TestPluginList`: two entries; config enables one → output rows show id, name, version, source, path and `enabled`/`disabled`.
  - `TestPluginList_Empty` → `list_empty` text.
  - `TestPluginRemove_WarnsWhenStillEnabled`: section `enabled: true` → output contains `still_enabled` text; entry removed.
- [ ] **Step 2: Run** `go test ./cmd/openbee/internal/cli/plugincmd` — Expected: FAIL.
- [ ] **Step 3: Implement** commands and i18n entries (zh copy is user-facing Chinese; en copy English).
- [ ] **Step 4: Run** `go test ./cmd/... ./internal/infra/i18n` — Expected: PASS; `go run ./cmd/openbee plugin --help` lists the three subcommands.
- [ ] **Step 5: Commit** `git commit -m "feat(cli): add plugin install, list and remove"`

### Task 10: `openbee config` wizard support for plugin channels

**Files:**
- Create: `cmd/openbee/internal/cli/configcmd/plugins.go`
- Modify: `cmd/openbee/internal/cli/configcmd/values.go` (`configValues.PluginSections`, `configRenderData.PluginPlatformsYAML`, `loadExistingConfig`)
- Modify: `cmd/openbee/internal/cli/configcmd/platforms.go` (`runPlatformStep`)
- Modify: `cmd/openbee/internal/cli/configcmd/command.go` (render data)
- Modify: `internal/infra/config/config.yaml.tmpl` (after the `linear` block: `{{- if .PluginPlatformsYAML}}` newline `{{.PluginPlatformsYAML}}{{end}}`, emitting nothing when there are no plugin sections)
- Modify: i18n files (`prompt.plugin_suffix`, `prompt.plugin_list_hint` for list inputs)
- Test: `cmd/openbee/internal/cli/configcmd/plugins_test.go`

**Interfaces:**
- Consumes: `plugin.Registry.List`, `plugin.LoadManifest`, `Manifest.DisplayName`, `ConfigField` (Task 3).
- Produces:
  - `configValues.PluginSections map[string]map[string]any` (copied from `cfg.Bee.Platforms.Plugins` in `loadExistingConfig`).
  - `func renderPluginSections(sections map[string]map[string]any) (string, error)` — ids sorted; each section rendered as YAML indented by 4 spaces under its id (`    <id>:` then 6-space keys); empty map → `""`.
  - `func renderConfig(vals configValues) ([]byte, error)` extracted from `runConfig` so tests can render the template.
  - `type pluginOption struct { Manifest plugin.Manifest; Label string }`; `func loadPluginOptions(reg *plugin.Registry, lang string) []pluginOption` (skips entries whose manifest fails to load).
  - `func promptPluginSection(m plugin.Manifest, lang string, current map[string]any) (map[string]any, error)` — survey prompts per field type (`secret` → `survey.Password`, keeping the current value when left empty; `bool` → `Confirm`; `int` → `Input` with integer validator; `list` → comma-separated `Input`; `string` → `Input`), `Required` → `survey.Required`; then `promptBotName`; sets `enabled: true`.

- [ ] **Step 1: Write failing tests**
  - `TestRenderConfig_WithPluginSection`: vals with feishu enabled + `PluginSections{"discord":{enabled:true, bot_name:"DBot", token:"x", allowed_guilds:[]string{"1"}}}` → `config.Load` on the rendered file gives `Plugins["discord"]["token"]=="x"`, `Feishu.Enabled==true`, and the `bee.feeder` block still parses.
  - `TestRenderConfig_PreservesUnknownPluginSection`: load a config containing section `ghost` (not installed) → render → reload → `Plugins["ghost"]` deep-equals the original.
  - `TestRenderPluginSections_Empty` → `""`; rendering a config without plugins has `max_media_size:` of the linear block immediately followed by the `  feeder:` line (no blank line inserted).
  - `TestRunPlatformStep_DeselectedPluginKeepsValues`: call the pure helper used by `runPlatformStep` to apply a selection (`applyPluginSelection(vals, selectedIDs)`) → deselected section has `enabled:false` and keeps `token`.
- [ ] **Step 2: Run** `go test ./cmd/openbee/internal/cli/configcmd` — Expected: FAIL.
- [ ] **Step 3: Implement.** In `runPlatformStep`, options = built-in labels + one option per installed plugin labelled `DisplayName(lang) + i18n.M.Prompt.PluginSuffix`; defaults include plugins whose section is enabled. Prompting happens only for selected plugins.
- [ ] **Step 4: Run** `go test ./cmd/... ./internal/...` — Expected: PASS.
- [ ] **Step 5: Commit** `git commit -m "feat(config): configure plugin channels in the wizard"`

---

## Step 1 — SDK, reference plugin, docs

### Task 11: Node/TS channel SDK

**Files:**
- Create: `sdk/channel-node/package.json`, `tsconfig.json`, `src/index.ts`, `src/rpc.ts`, `src/types.ts`, `test/sdk.test.mjs`, `test/fixtures/echo-plugin.mjs`, `README.md`, `.gitignore` (`node_modules/`, `dist/`)

**Interfaces:**
- Produces (package `@theopenbee/channel-sdk`, `"type": "module"`, `"private": true` until publishing is approved, `engines.node >= 20`, build `tsc` → `dist/`, test `node --test test/`):
  - Types `InboundMessage`, `Attachment`, `SendRequest`, `SendResult` mirroring spec §6.6 field names (snake_case).
  - `interface ChannelContext { channelId: string; config: Record<string, unknown>; botName: string; language: string; dataDir: string; mediaDir: string; openbeeVersion: string; dispatch(msg: InboundMessage): void; saveMedia(data: Uint8Array, ext: string): Promise<string>; log: Logger }`
  - `interface Logger { debug(msg: string, fields?: Record<string, unknown>): void; info(...): void; warn(...): void; error(...): void }` — sends `log` notifications.
  - `interface ChannelDefinition { start(ctx: ChannelContext): Promise<{ stop?: () => void | Promise<void> } | void>; send(msg: SendRequest, ctx: ChannelContext, signal: AbortSignal): Promise<SendResult | void> }`
  - `class ConfigError extends Error`
  - `function defineChannel(def: ChannelDefinition, io?: { input: NodeJS.ReadableStream; output: NodeJS.WritableStream }): void`

Behavior: `initialize` → build ctx → `await def.start(ctx)` → reply `{protocol_version: 1}`; `ConfigError` → error `-32001`; other start errors → error `-32603` then exit 1. `send` → `def.send` with an `AbortSignal` aborted by `$/cancelRequest`. `shutdown` → `stop()` → reply → exit 0. stdin `end` → `stop()` (if started) → exit 0. On load: `console.log/info/debug` rewired to stderr. `uncaughtException`/`unhandledRejection` → stderr + exit 1. `saveMedia` writes `<mediaDir>/<Date.now()>-<8 random hex><ext-with-dot>`.

- [ ] **Step 1: Scaffold** package with `typescript` and `@types/node` dev deps via `pnpm install`; `pnpm build` succeeds on an empty `src/index.ts`.
- [ ] **Step 2: Write failing tests** in `test/sdk.test.mjs` (spawn `node test/fixtures/echo-plugin.mjs`, talk NDJSON over its stdio):
  - `initialize then send round-trip` — `send` echoes `content` back as an `inbound` notification and replies `{}`.
  - `config error maps to -32001` — fixture env `FAIL_CONFIG=1`.
  - `cancelRequest aborts send` — fixture env `SLOW_SEND=1` waits on the signal; after `$/cancelRequest` the reply is an error.
  - `console.log is redirected to stderr` — fixture logs on start; every stdout line parses as JSON.
  - `exits when stdin closes` — end stdin → process exits with code 0 within 2s.
  - `saveMedia writes into mediaDir` — returned path is inside the temp media dir and file contents match.
- [ ] **Step 3: Run** `pnpm build && pnpm test` — Expected: FAIL.
- [ ] **Step 4: Implement** `src/rpc.ts` (line framing, id correlation) and `src/index.ts`.
- [ ] **Step 5: Run** `pnpm build && pnpm test` — Expected: PASS.
- [ ] **Step 6: Commit** `git commit -m "feat(sdk): add Node channel plugin SDK"`

### Task 12: Discord reference plugin

**Files:**
- Create: `plugins/discord/package.json`, `openbee.plugin.json`, `tsconfig.json`, `src/index.ts`, `src/discord.ts`, `src/helpers.ts`, `test/helpers.test.mjs`, `README.md`, `.gitignore`

**Interfaces:**
- Consumes: `@theopenbee/channel-sdk` via `"file:../../sdk/channel-node"`.
- Produces: manifest `{protocol_version:1, id:"discord", name:"Discord", command:["node","dist/index.js"], config:[token(secret, required), allowed_guilds(list), max_media_size(int, default 10485760)]}` with zh/en labels; package `openbee-channel-discord`, `"private": true`.
- Helpers in `src/helpers.ts`: `chunkText(text: string, limit = 2000): string[]` (prefers splitting at the last newline before the limit, never returns empty chunks); `stripMention(text: string, botId: string): string` (removes `<@id>` and `<@!id>`, trims); `attachmentType(contentType: string | null): "image"|"audio"|"video"|"document"`; `encodeRaw({channel_id, message_id}) / decodeRaw(raw)`.

Behavior:
- Intents `Guilds`, `GuildMessages`, `MessageContent`, `DirectMessages`; partial `Channel`.
- Ignore messages from bots. Guild messages only when the bot is mentioned; DMs always. If `allowed_guilds` is non-empty, ignore guild messages from other guilds.
- Inbound: `chat_id = message.channelId`, `sender_id = author.id`, `message_id = message.id`, `text = stripMention(content)`, `raw = encodeRaw(...)`, `context = {guild_id, channel_id, author_name}`, `time = createdTimestamp`; call `channel.sendTyping()`.
- Attachments ≤ `max_media_size` are downloaded with `fetch` and stored via `ctx.saveMedia`; larger ones are dropped and `[attachment too large: <name>]` is appended to the text.
- Send: fetch channel by `chat_id`; text via `chunkText`, first chunk replies to `reply_to.message_id` when not proactive (`failIfNotExists: false`); media sent after text as `files: [{attachment: path, name}]`.
- `start`: missing token → `ConfigError`; login rejected with `TokenInvalid` → `ConfigError`. `stop` → `client.destroy()`.

- [ ] **Step 1: Write failing tests** `test/helpers.test.mjs`: `chunkText splits at 2000` (4500 chars → 3 chunks, each ≤ 2000, joined equals input), `chunkText prefers newlines`, `stripMention removes both forms`, `attachmentType maps content types`, `raw round-trips`, `oversized attachment becomes placeholder` (pure function `planAttachments(list, maxSize)` returns kept items and the placeholder suffix).
- [ ] **Step 2: Run** `pnpm install && pnpm build && pnpm test` — Expected: FAIL.
- [ ] **Step 3: Implement** helpers and `src/discord.ts` / `src/index.ts`.
- [ ] **Step 4: Run** `pnpm build && pnpm test` — Expected: PASS; `node dist/index.js < /dev/null` exits 0.
- [ ] **Step 5: Write `README.md`**: Discord bot setup (Developer Portal → Bot → token, enable Message Content intent, invite with scopes `bot` and permissions View Channels, Send Messages, Read Message History, Attach Files), `pnpm install && pnpm build`, `openbee plugin install <abs path>`.
- [ ] **Step 6: Commit** `git commit -m "feat(plugins): add Discord reference channel plugin"`

### Task 13: Protocol reference and changelog

**Files:**
- Create: `docs/channel-plugin-protocol.md`
- Modify: `CHANGELOG.md`, `README.md` and `README.zh.md` (one short "Channel plugins" paragraph linking the protocol doc)

- [ ] **Step 1: Write `docs/channel-plugin-protocol.md`** (English): manifest schema, install layout, config placement, process rules (stdout protocol only, stderr logs, exit on stdin EOF, restart policy), every method with params/results/error codes exactly as implemented in Tasks 5–8, session key and dedup rules, attachment rules, versioning rule (unknown fields ignored).
- [ ] **Step 2: Cross-check** each JSON field name in the doc against `internal/platform/pluginchannel/protocol.go` and `sdk/channel-node/src/types.ts` — Expected: identical sets.
- [ ] **Step 3: Add CHANGELOG entry** under the unreleased section: `Add external channel plugins with Discord reference plugin` (8 words).
- [ ] **Step 4: Commit** `git commit -m "docs: document channel plugin protocol"`

### Task 14: End-to-end acceptance on an isolated instance

Requires a Discord test bot token from the owner (requested when the plan starts executing).

- [ ] **Step 1:** Build web + binary per the E2E recipe; isolated `HOME`; port different from production.
- [ ] **Step 2:** `cd plugins/discord && pnpm install && pnpm build`; `HOME=<test home> ./openbee plugin install <abs>/plugins/discord` → success text.
- [ ] **Step 3:** Add the discord section to the test `config.yaml` (via `openbee config` once to validate the wizard path), start the instance.
- [ ] **Step 4:** Verify in Discord: DM → bee reply; guild @mention → reply, no reply without mention; worker reply via `openbee ctl message send`; attachment in (image) and out (file); `/clear`; proactive send via `--session-key`; reply > 2000 chars is chunked.
- [ ] **Step 5:** Kill the plugin process (`kill <pid>`) → log shows restart, bot works again. `kill -9` the openbee process → plugin process exits (no orphan in `ps`).
- [ ] **Step 6:** Set an invalid token → log shows config error for `discord`, other channels and web UI keep working.
- [ ] **Step 7:** Report results to the owner with screenshots/log excerpts.
