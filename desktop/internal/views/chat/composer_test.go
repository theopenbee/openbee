package chat_test

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	mediaPath      = "/api/local/media"
	composerCap    = 160
	composerMin    = 48
	composerLines  = 10
	narrowComposer = 600
)

func composerLabel() string {
	return locales.ChatMessageLabel()
}

func messagesPath(cid string) string {
	return "/api/local/conversations/" + cid + "/messages"
}

func (f *fixture) sent(cid string) []api.SendMessageRequest {
	f.t.Helper()
	var out []api.SendMessageRequest
	for _, call := range f.Fake.Calls(http.MethodPost, messagesPath(cid)) {
		var req api.SendMessageRequest
		if err := json.Unmarshal(call.Body, &req); err != nil {
			f.t.Fatalf("decode sent message: %v", err)
		}
		out = append(out, req)
	}
	return out
}

func (f *fixture) sentContents(cid string) []string {
	var out []string
	for _, req := range f.sent(cid) {
		out = append(out, req.Content)
	}
	return out
}

func (f *fixture) wantNothingSent(cid string) {
	f.t.Helper()
	if got := f.sent(cid); len(got) != 0 {
		f.t.Fatalf("sent %d messages, want none: %+v", len(got), got)
	}
}

func (f *fixture) uploads() int {
	return len(f.Fake.Calls(http.MethodPost, mediaPath))
}

func (f *fixture) dropOnComposer(paths ...string) {
	f.t.Helper()
	r, ok := f.T.Find(composerLabel())
	if !ok {
		f.t.Fatalf("no message field; texts %q", f.T.Texts())
	}
	if !f.T.DropFiles(r.X+r.W/2, r.Y+r.H/2, paths...) {
		f.t.Fatal("the composer did not take the dropped files")
	}
	f.settleMotion()
}

func (f *fixture) wantAttachments(cid string, suffixes ...string) []string {
	f.t.Helper()
	got := f.view.Attachments(cid)
	if len(got) != len(suffixes) {
		f.t.Fatalf("attachments = %q, want %d ending in %q", got, len(suffixes), suffixes)
	}
	for i, suffix := range suffixes {
		if !strings.HasSuffix(got[i], suffix) {
			f.t.Fatalf("attachment %d = %q, want it to end in %q", i, got[i], suffix)
		}
		f.wantShown(locales.ChatRemoveNamedAttachment(got[i]))
	}
	return got
}

func tempFiles(t *testing.T, names ...string) []string {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = filepath.Join(dir, name)
		if err := os.WriteFile(paths[i], []byte("content of "+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

func TestEnterSends(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.startTyping("hello there")
	f.stroke(0, ui.KeyEnter)
	f.wantDraft(workerCID, "")
	if got := f.sentContents(workerCID); len(got) != 1 || got[0] != "hello there" {
		t.Fatalf("sent %q, want the typed text", got)
	}
	f.wantShown("hello there")
}

func TestShiftEnterNewline(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.startTyping("first")
	f.stroke(ui.Shift, ui.KeyEnter)
	f.typeIn("second")
	f.wantDraft(workerCID, "first\nsecond")
	f.wantNothingSent(workerCID)
	f.stroke(0, ui.KeyEnter)
	if got := f.sentContents(workerCID); len(got) != 1 || got[0] != "first\nsecond" {
		t.Fatalf("sent %q", got)
	}
}

func TestComposerGrowsToCap(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.startTyping("one")
	r, _ := f.T.Find(composerLabel())
	if r.H != composerMin {
		t.Fatalf("one line is %v high, want %v", r.H, composerMin)
	}
	for range composerLines {
		f.stroke(ui.Shift, ui.KeyEnter)
		f.typeIn("more")
	}
	r, _ = f.T.Find(composerLabel())
	if r.H != composerCap {
		t.Fatalf("%d lines are %v high, want the %v cap", composerLines+1, r.H, composerCap)
	}
}

func TestComposerIMEEnterDoesNotSend(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.startTyping("hi")
	f.T.Compose("ni", 2)
	f.settleMotion()
	f.stroke(0, ui.KeyEnter)
	f.wantNothingSent(workerCID)
	f.wantDraft(workerCID, "hi")
	f.T.Type("你")
	f.settleMotion()
	f.wantDraft(workerCID, "hi你")
	f.stroke(0, ui.KeyEnter)
	if got := f.sentContents(workerCID); len(got) != 1 || got[0] != "hi你" {
		t.Fatalf("sent %q after the composition ended", got)
	}
}

func TestSendDisabledWhenEmpty(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.click(locales.ChatSend())
	f.startTyping("   ")
	f.click(locales.ChatSend())
	f.stroke(0, ui.KeyEnter)
	f.wantNothingSent(workerCID)
	f.wantDraft(workerCID, "   ")

	f.dropOnComposer(tempFiles(t, "notes.txt")...)
	f.click(locales.ChatSend())
	got := f.sent(workerCID)
	if len(got) != 1 || got[0].Content != " " || len(got[0].MediaPaths) != 1 || !strings.HasSuffix(got[0].MediaPaths[0], "notes.txt") {
		t.Fatalf("sent %+v, want only the attachment", got)
	}
	f.wantDraft(workerCID, "")
	f.wantAttachments(workerCID)
}

func TestSendFailureRestoresText(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.dropOnComposer(tempFiles(t, "plan.pdf")...)
	f.startTyping("ship it")
	f.Fake.FailNext(http.MethodPost, messagesPath(workerCID), http.StatusInternalServerError, map[string]string{"error": "database is locked"})
	f.T.Key(0, ui.KeyEnter)
	f.wantDraft(workerCID, "")
	f.T.Type("and tag it")
	f.settleMotion()
	f.wantDraft(workerCID, "ship it\nand tag it")
	f.wantAttachments(workerCID, "plan.pdf")
	f.wantShown(locales.ChatSendFailed(), "database is locked")
	if n := len(f.Store.Chat(workerCID).List.Msgs); n != 0 {
		t.Fatalf("%d messages left in the log after the failure", n)
	}
}

func TestPasteImageUploads(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.T.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, pngImage(t, 4, 4)))))
	f.startTyping("look")
	f.T.Command("paste")
	f.settleMotion()
	if n := f.uploads(); n != 1 {
		t.Fatalf("%d uploads, want 1", n)
	}
	body := string(f.Fake.Calls(http.MethodPost, mediaPath)[0].Body)
	if !strings.Contains(body, `filename="pasted-`) || !strings.Contains(body, `.png"`) {
		t.Fatalf("upload is not named pasted-<unix>.png: %.200q", body)
	}
	f.wantAttachments(workerCID, ".png")
	f.wantDraft(workerCID, "look")
}

type logSink struct {
	mu   sync.Mutex
	text strings.Builder
}

func (l *logSink) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.Write(p)
}

func (l *logSink) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.String()
}

func TestPasteCancelledBySessionResetIsSilent(t *testing.T) {
	sink := &logSink{}
	log.SetOutput(sink)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.T.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, pngImage(t, 4, 4)))))
	f.startTyping("look")
	f.T.Command("paste")
	f.Store.Logout()
	f.Settle()
	if got := sink.String(); strings.Contains(got, "upload pasted image") {
		t.Fatalf("a cancelled paste upload was logged: %q", got)
	}
}

func TestPasteTextStillInserts(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.T.SetClipboard(" pasted")
	f.startTyping("text")
	f.T.Command("paste")
	f.settleMotion()
	f.wantDraft(workerCID, "text pasted")
	if n := f.uploads(); n != 0 {
		t.Fatalf("%d uploads for a text paste", n)
	}
}

func TestDropFilesUploads(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.dropOnComposer(tempFiles(t, "report.pdf", "data.csv")...)
	if n := f.uploads(); n != 2 {
		t.Fatalf("%d uploads, want 2", n)
	}
	got := f.view.Attachments(workerCID)
	if len(got) != 2 {
		t.Fatalf("attachments = %q", got)
	}
	for _, name := range []string{"report.pdf", "data.csv"} {
		if !strings.HasSuffix(got[0], name) && !strings.HasSuffix(got[1], name) {
			t.Fatalf("attachments %q miss %s", got, name)
		}
	}
	f.wantShown(locales.ChatRemoveNamedAttachment(got[0]), locales.ChatRemoveNamedAttachment(got[1]))
}

func TestUploadFailureAlert(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	paths := append(tempFiles(t, "kept.txt"), filepath.Join(t.TempDir(), "missing.txt"))
	f.dropOnComposer(paths...)
	title := locales.ChatUploadError(1)
	f.wantShown(title)
	f.wantAttachments(workerCID, "kept.txt")
	f.click(locales.ArcAlertDismiss(title))
	f.wantHidden(title)
	f.wantAttachments(workerCID, "kept.txt")
}

func TestRemoveChip(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.dropOnComposer(tempFiles(t, "a.txt", "b.txt")...)
	got := f.view.Attachments(workerCID)
	if len(got) != 2 {
		t.Fatalf("attachments = %q", got)
	}
	f.click(locales.ChatRemoveNamedAttachment(got[0]))
	if left := f.view.Attachments(workerCID); len(left) != 1 || left[0] != got[1] {
		t.Fatalf("attachments after removing %q = %q", got[0], left)
	}
	f.wantHidden(locales.ChatRemoveNamedAttachment(got[0]))
	f.click(locales.ChatRemoveNamedAttachment(got[1]))
	f.click(locales.ChatSend())
	f.wantNothingSent(workerCID)
}

func TestAttachOpensPicker(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	picked := tempFiles(t, "brief.md")
	var calls atomic.Int32
	f.view.SetFilePicker(func() ([]string, error) {
		calls.Add(1)
		return picked, nil
	})
	f.click(locales.ChatUploadFile())
	if n := calls.Load(); n != 1 {
		t.Fatalf("picker opened %d times", n)
	}
	f.wantAttachments(workerCID, "brief.md")

	f.view.SetFilePicker(func() ([]string, error) { return nil, errors.New("dialog unavailable") })
	f.click(locales.ChatUploadFile())
	f.view.SetFilePicker(func() ([]string, error) { return nil, nil })
	f.click(locales.ChatUploadFile())
	f.wantAttachments(workerCID, "brief.md")
	if n := f.uploads(); n != 1 {
		t.Fatalf("%d uploads, want only the first pick", n)
	}
}

func TestComposerHint(t *testing.T) {
	bee := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(team(), api.BeeConversation))
	bee.wantShown(locales.ChatComposerHint())
	worker := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	worker.wantShown(locales.ChatComposerHintPlain())
	worker.wantHidden(locales.ChatComposerHint())
	narrow := newFixture(t, narrowComposer, wideH, "w1", seedChat(team(), workerCID))
	narrow.wantHidden(locales.ChatComposerHintPlain())
	narrow.wantShown(locales.ChatUploadFile(), locales.ChatSend())
}

func TestDraftsArePerConversation(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(team(), workerCID))
	f.startTyping("for alice")
	f.router.Push(route.Chat("w2"))
	f.settleMotion()
	f.startTyping("for bob")
	f.router.Push(route.Chat("w1"))
	f.settleMotion()
	f.wantDraft(workerCID, "for alice")
	f.wantDraft("worker:w2", "for bob")
}
