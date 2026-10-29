package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/l10n"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	pongPrompt    = "Reply with the single word PONG and nothing else."
	statusCommand = "/status"
	handoffPrompt = "Hand this task to the worker named %s right now as an immediate task: reply with the single word PONG and nothing else."
	longPrompt    = "Run the shell command `sleep 90` and then reply with the single word DONE and nothing else."
	pong          = "PONG"
	finished      = "DONE"
	quickWait     = 30 * time.Second
	startWait     = 60 * time.Second
	replyWait     = 120 * time.Second
	catalogWait   = 90 * time.Second
	restartWait   = 3 * time.Minute
	restartTries  = 6
	restartPause  = 5 * time.Second
	restartBusy   = "retry later"
	envFirst      = "alpha-value-1234"
	envSecond     = "bravo-value-5678"
	badArgs       = `--thinking "low`
	goodArgs      = "--thinking low"
	keyArgsBee    = "engine_args_bee"
	keyEngine     = "default_engine"
	maskVisible   = 4
	senderUser    = "user"
	senderWorker  = "worker"
	senderBee     = "bee"
	senderSystem  = "system"
	kindHandoff   = "handoff"
	kindReset     = "reset"
	stateRunning  = "running"
	stateIdle     = "idle"
)

func TestLiveSignIn(t *testing.T) {
	f := start(t, nil)
	if got := f.path(); got != route.Chat(api.BeeConversation) {
		t.Fatalf("path after signing in = %q, want the Bee chat", got)
	}
	f.await("the server's workers in the list", quickWait, func() bool {
		return f.Store.Workers.Loaded && f.Store.Conversations.Loaded
	})
	f.wantShown(locales.ChatBeeLabel())
	for _, w := range f.admin.workers() {
		f.wantShown(w.Name)
	}
	f.shot("signin-ready")

	f.click(locales.ArcUserMenuTrigger(f.cfg.user))
	f.wantShown(locales.AccountRole(), locales.ArcUserMenuSignOut())
	f.shot("signin-account-menu")
	f.click(locales.ArcUserMenuSignOut())
	f.await("the login screen after signing out", quickWait, func() bool { return f.Store.Phase == store.PhaseLogin })
	f.wantShown(locales.LoginTitle(), locales.LoginServer(f.Store.Server))
	if !f.T.Focused(locales.LoginPassword()) {
		t.Fatal("the password field does not take the focus with a remembered username")
	}

	f.Fill(locales.LoginPassword(), "wrong-"+f.cfg.pass)
	f.enter()
	f.await("the rejected password alert", quickWait, func() bool { return f.shows(locales.LoginError401()) })
	if f.Store.Phase != store.PhaseLogin {
		t.Fatalf("phase = %v after a wrong password", f.Store.Phase)
	}
	f.shot("signin-error")

	f.SignIn(f.cfg.user, f.cfg.pass)
	f.await("the Bee chat after signing in again", quickWait, func() bool {
		return f.Store.Phase == store.PhaseReady && f.path() == route.Chat(api.BeeConversation) && f.Store.Online
	})
}

func TestLiveCreateWorkerAndChat(t *testing.T) {
	name := uniqueName("chat")
	f := start(t, func(a *admin) { a.cleanupWorker(name) })
	f.click(locales.ChatNewWorker())
	f.wantShown(locales.WorkersFormSectionBasic())
	f.Fill(locales.WorkersFormName(), name)
	f.Fill(locales.WorkersFormDescriptionOptional(), workerNote)
	f.shot("create-form")
	f.click(locales.WorkersCreateWorker())

	var w api.Worker
	f.await("the new worker's chat", quickWait, func() bool {
		found, ok := f.worker(name)
		w = found
		return ok && f.path() == route.Chat(w.ID)
	})
	cid := api.WorkerConversation(w.ID)
	f.wantShown(locales.WorkerDetailConstraintsPanelEmptyTitle(name))
	f.Fill(locales.ChatMessageLabel(), pongPrompt)
	f.enter()
	if _, ok := f.message(cid, func(m model.Message) bool { return m.Sender.Type == senderUser && m.Content == pongPrompt }); !ok {
		t.Fatalf("the prompt is not in the chat; texts %q", f.T.Texts())
	}
	f.wantHidden(locales.ChatSendFailed())
	f.shot("create-sent")

	f.click(locales.ChatBeeLabel())
	if got := f.path(); got != route.Chat(api.BeeConversation) {
		t.Fatalf("path = %q, want the Bee chat", got)
	}
	var sum api.ConversationSummary
	f.await("the worker's PONG reply, unread in the list", replyWait, func() bool {
		sum = f.summary(cid)
		last := sum.LastMessage
		return sum.Unread > 0 && last != nil && last.Sender.Type == senderWorker && containsFold(last.Preview, pong)
	})
	f.wantShown(sum.LastMessage.Preview, model.BadgeText(sum.Unread))
	f.shot("create-unread")

	f.click(name)
	f.await("the reply in the worker chat with the unread badge cleared", quickWait, func() bool {
		_, replied := f.message(cid, func(m model.Message) bool { return m.Sender.Type == senderWorker && containsFold(m.Content, pong) })
		shown := f.countTexts(func(text string) bool { return containsFold(text, pong) && text != pongPrompt })
		return f.path() == route.Chat(w.ID) && replied && shown >= 2 && f.summary(cid).Unread == 0
	})
	f.await("the worker chat back to idle", replyWait, func() bool {
		return f.summary(cid).Activity.State == stateIdle && !f.shows(locales.ChatStop())
	})
	f.shot("create-chat")
}

func TestLiveBeeHandoff(t *testing.T) {
	name := uniqueName("handoff")
	var w api.Worker
	f := start(t, func(a *admin) {
		w = a.createWorker(name)
		a.clear(api.BeeConversation)
		a.cleanupConversation(api.BeeConversation)
	})
	cid := api.WorkerConversation(w.ID)
	f.await("the new worker in the list", quickWait, func() bool { return f.shows(name) })
	f.Fill(locales.ChatMessageLabel(), fmt.Sprintf(handoffPrompt, name))
	f.enter()

	title := locales.ChatHandedOffTo(name)
	f.await("the Bee's handoff card", replyWait, func() bool {
		_, ok := f.message(api.BeeConversation, func(m model.Message) bool {
			return m.Kind == kindHandoff && m.Handoff != nil && m.Handoff.WorkerID == w.ID
		})
		return ok && f.shows(title)
	})
	f.shot("handoff-card")

	f.T.SetSize(narrowW, windowH)
	f.Motion()
	f.reveal(title, narrowChat)
	f.click(title)
	f.T.SetSize(windowW, windowH)
	f.Motion()
	if got := f.path(); got != route.Chat(w.ID) {
		t.Fatalf("path after clicking the handoff card = %q, want %q", got, route.Chat(w.ID))
	}
	f.await("the handed-off instruction in the worker chat", quickWait, func() bool {
		_, ok := f.message(cid, func(m model.Message) bool { return m.Sender.Type == senderBee })
		return ok && f.shows(locales.ChatFromBee())
	})
	f.await("the worker's answer to the handed-off task", replyWait, func() bool {
		_, ok := f.message(cid, func(m model.Message) bool { return m.Sender.Type == senderWorker })
		return ok && f.summary(cid).Activity.State == stateIdle
	})
	f.shot("handoff-worker-chat")
}

func TestLiveStopAndReset(t *testing.T) {
	name := uniqueName("stop")
	var w api.Worker
	f := start(t, func(a *admin) { w = a.createWorker(name) })
	cid := api.WorkerConversation(w.ID)
	f.await("the worker in the list", quickWait, func() bool { return f.shows(name) })
	f.click(name)
	f.Fill(locales.ChatMessageLabel(), longPrompt)
	f.enter()
	f.await("the running task's Stop button", startWait, func() bool {
		return f.summary(cid).Activity.State == stateRunning && f.shows(locales.ChatStop())
	})
	f.wantShown(locales.ChatStatusRunning())
	f.shot("stop-running")

	f.click(locales.ChatStop())
	f.await("the stopped chat back to idle", startWait, func() bool {
		return f.summary(cid).Activity.State == stateIdle && !f.shows(locales.ChatStop()) && f.shows(locales.StatusesIdle())
	})
	if m, ok := f.message(cid, func(m model.Message) bool { return m.Sender.Type == senderWorker && containsFold(m.Content, finished) }); ok {
		t.Fatalf("the stopped task still finished: %q", m.Content)
	}
	f.shot("stop-idle")

	dividers := func() int {
		return f.countTexts(func(text string) bool { return text == locales.ChatNewConversation() })
	}
	before := dividers()
	f.click(locales.ChatNewConversation())
	f.wantShown(locales.ChatNewConversationConfirm())
	f.clickBeside(locales.CommonCancel())
	if f.shows(locales.ChatStopAndRestart()) {
		f.click(locales.ChatStopAndRestart())
	}
	f.await("the new conversation divider", quickWait, func() bool {
		msgs := f.Store.Chat(cid).List.Msgs
		return len(msgs) > 0 && msgs[len(msgs)-1].Kind == kindReset && !f.shows(locales.ChatNewConversationConfirm()) &&
			dividers() == before+1
	})
	f.shot("reset-divider")
}

func masked(value string) string {
	runes := []rune(value)
	if len(runes) < 2*maskVisible {
		return "****"
	}
	return string(runes[:maskVisible]) + "****" + string(runes[len(runes)-maskVisible:])
}

func (f *flow) globalEnv(key string) (api.EnvConfig, bool) {
	for _, e := range f.admin.envs("global", "") {
		if e.Key == key {
			return e, true
		}
	}
	return api.EnvConfig{}, false
}

func TestLiveEnvCrud(t *testing.T) {
	key := strings.ToUpper(strings.ReplaceAll(strings.TrimPrefix(uniqueName("env"), namePrefix), "-", "_"))
	key = envPrefix + key
	f := start(t, func(a *admin) { a.cleanupEnv("global", "", key) })
	f.click(locales.NavSettings())
	f.await("the env settings", quickWait, func() bool {
		return f.shows(locales.EnvConfigPageDescription()) && f.shows(locales.EnvConfigAdd())
	})

	f.click(locales.EnvConfigAdd())
	f.wantShown(locales.EnvConfigAddDescription())
	f.Fill(locales.EnvConfigKey(), key)
	f.Fill(locales.EnvConfigValue(), envFirst)
	f.shot("env-add-dialog")
	f.enter()
	f.await("the added variable", quickWait, func() bool {
		return !f.shows(locales.EnvConfigAddDescription()) && f.shows(key) && f.shows(masked(envFirst))
	})
	if e, ok := f.globalEnv(key); !ok || e.Masked != masked(envFirst) {
		t.Fatalf("server env %s = %+v (found %v), want masked %q", key, e, ok, masked(envFirst))
	}
	f.shot("env-added")

	f.click(locales.EnvConfigEditLabel(key))
	f.wantShown(locales.EnvConfigEditDescription())
	f.Fill(locales.EnvConfigNewValue(), envSecond)
	f.click(locales.EnvConfigSaveValue())
	f.await("the edited variable", quickWait, func() bool {
		return !f.shows(locales.EnvConfigEditDescription()) && f.shows(masked(envSecond))
	})
	if e, ok := f.globalEnv(key); !ok || e.Masked != masked(envSecond) {
		t.Fatalf("server env %s = %+v (found %v), want masked %q", key, e, ok, masked(envSecond))
	}

	f.click(locales.EnvConfigDeleteLabel(key))
	f.wantShown(locales.EnvConfigDeleteTitle(key))
	f.shot("env-delete-dialog")
	f.clickBeside(locales.CommonCancel())
	f.await("the deleted variable gone", quickWait, func() bool {
		return !f.shows(locales.EnvConfigDeleteTitle(key)) && !f.shows(key)
	})
	if e, ok := f.globalEnv(key); ok {
		t.Fatalf("server still has %+v", e)
	}
	f.shot("env-deleted")
}

func (f *flow) openSystem() {
	f.t.Helper()
	if !f.shows(locales.ArcDialogClose()) {
		f.click(locales.NavSettings())
	}
	if !f.shows(locales.SystemSettingsSubtitle()) {
		plugins := f.rect(locales.PluginsTitle())
		f.T.ClickAt(plugins.X+plugins.W/2, plugins.Y+plugins.H/2-navStep)
		f.Motion()
	}
	f.await("the system settings form", quickWait, func() bool {
		return f.Store.SystemConfigs.Loaded && f.shows(locales.SystemSettingsSubtitle()) && f.shows(locales.SystemSettingsSave())
	})
}

func (f *flow) fieldText(field string) string {
	f.t.Helper()
	f.Click(field)
	f.T.SetClipboard("")
	f.T.Command("selectAll")
	f.T.Command("copy")
	return f.T.Clipboard()
}

func beeArgsField(engines []string, engine string) string {
	label := l10n.EngineLabel(engine)
	if len(engines) == 1 {
		label = locales.WorkersFormEngineArgs()
	}
	return locales.SystemSettingsBeeArgsSectionTitle() + ": " + label
}

func (f *flow) beeArgs(engine string) string {
	return model.ParseEngineArgsConfig(f.admin.systemConfigs()[keyArgsBee])[engine]
}

func TestLiveSystemSettings(t *testing.T) {
	var saved map[string]string
	f := start(t, func(a *admin) {
		saved = a.systemConfigs()
		a.restoreSystemConfigs(saved)
	})
	f.openSystem()
	engines := f.Store.EnabledEngines()
	if len(engines) == 0 {
		t.Fatal("the server has no enabled engine")
	}
	engine := model.DefaultEngine(saved[keyEngine], engines)
	field := beeArgsField(engines, engine)
	f.reveal(field, settingsPane)
	f.Fill(field, badArgs)
	f.enter()
	f.await("the rejected engine args", quickWait, func() bool { return f.shows(locales.SystemSettingsSaveError()) })
	f.shot("system-error")

	f.Fill(field, goodArgs)
	f.enter()
	f.await("the saved engine args", quickWait, func() bool { return f.shows(locales.SystemSettingsSaved()) })
	if got := f.beeArgs(engine); got != goodArgs {
		t.Fatalf("server bee args for %s = %q, want %q", engine, got, goodArgs)
	}
	f.shot("system-saved")

	f.T.Key(0, ui.KeyEscape)
	f.Motion()
	f.wantHidden(locales.SystemSettingsSubtitle())
	f.openSystem()
	f.reveal(field, settingsPane)
	if got := f.fieldText(field); got != goodArgs {
		t.Fatalf("bee args after reopening = %q, want %q", got, goodArgs)
	}
	f.Fill(field, "")
	f.enter()
	f.await("the cleared engine args", quickWait, func() bool { return f.shows(locales.SystemSettingsSaved()) && f.beeArgs(engine) == "" })
}

func TestLivePluginsCatalogLoads(t *testing.T) {
	var catalog api.PluginCatalog
	var installed []api.PluginStatus
	f := start(t, func(a *admin) {
		catalog = a.catalog()
		installed = a.plugins()
	})
	if len(catalog.Plugins) == 0 {
		t.Fatal("the server catalog lists no plugins")
	}
	names := make([]string, len(catalog.Plugins))
	for i, p := range catalog.Plugins {
		names[i] = l10n.Localized(p.Name, i18n.Locale())
	}
	f.click(locales.NavSettings())
	f.click(locales.PluginsTitle())
	f.await("the plugin catalog", catalogWait, func() bool { return f.Store.Plugins().Catalog.Loaded && f.shows(names[0]) })
	f.wantShown(names...)
	f.shot("plugins-market")

	f.click(locales.PluginsRefreshCatalog())
	f.await("the refreshed catalog", catalogWait, func() bool {
		r := f.Store.Plugins().Catalog
		return r.Loaded && !r.Loading && r.Err == nil && f.shows(names[0])
	})
	f.wantHidden(locales.PluginsCatalogError())

	f.click(locales.PluginsTabsInstalled())
	want := locales.PluginsInstalledTitle()
	if len(installed) == 0 {
		want = locales.PluginsEmptyInstalledTitle()
	}
	f.await("the installed tab", quickWait, func() bool { return f.shows(want) })
	f.shot("plugins-installed")
}

type outcome struct {
	out []byte
	err error
}

func (f *flow) idle() bool {
	for _, cs := range f.Store.Conversations.Data {
		if cs.Activity.State != stateIdle {
			return false
		}
	}
	return f.Store.Conversations.Loaded
}

func (f *flow) restartServer() {
	f.t.Helper()
	for attempt := 1; ; attempt++ {
		result := make(chan outcome, 1)
		go func() {
			out, err := exec.Command("sh", "-c", f.cfg.restart).CombinedOutput()
			result <- outcome{out: out, err: err}
		}()
		var res outcome
		f.await("the restart command", restartWait, func() bool {
			select {
			case res = <-result:
				return true
			default:
				return false
			}
		})
		if res.err == nil {
			return
		}
		if attempt == restartTries || !strings.Contains(string(res.out), restartBusy) {
			f.t.Fatalf("restart command %q: %v\n%s", f.cfg.restart, res.err, res.out)
		}
		f.t.Logf("restart refused (attempt %d): %s", attempt, strings.TrimSpace(string(res.out)))
		resume := time.Now().Add(restartPause)
		f.await("the pause before retrying the restart", 2*restartPause, func() bool { return time.Now().After(resume) })
	}
}

func TestLiveReconnect(t *testing.T) {
	cfg := loadConfig(t)
	if cfg.restart == "" {
		t.Skip(RestartEnv + " is not set")
	}
	name := uniqueName("reconnect")
	var w api.Worker
	link := newRelay(t, cfg.url)
	f := launch(t, cfg, link.url, func(a *admin) { w = a.createWorker(name) })
	cid := api.WorkerConversation(w.ID)
	f.await("the worker in the list", quickWait, func() bool { return f.shows(name) })
	f.click(name)
	f.await("the worker chat online", quickWait, func() bool { return f.Store.Chat(cid).Loaded && f.Store.Online })

	f.admin.send(cid, statusCommand)
	f.await("the live reply to a command sent from another client", quickWait, func() bool {
		_, ok := f.message(cid, func(m model.Message) bool { return m.Sender.Type == senderSystem })
		return ok
	})
	f.await("every conversation idle before the outage", replyWait, f.idle)
	f.shot("reconnect-online")

	link.cut()
	f.await("the offline banner", quickWait, func() bool {
		return f.Store.Offline && f.shows(locales.ChatConnectionLost()) && f.shows(locales.ChatReconnectNow())
	})
	f.shot("reconnect-offline")

	f.restartServer()
	marker := statusCommand + " " + uniqueName("marker")
	f.admin.send(cid, marker)
	f.await("the server recording the message sent while offline", quickWait, func() bool { return f.admin.recorded(cid, marker) })
	f.wantHidden(marker)

	link.restore()
	f.click(locales.ChatReconnectNow())
	f.await("the banner cleared after reconnecting", startWait, func() bool {
		return f.Store.Online && !f.Store.Offline && !f.shows(locales.ChatConnectionLost())
	})
	f.await("the message sent while offline after the resync", quickWait, func() bool {
		_, ok := f.message(cid, func(m model.Message) bool { return m.Content == marker })
		return ok && f.shows(marker)
	})
	f.await("the open chat read after the resync", quickWait, func() bool {
		sum := f.summary(cid)
		return sum.Unread == 0 && sum.LastMessage != nil && sum.LastMessage.Sender.Type == senderSystem
	})
	seen := map[string]bool{}
	for _, m := range f.Store.Chat(cid).List.Msgs {
		if seen[m.ID] {
			t.Fatalf("message %s shows twice after the resync", m.ID)
		}
		seen[m.ID] = true
	}
	f.shot("reconnect-resynced")
}
