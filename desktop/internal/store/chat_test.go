package store_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/store/storetest"
)

const mediaUploadPath = "/api/local/media"

func conversationPath(cid, action string) string {
	return "/api/local/conversations/" + cid + "/" + action
}

func withHistory(cid string, n int) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		team(st)
		for i := range n {
			st.Messages[cid] = append(st.Messages[cid], api.ChatMessage{
				ID:      fmt.Sprintf("h%02d", i),
				Sender:  api.ChatSender{Type: "worker", ID: "w1"},
				Kind:    "message",
				Content: fmt.Sprintf("message %d", i),
				TS:      int64(1000 + i),
			})
		}
	}
}

func withActiveTask(seed func(st *fakeapi.State)) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		seed(st)
		st.ActiveTasks["worker:w1"] = []api.ActiveTask{{ID: "t1", Instruction: "Build the site", Type: "immediate"}}
	}
}

func ensured(t *testing.T, st *store.Store, fix *storetest.Fixture, cid string) *store.Chat {
	t.Helper()
	ch := st.Chat(cid)
	ch.Ensure()
	fix.Settle(t)
	if !ch.Loaded || ch.Err != nil {
		t.Fatalf("chat %q Loaded=%v Err=%v, want loaded", cid, ch.Loaded, ch.Err)
	}
	return ch
}

func chatIDs(ch *store.Chat) []string {
	ids := make([]string, 0, len(ch.List.Msgs))
	for _, m := range ch.List.Msgs {
		ids = append(ids, m.ID)
	}
	return ids
}

func hasMessage(ch *store.Chat, id string) bool {
	return slices.Contains(chatIDs(ch), id)
}

func lastBody(t *testing.T, fake *fakeapi.Server, method, path string, out any) {
	t.Helper()
	calls := fake.Calls(method, path)
	if len(calls) == 0 {
		t.Fatalf("no %s %s call", method, path)
	}
	body := calls[len(calls)-1].Body
	if len(body) == 0 {
		body = []byte("{}")
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode %s %s body %q: %v", method, path, body, err)
	}
}

func TestChatLoadsLatestPage(t *testing.T) {
	st, fake, fix := loaded(t, withHistory("worker:w1", 60))
	ch := st.Chat("worker:w1")
	if st.Chat("worker:w1") != ch {
		t.Fatal("Chat created a new object for the same conversation")
	}

	ch.Ensure()
	if !ch.Loading {
		t.Fatal("Loading false right after Ensure")
	}
	fix.Settle(t)

	ids := chatIDs(ch)
	if !ch.Loaded || ch.Loading || ch.Err != nil || !ch.List.HasMore {
		t.Fatalf("Loaded=%v Loading=%v Err=%v HasMore=%v, want the latest page with more", ch.Loaded, ch.Loading, ch.Err, ch.List.HasMore)
	}
	if len(ids) != 50 || ids[0] != "h10" || ids[49] != "h59" {
		t.Fatalf("messages = %d (%v … %v), want h10..h59", len(ids), ids[0], ids[len(ids)-1])
	}
	calls := fake.Calls("GET", conversationPath("worker:w1", "messages"))
	if len(calls) != 1 || calls[0].Query != "limit=50" {
		t.Fatalf("message calls = %+v, want one with limit=50", calls)
	}

	ch.Ensure()
	fix.Settle(t)
	if n := countCalls(fake, "GET", conversationPath("worker:w1", "messages")); n != 1 {
		t.Fatalf("message calls after a second Ensure = %d, want 1", n)
	}

	st.Logout()
	fix.Settle(t)
	if st.Chat("worker:w1") != ch || ch.Loaded || len(ch.List.Msgs) != 0 {
		t.Fatalf("after sign-out same=%v Loaded=%v messages=%d, want the cached chat reset", st.Chat("worker:w1") == ch, ch.Loaded, len(ch.List.Msgs))
	}
}

func TestChatLoadOlderPrepends(t *testing.T) {
	st, fake, fix := loaded(t, withHistory("worker:w1", 60))
	ch := ensured(t, st, fix, "worker:w1")
	path := conversationPath("worker:w1", "messages")

	fake.FailNext("GET", path, http.StatusInternalServerError, map[string]any{"error": "boom"})
	ch.LoadOlder()
	fix.Settle(t)
	if ch.OlderErr == nil || ch.LoadingOlder || len(ch.List.Msgs) != 50 || !ch.List.HasMore {
		t.Fatalf("after a failed older load OlderErr=%v LoadingOlder=%v messages=%d HasMore=%v", ch.OlderErr, ch.LoadingOlder, len(ch.List.Msgs), ch.List.HasMore)
	}

	ch.LoadOlder()
	if !ch.LoadingOlder || ch.OlderErr != nil {
		t.Fatalf("LoadingOlder=%v OlderErr=%v right after LoadOlder", ch.LoadingOlder, ch.OlderErr)
	}
	ch.LoadOlder()
	fix.Settle(t)

	calls := fake.Calls("GET", path)
	if len(calls) != 3 || calls[2].Query != "before=1010&limit=50" {
		t.Fatalf("message calls = %+v, want a third with before=1010&limit=50", calls)
	}
	ids := chatIDs(ch)
	if len(ids) != 60 || ids[0] != "h00" || ids[10] != "h10" || ids[59] != "h59" {
		t.Fatalf("messages = %d (%v … %v), want h00..h59", len(ids), ids[0], ids[len(ids)-1])
	}
	if ch.List.HasMore || ch.LoadingOlder || ch.OlderErr != nil {
		t.Fatalf("HasMore=%v LoadingOlder=%v OlderErr=%v after the last page", ch.List.HasMore, ch.LoadingOlder, ch.OlderErr)
	}

	ch.Refresh()
	fix.Settle(t)
	if ids := chatIDs(ch); len(ids) != 60 || ids[0] != "h00" || ch.List.HasMore {
		t.Fatalf("after a refresh messages=%d HasMore=%v, want the older history kept", len(ids), ch.List.HasMore)
	}
}

func TestSendOptimisticThenConfirmed(t *testing.T) {
	st, fake, fix := loaded(t, team)
	ch := ensured(t, st, fix, "worker:w1")
	fix.Clock.Advance(time.Hour)

	var got []error
	ch.Send("hello", nil, func(err error) { got = append(got, err) })
	if len(ch.List.Msgs) != 1 {
		t.Fatalf("messages right after Send = %d, want the optimistic one", len(ch.List.Msgs))
	}
	m := ch.List.Msgs[0]
	if !m.Pending || m.Content != "hello" || m.Sender.Type != "user" || m.Kind != "message" || len(m.ID) != 32 {
		t.Fatalf("optimistic message = %+v", m)
	}
	if want := fix.Clock.Now().UnixMilli(); m.TS != want {
		t.Fatalf("optimistic ts = %d, want %d", m.TS, want)
	}
	if ch.BeeBusy {
		t.Fatal("BeeBusy set by a worker conversation")
	}

	fix.Settle(t)
	if len(got) != 1 || got[0] != nil {
		t.Fatalf("done = %v, want one nil", got)
	}
	var req api.SendMessageRequest
	lastBody(t, fake, "POST", conversationPath("worker:w1", "messages"), &req)
	if req.ID != m.ID || req.Content != "hello" {
		t.Fatalf("send request = %+v, want id %q and content hello", req, m.ID)
	}
	stored := fake.Snapshot().Messages["worker:w1"]
	if len(stored) != 1 || stored[0].ID != m.ID || stored[0].TS == m.TS {
		t.Fatalf("stored = %+v, want the message with a server ts", stored)
	}
	if len(ch.List.Msgs) != 1 || ch.List.Msgs[0].TS != stored[0].TS {
		t.Fatalf("messages after confirm = %+v, want the server ts %d", ch.List.Msgs, stored[0].TS)
	}
	if n := countCalls(fake, "GET", "/api/local/conversations"); n != 2 {
		t.Fatalf("conversation list calls = %d, want a refetch after sending", n)
	}
}

func TestSendFailureRemovesAndReports(t *testing.T) {
	st, fake, fix := loaded(t, team)
	ch := ensured(t, st, fix, "bee")
	fake.FailNext("POST", conversationPath("bee", "messages"), http.StatusInternalServerError, map[string]any{"error": "boom"})

	var got error
	calls := 0
	ch.Send("hi", nil, func(err error) {
		got = err
		calls++
	})
	if len(ch.List.Msgs) != 1 || !ch.BeeBusy {
		t.Fatalf("messages=%d BeeBusy=%v right after Send", len(ch.List.Msgs), ch.BeeBusy)
	}
	fix.Settle(t)

	var apiErr *api.Error
	if calls != 1 || !errors.As(got, &apiErr) || apiErr.Status != http.StatusInternalServerError {
		t.Fatalf("done called %d times with %v, want one 500", calls, got)
	}
	if len(ch.List.Msgs) != 0 || ch.BeeBusy {
		t.Fatalf("messages=%d BeeBusy=%v after the failure, want removed and idle", len(ch.List.Msgs), ch.BeeBusy)
	}
	if n := countCalls(fake, "GET", "/api/local/conversations"); n != 1 {
		t.Fatalf("conversation list calls = %d, want no refetch after a failed send", n)
	}
}

func TestSendAttachmentOnlySendsSpace(t *testing.T) {
	st, fake, fix := loaded(t, team)
	ch := ensured(t, st, fix, "worker:w1")
	media := []string{"upload-1_a.png"}

	if err := call(t, fix, func(done func(error)) { ch.Send("", media, done) }); err != nil {
		t.Fatalf("Send: %v", err)
	}
	var req api.SendMessageRequest
	lastBody(t, fake, "POST", conversationPath("worker:w1", "messages"), &req)
	if req.Content != " " || !slices.Equal(req.MediaPaths, media) {
		t.Fatalf("send request = %+v, want a single space with the attachment", req)
	}
	if len(ch.List.Msgs) != 1 || ch.List.Msgs[0].Content != "" || !slices.Equal(ch.List.Msgs[0].MediaPaths, media) {
		t.Fatalf("messages = %+v, want the attachment with the typed content", ch.List.Msgs)
	}
}

func TestBeeBusyUntilReply(t *testing.T) {
	st, fake, fix := loaded(t, team)
	bee := ensured(t, st, fix, "bee")
	worker := ensured(t, st, fix, "worker:w1")

	if err := call(t, fix, func(done func(error)) { worker.Send("task", nil, done) }); err != nil {
		t.Fatalf("worker Send: %v", err)
	}
	if bee.BeeBusy || worker.BeeBusy {
		t.Fatalf("BeeBusy bee=%v worker=%v after a worker send", bee.BeeBusy, worker.BeeBusy)
	}

	if err := call(t, fix, func(done func(error)) { bee.Send("plan", nil, done) }); err != nil {
		t.Fatalf("bee Send: %v", err)
	}
	if !bee.BeeBusy {
		t.Fatal("BeeBusy false after sending to the Bee")
	}

	fake.Reply("bee", api.ChatMessage{ID: "u1", Sender: api.ChatSender{Type: "user"}, Content: "echo"})
	fake.Reply("bee", api.ChatMessage{ID: "r1", Sender: api.ChatSender{Type: "system"}, Kind: "reset"})
	fake.Reply("worker:w1", api.ChatMessage{ID: "wr1", Content: "worker reply"})
	fix.Await(t, func() bool { return hasMessage(bee, "r1") && hasMessage(worker, "wr1") })
	if !bee.BeeBusy {
		t.Fatal("BeeBusy cleared by a user message, a reset or another conversation")
	}

	fake.Reply("bee", api.ChatMessage{ID: "b1", Content: "on it"})
	fix.Await(t, func() bool { return hasMessage(bee, "b1") })
	if bee.BeeBusy {
		t.Fatal("BeeBusy still set after the Bee replied")
	}
}

func TestIncomingMessageDeduped(t *testing.T) {
	st, fake, fix := loaded(t, team)
	ch := ensured(t, st, fix, "worker:w1")
	path := conversationPath("worker:w1", "messages")

	fake.Reply("worker:w1", api.ChatMessage{ID: "m1", Content: "hi"})
	fix.Await(t, func() bool { return hasMessage(ch, "m1") })
	stored := fake.Snapshot().Messages["worker:w1"][0]
	fake.Emit(api.ChatEvent{Type: "message", ConversationID: "worker:w1", Message: &stored})
	fake.Reply("worker:w1", api.ChatMessage{ID: "m2", Content: "next"})
	fix.Await(t, func() bool { return hasMessage(ch, "m2") })
	if ids := chatIDs(ch); !slices.Equal(ids, []string{"m1", "m2"}) {
		t.Fatalf("messages after a repeated event = %v, want [m1 m2]", ids)
	}
	if !ch.List.Msgs[0].Pending {
		t.Fatal("live message not marked pending")
	}

	dropStream(t, fake, fix, st)
	fix.Clock.Advance(2 * time.Second)
	fix.Await(t, func() bool {
		return st.Online && countCalls(fake, "GET", path) == 2 && !ch.List.Msgs[0].Pending
	})
	if ids := chatIDs(ch); !slices.Equal(ids, []string{"m1", "m2"}) {
		t.Fatalf("messages after the resync refresh = %v, want [m1 m2]", ids)
	}
	if ch.List.Msgs[1].Pending {
		t.Fatal("refreshed message still pending")
	}
}

func TestMarkReadOnIncomingWhenActive(t *testing.T) {
	st, fake, fix := loaded(t, team)
	ch := ensured(t, st, fix, "worker:w1")
	readPath := conversationPath("worker:w1", "read")
	reads := func() int { return countCalls(fake, "POST", readPath) }

	ch.MarkRead(0)
	fake.Reply("worker:w1", api.ChatMessage{ID: "m1", Content: "unseen"})
	fix.Await(t, func() bool { return hasMessage(ch, "m1") })
	if n := reads(); n != 0 {
		t.Fatalf("read calls while inactive = %d, want 0", n)
	}

	st.SetActive("worker:w1")
	ch.MarkRead(0)
	fix.Await(t, func() bool { return summaryOf(t, st, "worker:w1").Unread == 0 })
	var body struct {
		TS int64 `json:"ts"`
	}
	lastBody(t, fake, "POST", readPath, &body)
	if n := reads(); n != 1 || body.TS != 0 {
		t.Fatalf("read calls = %d with ts %d, want one without a ts", n, body.TS)
	}

	fake.Reply("worker:w1", api.ChatMessage{ID: "m2", Content: "seen"})
	fix.Await(t, func() bool { return reads() == 2 })
	lastBody(t, fake, "POST", readPath, &body)
	m2 := fake.Snapshot().Messages["worker:w1"][1]
	if body.TS != m2.TS {
		t.Fatalf("read ts = %d, want the message ts %d", body.TS, m2.TS)
	}

	fake.Reply("worker:w1", api.ChatMessage{ID: "u1", Sender: api.ChatSender{Type: "user"}, Content: "mine"})
	fake.Reply("worker:w1", api.ChatMessage{ID: "r1", Sender: api.ChatSender{Type: "system"}, Kind: "reset"})
	fix.Await(t, func() bool { return hasMessage(ch, "r1") })
	if n := reads(); n != 2 {
		t.Fatalf("read calls after a user message and a reset = %d, want 2", n)
	}

	st.SetForeground(false)
	fake.Reply("worker:w1", api.ChatMessage{ID: "m3", Content: "hidden"})
	fix.Await(t, func() bool { return hasMessage(ch, "m3") })
	if n := reads(); n != 2 {
		t.Fatalf("read calls in the background = %d, want 2", n)
	}
}

func TestClearConflictReturnsActiveTasks(t *testing.T) {
	st, fake, fix := loaded(t, withActiveTask(withHistory("worker:w1", 3)))
	ch := ensured(t, st, fix, "worker:w1")

	var tasks []api.ActiveTask
	var got error
	calls := 0
	ch.Clear(false, func(active []api.ActiveTask, err error) {
		tasks, got = active, err
		calls++
	})
	if !ch.Clearing {
		t.Fatal("Clearing false right after Clear")
	}
	fix.Settle(t)

	var apiErr *api.Error
	if calls != 1 || !errors.As(got, &apiErr) || apiErr.Status != http.StatusConflict {
		t.Fatalf("done called %d times with %v, want one 409", calls, got)
	}
	want := []api.ActiveTask{{ID: "t1", Instruction: "Build the site", Type: "immediate"}}
	if !reflect.DeepEqual(tasks, want) {
		t.Fatalf("active tasks = %+v, want %+v", tasks, want)
	}
	if ch.Clearing {
		t.Fatal("Clearing still set")
	}
	if body := fake.Calls("POST", conversationPath("worker:w1", "clear"))[0].Body; len(body) != 0 {
		t.Fatalf("clear body = %q, want none", body)
	}
	if c, m := countCalls(fake, "GET", "/api/local/conversations"), countCalls(fake, "GET", conversationPath("worker:w1", "messages")); c != 1 || m != 1 {
		t.Fatalf("calls after a conflict conversations=%d messages=%d, want no refetch", c, m)
	}
}

func TestClearForce(t *testing.T) {
	st, fake, fix := loaded(t, withActiveTask(withHistory("worker:w1", 3)))
	ch := ensured(t, st, fix, "worker:w1")

	var tasks []api.ActiveTask
	var got error
	calls := 0
	ch.Clear(true, func(active []api.ActiveTask, err error) {
		tasks, got = active, err
		calls++
	})
	fix.Settle(t)
	if calls != 1 || got != nil || tasks != nil || ch.Clearing {
		t.Fatalf("done called %d times with %v / %v, Clearing=%v", calls, tasks, got, ch.Clearing)
	}
	var body struct {
		Force bool `json:"force"`
	}
	lastBody(t, fake, "POST", conversationPath("worker:w1", "clear"), &body)
	if !body.Force {
		t.Fatal("clear request without force")
	}
	if len(fake.Snapshot().ActiveTasks["worker:w1"]) != 0 {
		t.Fatal("active tasks not cancelled")
	}
	if c, m := countCalls(fake, "GET", "/api/local/conversations"), countCalls(fake, "GET", conversationPath("worker:w1", "messages")); c != 2 || m != 2 {
		t.Fatalf("calls after clearing conversations=%d messages=%d, want one refetch each", c, m)
	}

	fake.Reply("worker:w1", api.ChatMessage{ID: "after", Content: "fresh start"})
	fix.Await(t, func() bool { return hasMessage(ch, "after") })
	resets := 0
	for _, m := range ch.List.Msgs {
		if m.Kind == "reset" {
			resets++
		}
	}
	if ids := chatIDs(ch); resets != 1 || len(ids) != 5 || ch.List.Msgs[3].Kind != "reset" {
		t.Fatalf("messages after clearing = %v with %d resets, want history, one reset, then the reply", ids, resets)
	}
}

func TestStop(t *testing.T) {
	st, fake, fix := loaded(t, team)
	ch := st.Chat("worker:w1")

	var got error
	calls := 0
	ch.Stop(func(err error) {
		got = err
		calls++
	})
	if !ch.Stopping {
		t.Fatal("Stopping false right after Stop")
	}
	fix.Settle(t)
	if calls != 1 || got != nil || ch.Stopping {
		t.Fatalf("done called %d times with %v, Stopping=%v", calls, got, ch.Stopping)
	}
	if n := countCalls(fake, "POST", conversationPath("worker:w1", "stop")); n != 1 {
		t.Fatalf("stop calls = %d, want 1", n)
	}
	if n := countCalls(fake, "GET", "/api/local/conversations"); n != 2 {
		t.Fatalf("conversation list calls = %d, want a refetch after stopping", n)
	}

	bee := st.Chat("bee")
	var apiErr *api.Error
	if err := call(t, fix, bee.Stop); !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
		t.Fatalf("bee Stop = %v, want 400", err)
	}
	if bee.Stopping {
		t.Fatal("Stopping still set after a failure")
	}
	if n := countCalls(fake, "GET", "/api/local/conversations"); n != 2 {
		t.Fatalf("conversation list calls = %d, want no refetch after a failed stop", n)
	}
}

func TestUploadReportsFailures(t *testing.T) {
	st, fake, fix := loaded(t, team)
	dir := t.TempDir()
	contents := map[string]string{"a.txt": "alpha", "b.txt": "beta", "c.txt": "gamma"}
	var paths []string
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(contents[name]), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	fake.FailNext("POST", mediaUploadPath, http.StatusInternalServerError, map[string]any{"error": "disk full"})

	var uploaded []string
	failed, calls := -1, 0
	st.Upload(paths, func(ok []string, n int) {
		uploaded, failed = ok, n
		calls++
	})
	fix.Settle(t)

	if calls != 1 || failed != 1 || len(uploaded) != 2 {
		t.Fatalf("done called %d times with %v and %d failed, want two uploaded and one failed", calls, uploaded, failed)
	}
	media := fake.Snapshot().Media
	last := -1
	for _, p := range uploaded {
		_, name, _ := strings.Cut(p, "_")
		if string(media[p]) != contents[name] {
			t.Fatalf("server file %q = %q, want %q", p, media[p], contents[name])
		}
		i := slices.Index([]string{"a.txt", "b.txt", "c.txt"}, name)
		if i <= last {
			t.Fatalf("uploaded = %v, want the selection order", uploaded)
		}
		last = i
	}
	if n := countCalls(fake, "POST", mediaUploadPath); n != 3 {
		t.Fatalf("upload calls = %d, want 3", n)
	}

	calls = 0
	st.Upload([]string{filepath.Join(dir, "missing.txt")}, func(ok []string, n int) {
		uploaded, failed = ok, n
		calls++
	})
	fix.Settle(t)
	if calls != 1 || failed != 1 || len(uploaded) != 0 {
		t.Fatalf("missing file: done called %d times with %v and %d failed", calls, uploaded, failed)
	}
	if n := countCalls(fake, "POST", mediaUploadPath); n != 3 {
		t.Fatalf("upload calls = %d, want no request for a missing file", n)
	}
}

func TestUploadBytes(t *testing.T) {
	st, fake, fix := loaded(t, team)

	var path string
	var got error
	calls := 0
	st.UploadBytes("pasted-1.png", []byte("png bytes"), func(p string, err error) {
		path, got = p, err
		calls++
	})
	fix.Settle(t)
	if calls != 1 || got != nil || !strings.HasSuffix(path, "_pasted-1.png") {
		t.Fatalf("done called %d times with %q / %v", calls, path, got)
	}
	if data := fake.Snapshot().Media[path]; string(data) != "png bytes" {
		t.Fatalf("server file = %q, want the pasted bytes", data)
	}
}

func TestMediaFileCached(t *testing.T) {
	st, fake, fix := loaded(t, func(st *fakeapi.State) {
		team(st)
		st.Media["m1_a.png"] = []byte("image bytes")
	})
	type result struct {
		path string
		err  error
	}
	var got []result
	collect := func(p string, err error) { got = append(got, result{p, err}) }

	st.MediaFile("m1_a.png", collect)
	st.MediaFile("m1_a.png", collect)
	fix.Settle(t)
	if len(got) != 2 || got[0].err != nil || got[1] != got[0] {
		t.Fatalf("results = %+v, want two identical successes", got)
	}
	local := got[0].path
	if filepath.Base(local) != "m1_a.png" || filepath.Base(filepath.Dir(local)) != "media" {
		t.Fatalf("local path = %q, want <cache>/media/m1_a.png", local)
	}
	if data, err := os.ReadFile(local); err != nil || string(data) != "image bytes" {
		t.Fatalf("cached file = %q, %v", data, err)
	}
	if n := countCalls(fake, "GET", "/api/local/media/m1_a.png"); n != 1 {
		t.Fatalf("download calls = %d, want 1 for concurrent requests", n)
	}

	st.MediaFile("nested/m1_a.png", collect)
	fix.Settle(t)
	if len(got) != 3 || got[2] != got[0] {
		t.Fatalf("cache hit result = %+v, want %+v", got[len(got)-1], got[0])
	}
	if n := countCalls(fake, "GET", "/api/local/media/m1_a.png"); n != 1 {
		t.Fatalf("download calls = %d, want no request on a cache hit", n)
	}

	st.MediaFile("gone.png", collect)
	fix.Settle(t)
	var apiErr *api.Error
	if len(got) != 4 || got[3].path != "" || !errors.As(got[3].err, &apiErr) || apiErr.Status != http.StatusNotFound {
		t.Fatalf("missing media result = %+v, want a 404", got[len(got)-1])
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(local), "gone.png")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing media left a cache file: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(local)); len(entries) != 1 {
		t.Fatalf("cache entries = %v, want only m1_a.png", entries)
	}
}
