package desktop

import (
	"testing"

	"github.com/egoist/mygo"
)

func TestTitleBarStyle(t *testing.T) {
	for _, tc := range []struct {
		goos string
		want mygo.TitleBarStyle
	}{
		{"darwin", mygo.TitleBarHidden},
		{"windows", mygo.TitleBarDefault},
		{"linux", mygo.TitleBarDefault},
	} {
		if got := titleBarStyle(tc.goos); got != tc.want {
			t.Errorf("titleBarStyle(%q) = %q, want %q", tc.goos, got, tc.want)
		}
	}
}

func TestConversationFromNotification(t *testing.T) {
	for _, tc := range []struct {
		id      string
		wantCID string
		wantOK  bool
	}{
		{"notifications-enabled", "", false},
		{"c1/m1", "c1", true},
		{"/m1", "", false},
		{"bee/x", "bee", true},
		{"", "", false},
	} {
		cid, ok := conversationFromNotification(tc.id)
		if cid != tc.wantCID || ok != tc.wantOK {
			t.Errorf("conversationFromNotification(%q) = (%q, %v), want (%q, %v)", tc.id, cid, ok, tc.wantCID, tc.wantOK)
		}
	}
}

func TestPostQueuesUntilWindowExists(t *testing.T) {
	a := &App{}
	a.post(func() {})
	a.post(func() {})
	if len(a.queued) != 2 {
		t.Fatalf("queued = %d, want 2", len(a.queued))
	}
}
