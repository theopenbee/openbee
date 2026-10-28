package worker_test

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

func (f *fixture) wantDrawerOpen(panel string) {
	f.t.Helper()
	f.wantPanel(panel)
	f.wantShown(locales.ArcDrawerClose())
}

func TestDrawerDeleteFlowKeepsPanelOpen(t *testing.T) {
	f := newFixture(t, drawerW, route.ChatPanel("w1", route.PanelConstraints), crew)
	f.wantDrawerOpen(route.PanelConstraints)
	openDelete(f, "Alice")
	f.wantDrawerOpen(route.PanelConstraints)
	f.wantShown(locales.WorkersDeleteDialogTitle("Alice"))
	label := locales.WorkersDeleteDialogConfirmNameLabel("Alice")
	f.replaceText(label, "Alice")
	f.wantDrawerOpen(route.PanelConstraints)
	f.wantShown(locales.WorkersDeleteDialogTitle("Alice"))
	f.click(locales.WorkersDeleteDialogDeleteWorkDir())
	f.wantDrawerOpen(route.PanelConstraints)
	f.wantShown(locales.WorkersDeleteDialogTitle("Alice"))
	if got := f.router.Path(); got != "/chat/w1" {
		t.Fatalf("path = %q before confirming, want /chat/w1", got)
	}
	if n := len(f.deletes(alicePath)); n != 0 {
		t.Fatalf("sent %d deletes before confirming", n)
	}
	f.click(locales.WorkersDeleteDialogConfirm())
	calls := f.deletes(alicePath)
	if len(calls) != 1 || calls[0].Query != "delete_work_dir=true" {
		t.Fatalf("DELETE calls = %+v, want one with delete_work_dir=true", calls)
	}
	if got := f.router.Path(); got != "/chat/bee" {
		t.Fatalf("path = %q after deleting, want /chat/bee", got)
	}
	f.wantPanel("")
	f.wantHidden(locales.WorkersDeleteDialogTitle("Alice"), locales.ArcDrawerClose(), locales.WorkerDetailEditInfo())
}

func TestDrawerDeleteCancelKeepsPanelOpen(t *testing.T) {
	f := newFixture(t, drawerW, route.ChatPanel("w1", route.PanelConstraints), crew)
	openDelete(f, "Alice")
	f.click(locales.CommonCancel())
	f.wantHidden(locales.WorkersDeleteDialogTitle("Alice"))
	f.wantDrawerOpen(route.PanelConstraints)
	f.wantShown(locales.WorkerDetailEditInfo())
	if n := len(f.deletes(alicePath)); n != 0 {
		t.Fatalf("cancel sent %d deletes", n)
	}
}

func TestDrawerEscapeClosesOnlyTheDeleteDialog(t *testing.T) {
	f := newFixture(t, drawerW, route.ChatPanel("w1", route.PanelConstraints), crew)
	openDelete(f, "Alice")
	f.wantShown(locales.WorkersDeleteDialogTitle("Alice"))
	f.T.Key(0, ui.KeyEscape)
	f.settle()
	f.wantHidden(locales.WorkersDeleteDialogTitle("Alice"))
	f.wantDrawerOpen(route.PanelConstraints)
	f.wantShown(locales.WorkerDetailEditInfo())
	if got := f.router.Path(); got != "/chat/w1" {
		t.Fatalf("path = %q after Escape, want /chat/w1", got)
	}
	f.T.Key(0, ui.KeyEscape)
	f.settle()
	f.wantPanel("")
	f.wantHidden(locales.ArcDrawerClose())
}

func TestDrawerCreateEngineSelectKeepsPanelOpen(t *testing.T) {
	f := newFixture(t, drawerW, route.ChatPanel(api.BeeConversation, route.PanelNew), withSystemEngine("claude", "claude", "pi"))
	f.wantDrawerOpen(route.PanelNew)
	f.click(locales.WorkersFormEngine())
	f.wantDrawerOpen(route.PanelNew)
	f.click(locales.WorkersEnginesPi())
	f.wantDrawerOpen(route.PanelNew)
	f.wantShown(locales.WorkersEnginesPi(), locales.WorkersFormSectionBasic())
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.wantDrawerOpen(route.PanelNew)
	f.click(locales.WorkersCreateWorker())
	if body := f.lastPost(); body["engine"] != "pi" {
		t.Fatalf("POST body = %v, want the pi engine", body)
	}
	f.wantPanel(route.PanelConstraints)
	f.wantShown(locales.ArcDrawerClose(), locales.WorkerDetailEditInfo())
}

func TestDrawerCreateEscapeClosesOnlyTheEnginePopup(t *testing.T) {
	f := newFixture(t, drawerW, route.ChatPanel(api.BeeConversation, route.PanelNew), withSystemEngine("claude"))
	f.click(locales.WorkersFormEngine())
	f.wantShown(locales.WorkersEnginesPi())
	f.T.Key(0, ui.KeyEscape)
	f.settle()
	f.wantHidden(locales.WorkersEnginesPi())
	f.wantDrawerOpen(route.PanelNew)
}

func TestDrawerDeleteDialogScrimClosesOnlyTheDialog(t *testing.T) {
	f := newFixture(t, drawerW, route.ChatPanel("w1", route.PanelConstraints), crew)
	openDelete(f, "Alice")
	f.wantShown(locales.WorkersDeleteDialogTitle("Alice"))
	f.T.ClickAt(5, 5)
	f.settle()
	f.wantHidden(locales.WorkersDeleteDialogTitle("Alice"))
	f.wantDrawerOpen(route.PanelConstraints)
	f.wantShown(locales.WorkerDetailEditInfo())
	f.T.ClickAt(5, 5)
	f.settle()
	f.wantPanel("")
	f.wantHidden(locales.ArcDrawerClose())
}
