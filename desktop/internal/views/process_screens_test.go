package views_test

import (
	"strings"
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

func edited(summary, diff string, ms int64) api.ExecutionStep {
	step := tool("edit", "Edit", summary, "done", ms, "")
	step.Diff = diff
	return step
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
	scheduled := handedOff("b1", "ada", "scheduled", "0 9 * * *", "Check theopenbee.com and the docs for broken links every day at 09:00 and send a list.", now.Add(-50*time.Minute))
	scheduled.Handoff.Run = ptr(run("e-links", "completed", now.Add(-40*time.Minute), 108*time.Second))
	failed := handedOff("b2", "li", "immediate", "", "Translate the 1.0.10 changelog and publish it on /changelog.", now.Add(-49*time.Minute))
	failed.Handoff.Run = ptr(run("e-failed", "failed", now.Add(-48*time.Minute), 10*time.Minute))
	st.Messages = map[string][]api.ChatMessage{
		api.BeeConversation: {
			said("b0", "user", "", "Two things: check the sites for broken links every morning, and translate the 1.0.10 changelog.", now.Add(-51*time.Minute)),
			scheduled, failed,
			said("b3", "bee", "", "Both are arranged: Ada checks links daily at 9, 李娜 is translating the changelog.", now.Add(-48*time.Minute)),
		},
	}
	st.Executions = map[string]api.ExecutionDetail{
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
	}
}

var (
	heroDiff = strings.Join([]string{
		`       <div class="install">`,
		`-        <code>curl -fsSL https://theopenbee.com/install.sh | sh</code>`,
		`+        <code>npm i -g openbee</code>`,
		`+        <span class="hint">Node.js 20+</span>`,
		`         <CopyButton value={command} />`,
		`       </div>`,
	}, "\n")
	buildOutput = strings.Join([]string{
		"> website@0.0.1 build",
		"> astro build",
		"",
		`16:02:11 [build] output: "static"`,
		"16:02:11 [build] Collecting build info...",
		"16:02:14 [build] ✓ Completed in 2.81s.",
		"16:02:17 [build] 38 page(s) built in 6.21s",
		"16:02:17 [build] Complete!",
	}, "\n")
	readmeDiff = "-curl -fsSL https://theopenbee.com/install.sh | sh\n+npm i -g openbee"
)

func processTeam(st *fakeapi.State) {
	now := time.Now()
	ago := func(minutes float64) time.Time { return now.Add(-time.Duration(minutes * float64(time.Minute))) }
	st.Language = "zh"
	st.Workers = []api.Worker{
		staffWorker("lin", "林知夏", "负责官网和文档站前端", "working"),
		staffWorker("gu", "顾北辰", "负责后端与线上巡检", "idle"),
		staffWorker("su", "苏晴", "负责文档和多语言翻译", "idle"),
		staffWorker("zhou", "周砚", "负责桌面客户端", "idle"),
	}
	lin := workerChat("lin", lastMessage("worker", "好的，这就改 README 并跑一遍测试。", ago(1)), 0)
	lin.Activity = api.ConversationActivity{State: "running", ExecutionID: "e-lin-2"}
	st.Conversations = []api.ConversationSummary{
		{ID: api.BeeConversation, Kind: "bee", Activity: api.ConversationActivity{State: "idle"},
			LastMessage: lastMessage("bee", "早上好，两位同事的夜间任务都完成了。", ago(60*20))},
		lin,
		workerChat("su", lastMessage("system", "任务失败：执行超时", ago(9)), 1),
		workerChat("gu", lastMessage("worker", "今天巡检完成：官网 0 个死链，文档站 1 个。", ago(62)), 0),
		workerChat("zhou", lastMessage("worker", "托盘未读数在 Windows 上也正常了。", ago(60*26)), 0),
	}
	linked := func(id, content, execution string, at time.Time) api.ChatMessage {
		m := said(id, "worker", "lin", content, at)
		m.ExecutionID = execution
		return m
	}
	suReply := said("s2", "worker", "su", "收到，先装依赖再翻译。", ago(19))
	suReply.ExecutionID = "e-su-1"
	st.Messages = map[string][]api.ChatMessage{
		api.WorkerConversation("lin"): {
			said("m1", "user", "", "把官网首页的安装命令换成 `npm i -g openbee`，改完跑一遍构建", ago(22)),
			linked("m2", "收到，开始改首页安装命令。", "e-lin-1", ago(21.8)),
			linked("m3", "改好了：首页 hero 的安装命令换成了 `npm i -g openbee`，本地构建通过。", "e-lin-1", ago(18.6)),
			said("m4", "user", "", "顺便把 README 里的也改一下，再跑一遍测试", ago(1.3)),
			linked("m5", "好的，这就改 README 并跑一遍测试。", "e-lin-2", ago(1.1)),
		},
		api.WorkerConversation("su"): {
			said("s1", "user", "", "把 v1.0.10 的 CHANGELOG 翻译成英文，同步到官网 /changelog 页面", ago(19.5)),
			suReply,
		},
	}
	st.Executions = map[string]api.ExecutionDetail{
		"e-lin-1": {
			ExecutionSummary: run("e-lin-1", "completed", ago(21.9), 192*time.Second),
			TaskID:           "t-lin-1", WorkerID: "lin", Engine: "claude", Instruction: "把官网首页的安装命令换成 `npm i -g openbee`，改完跑一遍构建",
			Steps: []api.ExecutionStep{
				{Type: "thinking", Text: "先找出首页用到旧安装命令的地方\n首页 hero 在 src/components/Hero.astro，安装命令写死在 <code> 里。改完要跑一遍 astro build，确认 38 个页面都能生成。"},
				tool("read", "Read", "src/components/Hero.astro", "done", 300, "1  ---\n2  import CopyButton from './CopyButton.astro'\n3  const command = 'curl -fsSL https://theopenbee.com/install.sh | sh'\n4  ---"),
				tool("search", "Grep", "install.sh", "done", 1200, "src/components/Hero.astro:42\nsrc/content/docs/zh/install.md:12"),
				edited("src/components/Hero.astro", heroDiff, 600),
				tool("command", "Bash", "pnpm build", "done", 41_000, buildOutput),
				tool("other", "TodoWrite", "[x] 改首页安装命令  [x] 跑构建", "done", 200, ""),
				{Type: "text", Text: "首页的安装命令已经换成 `npm i -g openbee`，并加了 **Node.js 20+** 的提示。\n构建通过，38 个页面都生成了。"},
				tool("message", "send_message", "改好了：首页 hero 的安装命令换成了 `npm i -g openbee`，本地构建通过。", "done", 700, ""),
			},
		},
		"e-lin-2": {
			ExecutionSummary: api.ExecutionSummary{ID: "e-lin-2", Status: "running", StartedAt: now.Add(-72 * time.Second).UnixMilli()},
			TaskID:           "t-lin-2", WorkerID: "lin", Engine: "claude", Instruction: "顺便把 README 里的也改一下，再跑一遍测试",
			Steps: []api.ExecutionStep{
				{Type: "thinking", Text: "README 有中英文两份，先找到安装段落，改完再跑 pnpm test。"},
				tool("message", "send_message", "好的，这就改 README 并跑一遍测试。", "done", 400, ""),
				tool("search", "Grep", "install.sh", "done", 700, "README.md:31\nREADME.zh.md:29"),
				edited("README.md", readmeDiff, 500),
				edited("README.zh.md", readmeDiff, 500),
				tool("web", "WebFetch", "https://docs.npmjs.com/cli/v10/commands/npm-install", "done", 2300, "npm install -g installs the package globally."),
				tool("command", "Bash", "pnpm vitest run --reporter verbose src/components", "running", 0, ""),
			},
		},
		"e-su-1": {
			ExecutionSummary: run("e-su-1", "failed", ago(19), 10*time.Minute),
			TaskID:           "t-su", WorkerID: "su", Engine: "claude", Instruction: "把 v1.0.10 的 CHANGELOG 翻译成英文，同步到官网 /changelog 页面",
			Error: "执行超时：超过 10 分钟仍未完成，已停止。",
			Steps: []api.ExecutionStep{
				{Type: "thinking", Text: "先读 CHANGELOG 里 v1.0.10 这一节，再看官网 changelog 页面的数据来源。"},
				tool("read", "Read", "CHANGELOG.md", "done", 300, ""),
				tool("command", "Bash", "pnpm install", "error", 61_000, "Progress: resolved 812, reused 0, downloaded 213, added 0\nERR_PNPM_META_FETCH_FAIL  GET https://registry.npmjs.org/astro: request to https://registry.npmjs.org/astro failed, reason: connect ETIMEDOUT 104.16.2.35:443"),
				{Type: "error", Text: "API Error: 529 overloaded, retrying in 8s"},
				{Type: "text", Text: "安装依赖失败，看起来是网络问题，换个并发数重试一次。"},
				tool("command", "Bash", "pnpm install --network-concurrency 4", "running", 0, ""),
			},
		},
	}
}

func ptr[T any](v T) *T { return &v }

func openProcess(param, run string, expand ...func() string) func(t *testing.T, h *testkit.Harness) {
	return func(t *testing.T, h *testkit.Harness) {
		h.Root.Nav().Push(route.ChatRun(param, run))
		h.Settle()
		h.T.Advance(time.Second)
		h.Settle()
		for _, name := range expand {
			clickAndSettle(t, h, name(), 0)
		}
		h.T.Move(1, wideH-1)
		h.Settle()
	}
}

func thinkingRow(title string) func() string {
	return func() string { return locales.ExecutionThinking() + " · " + title }
}

var processScreens = []screen{
	{name: "process-completed", seed: processTeam, signedIn: true, shots: wideOnly,
		prepare: openProcess("lin", "e-lin-1", thinkingRow("先找出首页用到旧安装命令的地方"),
			func() string { return locales.ExecutionToolTitleEdit("src/components/Hero.astro") },
			func() string { return locales.ExecutionToolTitleCommand("pnpm build") }),
		want: locales.ExecutionStatusCompleted},
	{name: "process-running", seed: processTeam, signedIn: true, shots: wideOnly,
		prepare: openProcess("lin", "e-lin-2",
			func() string { return locales.ExecutionToolTitleEdit("README.md") },
			func() string { return locales.ExecutionToolTitleSearch("install.sh") }),
		want: locales.ExecutionLive},
	{name: "process-failed", seed: processTeam, signedIn: true,
		prepare: openProcess("su", "e-su-1", func() string {
			return locales.ExecutionToolTitleCommand("pnpm install") + " · " + locales.ExecutionStatusFailed()
		}),
		want: locales.ExecutionFailedTitle},
	{name: "process-handoff", seed: processCrew, signedIn: true, prepare: openProcess("bee", "e-links"), shots: wideOnly,
		want: locales.ExecutionRuns},
}
