package views_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	onboardingAPI = "/api/onboarding"
	skipAPI       = "/api/onboarding/skip"
	installAPI    = "/api/plugins/install"
	operationsAPI = "/api/plugins/operations"
	catalogAPI    = "/api/plugins/catalog"
	piSpec        = "@openbee/engine-pi@0.3.0"
)

func engineEntry(id, version, name, description string) api.CatalogPlugin {
	p := catalogEntry(id, version, true, bilingual(name, name), bilingual(description, description))
	p.Category, p.Package = "engine", "@openbee/engine-"+id
	return p
}

func freshCatalog(st *fakeapi.State) {
	st.OnboardingSkipped = false
	legacy := engineEntry("legacy", "0.1.0", "Legacy Agent", "Needs a newer server.")
	legacy.Installable = false
	st.Catalog = api.PluginCatalog{SchemaVersion: 1, Plugins: []api.CatalogPlugin{
		engineEntry("pi", "0.3.0", "Pi", "A small, fast coding agent."),
		legacy,
		catalogEntry("telegram", "1.3.0", true, bilingual("Telegram", "Telegram"), bilingual("Chat from Telegram.", "在 Telegram 里聊天。")),
	}}
}

func engineReady(st *fakeapi.State) {
	freshCatalog(st)
	st.Catalog.Plugins[0].Installed, st.Catalog.Plugins[0].InstalledVersion = true, "0.3.0"
}

func channelReady(st *fakeapi.State) {
	engineReady(st)
	st.Catalog.Plugins[2].Installed, st.Catalog.Plugins[2].InstalledVersion = true, "1.3.0"
	st.Plugins = []api.PluginDetail{installedPlugin("telegram", "1.3.0", "not_configured", "", false, bilingual("Telegram", "Telegram"))}
}

func welcomeSeed(st *fakeapi.State) {
	st.OnboardingSkipped = false
	claude := engineEntry("claude", "0.3.0", "Claude Code", "Anthropic's coding agent, driven from your terminal.")
	claude.Installed, claude.InstalledVersion = true, "0.3.0"
	telegram := catalogEntry("telegram", "1.3.0", true, bilingual("Telegram", "Telegram"),
		bilingual("Chat with Bee from Telegram groups and direct messages.", "在 Telegram 群组和私聊里与 Bee 对话。"))
	telegram.Installed, telegram.InstalledVersion = true, "1.3.0"
	st.Catalog = api.PluginCatalog{SchemaVersion: 1, Plugins: []api.CatalogPlugin{
		claude,
		engineEntry("codex", "0.3.0", "Codex", "OpenAI's coding agent with sandboxed shell access."),
		engineEntry("opencode", "0.2.0", "OpenCode", "Open source agent that works with many model providers."),
		engineEntry("pi", "0.3.0", "Pi", "A small, fast coding agent with a minimal toolset."),
		telegram,
		catalogEntry("feishu", "0.9.2", true, bilingual("Feishu", "飞书"),
			bilingual("Message Bee from Feishu chats and groups.", "在飞书群聊与私聊里给 Bee 发消息。")),
		catalogEntry("dingtalk", "0.8.1", true, bilingual("DingTalk", "钉钉"),
			bilingual("Answer DingTalk group mentions with the right worker.", "在钉钉群里由合适的员工回复 @ 消息。")),
		catalogEntry("wecom", "0.6.0", true, bilingual("WeCom", "企业微信"),
			bilingual("Route WeCom messages to Bee and your workers.", "把企业微信消息交给 Bee 和员工。")),
		catalogEntry("discord", "1.1.0", true, bilingual("Discord", "Discord"),
			bilingual("Bring Bee into Discord servers, threads and DMs.", "把 Bee 带进 Discord 服务器、话题和私信。")),
		catalogEntry("weixin", "0.3.0", true, bilingual("WeChat", "微信"),
			bilingual("Talk to Bee from a personal WeChat account.", "用个人微信与 Bee 对话。")),
	}}
	st.Operations = []api.PluginOperation{{ID: "op-codex", Kind: "install", Spec: "@openbee/engine-codex@0.3.0", Status: "running",
		CreatedAt: time.Now().Add(-12 * time.Second)}}
}

var welcomeScreens = []screen{
	{name: "welcome-engine", seed: welcomeSeed, signedIn: true, want: locales.PluginsActionInstallingShort},
	{name: "welcome-channel", seed: welcomeSeed, signedIn: true, prepare: nextStep, want: locales.PluginsActionConfigure},
}

func onPath(t *testing.T, h *testkit.Harness, want string) {
	t.Helper()
	if got := h.Root.Nav().Path(); got != want {
		t.Fatalf("path = %q, want %q; texts %q", got, want, h.T.Texts())
	}
}

func shows(t *testing.T, h *testkit.Harness, texts ...string) {
	t.Helper()
	for _, text := range texts {
		if !h.T.HasText(text) {
			t.Fatalf("%q is not shown; texts %q", text, h.T.Texts())
		}
	}
}

func hides(t *testing.T, h *testkit.Harness, texts ...string) {
	t.Helper()
	for _, text := range texts {
		if h.T.HasText(text) {
			t.Fatalf("%q is shown; texts %q", text, h.T.Texts())
		}
	}
}

func nextStep(t *testing.T, h *testkit.Harness) {
	t.Helper()
	h.Click(locales.WelcomeNext())
	shows(t, h, locales.WelcomeChannelTitle())
}

func TestPendingOnboardingShowsWelcome(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, freshCatalog)
	onPath(t, h, route.WelcomePath)
	shows(t, h, locales.WelcomeEngineTitle(), locales.WelcomeEngineSkipHint(), "Pi", locales.PluginsActionInstall(),
		locales.PluginsActionIncompatible())
	hides(t, h, "Telegram")
	for name, open := range map[string]func(){
		"settings":     func() { h.Root.OpenSettings("") },
		"conversation": func() { h.Root.OpenConversation(api.BeeConversation) },
		"new worker":   h.Root.NewWorker,
		"chat route":   func() { h.Root.Nav().Push(route.Chat(api.BeeConversation)) },
		"plugin route": func() { h.Root.Nav().Push(route.Settings("plugins", "telegram")) },
	} {
		open()
		h.Settle()
		if got := h.Root.Nav().Path(); got != route.WelcomePath {
			t.Fatalf("%s left the welcome page for %q", name, got)
		}
	}
}

func TestSettledOnboardingGoesToChat(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, staffed)
	onPath(t, h, route.Chat(api.BeeConversation))
	hides(t, h, locales.WelcomeEngineTitle())
	h.Root.Nav().Push(route.WelcomePath)
	h.Settle()
	onPath(t, h, route.Chat(api.BeeConversation))
}

func TestOnboardingErrorGoesToChat(t *testing.T) {
	h := testkit.New(t, wideW, wideH, freshCatalog)
	h.Connect()
	h.Fake.FailNext(http.MethodGet, onboardingAPI, http.StatusInternalServerError, map[string]any{"error": "boom"})
	h.SignIn("admin", "secret")
	onPath(t, h, route.Chat(api.BeeConversation))
	hides(t, h, locales.WelcomeEngineTitle())
}

func TestNoChatWhileOnboardingLoads(t *testing.T) {
	h := testkit.New(t, wideW, wideH, freshCatalog)
	h.Connect()
	release := h.Fake.HoldNext(http.MethodGet, onboardingAPI)
	h.Store.Login("admin", "secret", nil)
	h.Fix.Pump(t, func() bool {
		h.T.Frame()
		return h.Store.Phase == store.PhaseReady && h.Store.Onboarding.Loading
	})
	h.T.Frame()
	if texts := h.T.Texts(); len(texts) != 0 {
		t.Fatalf("texts while the onboarding status loads = %q, want a blank window", texts)
	}
	release()
	h.Settle()
	onPath(t, h, route.WelcomePath)
	shows(t, h, locales.WelcomeEngineTitle())
}

func TestNextWaitsForAnInstalledEngine(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, freshCatalog)
	h.Click(locales.WelcomeNext())
	hides(t, h, locales.WelcomeChannelTitle())

	h.Click(locales.PluginsActionInstall())
	calls := h.Fake.Calls(http.MethodPost, installAPI)
	if len(calls) != 1 {
		t.Fatalf("install posts = %d, want 1", len(calls))
	}
	var body map[string]string
	if err := json.Unmarshal(calls[0].Body, &body); err != nil || body["spec"] != piSpec {
		t.Fatalf("install body = %s (%v), want %s", calls[0].Body, err, piSpec)
	}
	shows(t, h, locales.PluginsActionInstallingShort(), "0:00")
	h.Click(locales.WelcomeNext())
	hides(t, h, locales.WelcomeChannelTitle())

	ops := h.Fake.Snapshot().Operations
	h.Fake.FinishOperation(ops[len(ops)-1].ID, true, "")
	h.Fix.Await(t, func() bool { return h.Fix.Clock.Sleepers() > 0 })
	n := len(h.Fake.Calls(http.MethodGet, operationsAPI))
	h.Fix.Clock.Advance(time.Second)
	h.Fix.Await(t, func() bool {
		h.T.Frame()
		return len(h.Fake.Calls(http.MethodGet, operationsAPI)) > n && h.T.HasText(locales.WelcomeInstalled())
	})
	h.Settle()
	hides(t, h, locales.PluginsActionInstallingShort())

	nextStep(t, h)
	onPath(t, h, route.WelcomePath)
	shows(t, h, locales.WelcomeChannelDescription(), "Telegram", locales.WelcomeDone())
	hides(t, h, "A small, fast coding agent.")
}

func TestSkipOnboardingGoesToChat(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, freshCatalog)
	h.Click(locales.WelcomeSkip())
	if n := len(h.Fake.Calls(http.MethodPost, skipAPI)); n != 1 {
		t.Fatalf("skip posts = %d, want 1", n)
	}
	if !h.Fake.Snapshot().OnboardingSkipped {
		t.Fatal("the server did not record the skip")
	}
	onPath(t, h, route.Chat(api.BeeConversation))
	hides(t, h, locales.WelcomeEngineTitle())
}

func TestSkipFailureStaysOnWelcome(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, freshCatalog)
	h.Fake.FailNext(http.MethodPost, skipAPI, http.StatusInternalServerError, map[string]any{"error": "disk full"})
	h.Click(locales.WelcomeSkip())
	onPath(t, h, route.WelcomePath)
	shows(t, h, "disk full", locales.WelcomeEngineTitle())
}

func TestChannelStepLeavesForChat(t *testing.T) {
	for name, button := range map[string]func() string{"done": locales.WelcomeDone, "skip": locales.WelcomeSkip} {
		t.Run(name, func(t *testing.T) {
			h := testkit.SignedIn(t, wideW, wideH, engineReady)
			nextStep(t, h)
			h.Click(button())
			onPath(t, h, route.Chat(api.BeeConversation))
			if n := len(h.Fake.Calls(http.MethodPost, skipAPI)); n != 0 {
				t.Fatalf("leaving the channel step posted %d skips, want none", n)
			}
			h.Root.Nav().Push(route.WelcomePath)
			h.Settle()
			onPath(t, h, route.Chat(api.BeeConversation))
		})
	}
}

func TestConfigureOpensPluginSettings(t *testing.T) {
	h := testkit.SignedIn(t, wideW, wideH, channelReady)
	nextStep(t, h)
	hides(t, h, locales.WelcomeInstalled())
	h.Click(locales.PluginsActionConfigure())
	onPath(t, h, route.Settings("plugins", "telegram"))
	shows(t, h, locales.PluginsTitle())
	if n := len(h.Fake.Calls(http.MethodPost, skipAPI)); n != 0 {
		t.Fatalf("configure posted %d skips, want none", n)
	}
}

func TestCatalogErrorOnWelcome(t *testing.T) {
	failing := func(t *testing.T) *testkit.Harness {
		h := testkit.New(t, wideW, wideH, freshCatalog)
		h.Connect()
		h.Fake.FailNext(http.MethodGet, catalogAPI, http.StatusInternalServerError, map[string]any{"error": "registry down"})
		h.SignIn("admin", "secret")
		shows(t, h, locales.PluginsCatalogError(), "registry down", locales.CommonRetry())
		hides(t, h, "Pi")
		return h
	}
	t.Run("retry", func(t *testing.T) {
		h := failing(t)
		h.Click(locales.CommonRetry())
		shows(t, h, "Pi")
		hides(t, h, locales.PluginsCatalogError())
	})
	t.Run("skip", func(t *testing.T) {
		h := failing(t)
		h.Click(locales.WelcomeSkip())
		onPath(t, h, route.Chat(api.BeeConversation))
	})
}
