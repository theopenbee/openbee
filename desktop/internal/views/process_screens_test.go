package views_test

import (
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/internal/views/testkit"
	"github.com/theopenbee/openbee/desktop/locales"
)

func tool(kind, name, summary, status string, ms int64, output string) api.ExecutionStep {
	return api.ExecutionStep{Type: "tool", Kind: kind, Name: name, Summary: summary, Status: status, DurationMS: ms, Output: output}
}

func run(id, status string, started time.Time, took time.Duration) api.ExecutionSummary {
	r := api.ExecutionSummary{ID: id, Status: status, StartedAt: started.UnixMilli()}
	if took > 0 {
		done := started.Add(took).UnixMilli()
		r.CompletedAt = &done
	}
	return r
}

func processCrew(st *fakeapi.State) {
	staffed(st)
	now := time.Now()
	linked := said("a2", "worker", "ada", "Swapped the install command on the home page and both install guides. The build passes.", now.Add(-20*time.Minute))
	linked.ExecutionID = "e-done"
	st.Messages = map[string][]api.ChatMessage{
		api.WorkerConversation("ada"): {
			said("a0", "user", "", "Swap the install command on the website for `npm i -g openbee`, then send me a screenshot.", now.Add(-24*time.Minute)),
			said("a1", "worker", "ada", "On it, changing the install command now.", now.Add(-23*time.Minute)),
			linked,
			said("a3", "user", "", "Update the README too and run the tests again.", now.Add(-2*time.Minute)),
		},
	}
	for i := range st.Conversations {
		if st.Conversations[i].WorkerID == "ada" {
			st.Conversations[i].Activity = api.ConversationActivity{State: "running", ExecutionID: "e-live"}
		}
	}
	scheduled := handedOff("b1", "ada", "scheduled", "0 9 * * *", "Check theopenbee.com and the docs for broken links every day at 09:00 and send a list.", now.Add(-50*time.Minute))
	scheduled.Handoff.Run = ptr(run("e-links", "completed", now.Add(-40*time.Minute), 108*time.Second))
	failed := handedOff("b2", "li", "immediate", "", "Translate the 1.0.10 changelog and publish it on /changelog.", now.Add(-49*time.Minute))
	failed.Handoff.Run = ptr(run("e-failed", "failed", now.Add(-48*time.Minute), 10*time.Minute))
	st.Messages[api.BeeConversation] = []api.ChatMessage{
		said("b0", "user", "", "Two things: check the sites for broken links every morning, and translate the 1.0.10 changelog.", now.Add(-51*time.Minute)),
		scheduled, failed,
		said("b3", "bee", "", "Both are arranged: Ada checks links daily at 9, 李娜 is translating the changelog.", now.Add(-48*time.Minute)),
	}
	st.Executions = map[string]api.ExecutionDetail{
		"e-live": {
			ExecutionSummary: run("e-live", "running", now.Add(-75*time.Second), 0),
			TaskID:           "t-live", WorkerID: "ada", Engine: "claude", Instruction: "Update the README too and run the tests again.",
			Steps: []api.ExecutionStep{
				{Type: "thinking", Text: "The README has an English and a Chinese copy. Find the install section, change it, then run pnpm test."},
				tool("message", "send_message", "On it, updating the README and running the tests.", "done", 0, ""),
				tool("search", "Grep", "install.sh", "done", 0, "README.md:12\nREADME.zh.md:12"),
				tool("edit", "Edit", "README.md", "done", 0, "-curl -fsSL https://theopenbee.com/install.sh | sh\n+npm i -g openbee"),
				tool("command", "Bash", "pnpm test", "running", 0, ""),
			},
		},
		"e-done": {
			ExecutionSummary: run("e-done", "completed", now.Add(-24*time.Minute), 192*time.Second),
			TaskID:           "t-done", WorkerID: "ada", Engine: "claude", Instruction: "Swap the install command on the website for `npm i -g openbee`, then send me a screenshot.",
			Steps: []api.ExecutionStep{
				{Type: "thinking", Text: "Find every place that still uses the old install command, both language versions included."},
				tool("read", "Read", "src/components/Hero.astro", "done", 0, ""),
				tool("edit", "Edit", "src/components/Hero.astro", "done", 0, "-curl -fsSL https://theopenbee.com/install.sh | sh\n+npm i -g openbee"),
				tool("command", "Bash", "pnpm build", "done", 41_000, "> website@0.0.1 build\n> astro build\n\n16:02:11 [build] output: \"static\"\n16:02:17 [build] 38 page(s) built in 6.21s\n16:02:17 [build] Complete!"),
				{Type: "text", Text: "Swapped the install command on the home page and both install guides. The build passes."},
			},
		},
		"e-links": {
			ExecutionSummary: run("e-links", "completed", now.Add(-40*time.Minute), 108*time.Second),
			TaskID:           "task-b1", WorkerID: "ada", Engine: "codex", Instruction: scheduled.Content,
			Runs: []api.ExecutionSummary{
				run("e-links", "completed", now.Add(-40*time.Minute), 108*time.Second),
				run("e-links-2", "completed", now.AddDate(0, 0, -1), 95*time.Second),
				run("e-links-1", "failed", now.AddDate(0, 0, -2), 30*time.Second),
			},
			Steps: []api.ExecutionStep{
				{Type: "thinking", Text: "Fetch the sitemap and check every link on each page, reporting only 4xx and 5xx."},
				tool("web", "WebFetch", "https://theopenbee.com/sitemap-index.xml", "done", 1200, "<sitemapindex>…</sitemapindex>"),
				tool("command", "Bash", "npx linkinator https://theopenbee.com --recurse", "done", 38_000, "Scanned 286 links: 285 OK, 1 broken\n  [404] https://theopenbee.com/docs/zh/plugins/wecom-setup"),
				{Type: "text", Text: "Site: no broken links. Docs: one, `/docs/zh/plugins/wecom-setup` returns 404 after a page rename."},
				tool("message", "send_message", "Daily check done: 0 broken links on the site, 1 in the docs (list attached).", "done", 0, ""),
			},
		},
		"e-failed": {
			ExecutionSummary: run("e-failed", "failed", now.Add(-48*time.Minute), 10*time.Minute),
			TaskID:           "task-b2", WorkerID: "li", Engine: "claude", Instruction: failed.Content,
			Error: "Timed out: still running after 10 minutes, stopped.",
			Steps: []api.ExecutionStep{
				{Type: "thinking", Text: "Read the 1.0.10 section of the changelog first, then see where the site's changelog page gets its data."},
				tool("read", "Read", "CHANGELOG.md", "done", 0, ""),
				tool("command", "Bash", "pnpm install", "error", 61_000, "ERR_PNPM_META_FETCH_FAIL GET https://registry.npmjs.org/astro: request failed, reason: connect ETIMEDOUT 104.16.2.35:443"),
				{Type: "text", Text: "Installing dependencies failed, it looks like a network problem. Trying again."},
				tool("command", "Bash", "pnpm install --network-concurrency 4", "running", 0, ""),
			},
		},
	}
}

func ptr[T any](v T) *T { return &v }

func openProcess(param, run, expand string) func(t *testing.T, h *testkit.Harness) {
	return func(t *testing.T, h *testkit.Harness) {
		h.Root.Nav().Push(route.ChatRun(param, run))
		h.Settle()
		h.T.Advance(time.Second)
		h.Settle()
		if expand != "" {
			clickAndSettle(t, h, expand, 0)
		}
	}
}

func hoverReply(t *testing.T, h *testkit.Harness) {
	openProcess("ada", "e-done", "")(t, h)
	r, ok := h.T.Find("Swapped the install command on the home page and both install guides. The build passes.")
	if !ok {
		t.Fatalf("no linked reply; texts %q", h.T.Texts())
	}
	h.T.Move(r.X+r.W/2, r.Y+r.H/2)
	h.Settle()
	h.T.Advance(time.Second)
	h.Settle()
}

var processScreens = []screen{
	{name: "process-running", seed: processCrew, signedIn: true, prepare: openProcess("ada", "e-live", locales.ExecutionToolEdit()),
		shots: wideOnly, want: locales.ExecutionLive},
	{name: "process-completed", seed: processCrew, signedIn: true, prepare: hoverReply, shots: wideOnly,
		want: func() string {
			return locales.ExecutionStepCount(locales.ExecutionSteps(5), locales.ExecutionToolCalls(3))
		}},
	{name: "process-handoff", seed: processCrew, signedIn: true, prepare: openProcess("bee", "e-links", ""), shots: wideOnly,
		want: locales.ExecutionRuns},
	{name: "process-failed", seed: processCrew, signedIn: true, prepare: openProcess("bee", "e-failed", locales.ExecutionToolCommand()),
		want: locales.ExecutionFailedTitle},
}
