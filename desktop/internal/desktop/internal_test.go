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

func TestShowsTray(t *testing.T) {
	for goos, want := range map[string]bool{"darwin": true, "windows": true, "linux": false} {
		if got := showsTray(goos); got != want {
			t.Errorf("showsTray(%q) = %v, want %v", goos, got, want)
		}
	}
}

func TestClosesToTray(t *testing.T) {
	for _, tc := range []struct {
		goos     string
		tray     bool
		quitting bool
		want     bool
	}{
		{"windows", true, false, true},
		{"windows", true, true, false},
		{"windows", false, false, false},
		{"darwin", true, false, false},
		{"linux", false, false, false},
	} {
		if got := closesToTray(tc.goos, tc.tray, tc.quitting); got != tc.want {
			t.Errorf("closesToTray(%q, tray=%v, quitting=%v) = %v, want %v", tc.goos, tc.tray, tc.quitting, got, tc.want)
		}
	}
}

func TestIconPixels(t *testing.T) {
	for _, tc := range []struct {
		points int
		scale  float64
		want   int
	}{
		{16, 1, 16},
		{16, 1.25, 20},
		{16, 1.5, 24},
		{32, 2, 64},
		{16, 0, 16},
	} {
		if got := iconPixels(tc.points, tc.scale); got != tc.want {
			t.Errorf("iconPixels(%d, %v) = %d, want %d", tc.points, tc.scale, got, tc.want)
		}
	}
}
