package chat_test

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/views/chat"
)

func mentionTeam() []api.Worker {
	return []api.Worker{worker("w1", "Alice", ""), worker("w2", "Mark Rivera", ""), worker("w3", "Maya Lin", "")}
}

func TestMentionQuery(t *testing.T) {
	for _, tc := range []struct {
		name  string
		text  string
		caret int
		start int
		query string
		ok    bool
	}{
		{"query before the caret", "hi @ma", 6, 3, "ma", true},
		{"space after the at sign", "a@b c", 5, 0, "", false},
		{"bare at sign", "@", 1, 0, "", true},
		{"caret inside the text", "hi @ma tail", 6, 3, "ma", true},
		{"newline after the at sign", "@ma\nx", 5, 0, "", false},
		{"no at sign", "hello", 5, 0, "", false},
		{"runes not bytes", "你好@李", 4, 2, "李", true},
		{"mid-word", "mail@ex", 7, 4, "ex", true},
		{"last at sign wins", "@a @bo", 6, 3, "bo", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, query, ok := chat.MentionQuery(tc.text, tc.caret)
			if ok != tc.ok || ok && (start != tc.start || query != tc.query) {
				t.Fatalf("mentionQuery(%q, %d) = %d, %q, %v; want %d, %q, %v", tc.text, tc.caret, start, query, ok, tc.start, tc.query, tc.ok)
			}
		})
	}
}

func TestMentionCandidates(t *testing.T) {
	var many []api.Worker
	for i := range 10 {
		many = append(many, worker(fmt.Sprintf("m%d", i), fmt.Sprintf("Mark %d", i), ""))
	}
	got := chat.MentionCandidates(many, "MARK")
	if len(got) != 8 || got[0].Name != "Mark 0" || got[7].Name != "Mark 7" {
		t.Fatalf("candidates = %v, want the first 8 workers", got)
	}
	got = chat.MentionCandidates(mentionTeam(), "ma")
	if len(got) != 2 || got[0].Name != "Mark Rivera" || got[1].Name != "Maya Lin" {
		t.Fatalf("candidates for ma = %v", got)
	}
	if got := chat.MentionCandidates(mentionTeam(), ""); len(got) != 3 {
		t.Fatalf("empty query matches %d workers, want all 3", len(got))
	}
	if got := chat.MentionCandidates(mentionTeam(), "zed"); len(got) != 0 {
		t.Fatalf("zed matches %v", got)
	}
}

func TestApplyMention(t *testing.T) {
	text, caret := chat.ApplyMention("hi @ma", 3, 6, "Maya Lin")
	if text != "hi @Maya Lin " || caret != 13 {
		t.Fatalf("applyMention = %q, %d; want %q, 13", text, caret, "hi @Maya Lin ")
	}
	text, caret = chat.ApplyMention("@ma tail", 0, 3, "李娜")
	if text != "@李娜  tail" || caret != 4 {
		t.Fatalf("applyMention mid-text = %q, %d", text, caret)
	}
}

func (f *fixture) startTyping(text string) {
	f.t.Helper()
	f.click(composerLabel())
	f.typeIn(text)
}

func (f *fixture) typeIn(text string) {
	f.T.Type(text)
	f.settleMotion()
}

func (f *fixture) stroke(mods ui.Modifiers, key ui.Key) {
	f.T.Key(mods, key)
	f.settleMotion()
}

func (f *fixture) wantDraft(cid, want string) {
	f.t.Helper()
	if got := f.view.Draft(cid); got != want {
		f.t.Fatalf("draft = %q, want %q", got, want)
	}
}

func TestMentionPopupPicks(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(mentionTeam(), api.BeeConversation))
	f.startTyping("@ma")
	f.wantShown("Mark Rivera", "Maya Lin")
	f.wantHidden("Alice")
	f.stroke(0, ui.KeyDown)
	f.stroke(0, ui.KeyEnter)
	f.wantDraft(api.BeeConversation, "@Maya Lin ")
	f.wantHidden("Mark Rivera", "Maya Lin")
	f.wantNothingSent(api.BeeConversation)
	f.typeIn("hi")
	f.wantDraft(api.BeeConversation, "@Maya Lin hi")
}

func TestMentionKeys(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(mentionTeam(), api.BeeConversation))
	f.startTyping("say @ma")
	f.stroke(0, ui.KeyEscape)
	f.wantHidden("Mark Rivera", "Maya Lin")
	f.typeIn("r")
	f.wantShown("Mark Rivera")
	f.wantHidden("Maya Lin")
	f.stroke(0, ui.KeyUp)
	f.stroke(0, ui.KeyTab)
	f.wantDraft(api.BeeConversation, "say @Mark Rivera ")
	f.wantNothingSent(api.BeeConversation)
}

func TestMentionClickPicks(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(mentionTeam(), api.BeeConversation))
	f.startTyping("ask @")
	f.wantShown("Alice", "Mark Rivera", "Maya Lin")
	f.click("Maya Lin")
	f.wantDraft(api.BeeConversation, "ask @Maya Lin ")
	if !f.T.Focused(composerLabel()) {
		t.Fatal("picking a worker took the focus from the message field")
	}
	f.typeIn("now")
	f.wantDraft(api.BeeConversation, "ask @Maya Lin now")
}

func TestMentionIMEEnterDoesNotPick(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(mentionTeam(), api.BeeConversation))
	f.startTyping("@ma")
	f.wantShown("Mark Rivera")
	f.T.Compose("ni", 2)
	f.settleMotion()
	f.stroke(0, ui.KeyEnter)
	f.wantDraft(api.BeeConversation, "@ma")
	f.wantNothingSent(api.BeeConversation)
	f.T.Compose("", 0)
	f.settleMotion()
	f.stroke(0, ui.KeyEnter)
	f.wantDraft(api.BeeConversation, "@Mark Rivera ")
}

func TestMentionOnlyInBee(t *testing.T) {
	f := newFixture(t, wideW, wideH, "w1", seedChat(mentionTeam(), workerCID))
	f.startTyping("@ma")
	f.wantHidden("Mark Rivera", "Maya Lin")
	f.stroke(0, ui.KeyEnter)
	if got := f.sentContents(workerCID); len(got) != 1 || got[0] != "@ma" {
		t.Fatalf("sent %q, want the typed text", got)
	}
}

func (f *fixture) moveCaretLeft(n int) {
	f.t.Helper()
	for range n {
		f.stroke(0, ui.KeyLeft)
	}
}

func TestMentionCaretMoveDoesNotOpen(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(mentionTeam(), api.BeeConversation))
	f.startTyping("@Maya Lin hi")
	f.wantHidden("Maya Lin")
	f.moveCaretLeft(7)
	f.wantHidden("Maya Lin", "Mark Rivera")
	f.stroke(0, ui.KeyEnter)
	f.wantDraft(api.BeeConversation, "")
	if got := f.sentContents(api.BeeConversation); len(got) != 1 || got[0] != "@Maya Lin hi" {
		t.Fatalf("sent %q, want the draft unchanged", got)
	}
}

func TestMentionTypingInsideReopens(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(mentionTeam(), api.BeeConversation))
	f.startTyping("@Maya Lin hi")
	f.moveCaretLeft(7)
	f.wantHidden("Maya Lin")
	f.typeIn("x")
	f.wantHidden("Maya Lin", "Mark Rivera")
	f.stroke(0, ui.KeyBackspace)
	f.wantShown("Maya Lin")
	f.wantHidden("Mark Rivera")
}

func TestMentionClosesWhenCaretLeaves(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(mentionTeam(), api.BeeConversation))
	f.startTyping("@ma")
	f.wantShown("Mark Rivera", "Maya Lin")
	f.stroke(0, ui.KeyLeft)
	f.wantHidden("Mark Rivera", "Maya Lin")
	f.stroke(0, ui.KeyEnter)
	if got := f.sentContents(api.BeeConversation); len(got) != 1 || got[0] != "@ma" {
		t.Fatalf("sent %q, want the draft unchanged", got)
	}
}

func TestMentionStaysOpenWithoutTyping(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(mentionTeam(), api.BeeConversation))
	f.startTyping("@ma")
	f.settleMotion()
	f.settleMotion()
	f.wantShown("Mark Rivera", "Maya Lin")
	f.stroke(0, ui.KeyDown)
	f.stroke(0, ui.KeyUp)
	f.wantShown("Mark Rivera", "Maya Lin")
}

func TestMentionShiftEnterPicks(t *testing.T) {
	f := newFixture(t, wideW, wideH, api.BeeConversation, seedChat(mentionTeam(), api.BeeConversation))
	f.startTyping("@ma")
	f.wantShown("Mark Rivera", "Maya Lin")
	f.stroke(ui.Shift, ui.KeyEnter)
	f.wantDraft(api.BeeConversation, "@Mark Rivera ")
	f.wantHidden("Mark Rivera", "Maya Lin")
	f.wantNothingSent(api.BeeConversation)
}
