package worker_test

import (
	"image/color"
	"net/http"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

func avatarCrew(count int) func(st *fakeapi.State) {
	return func(st *fakeapi.State) {
		crew(st)
		refs := st.AddAvatarPack("mfers", api.LocalizedText{"en": "mfers"}, count)
		st.Workers[0].Avatar = refs[0]
		st.Workers[1].Avatar = refs[1]
	}
}

func openPicker(t *testing.T, seed func(st *fakeapi.State)) *fixture {
	t.Helper()
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), seed)
	f.click(locales.WorkerDetailEditInfo())
	f.click(locales.WorkersFormAvatarChange())
	f.wantShown(locales.WorkersFormAvatarDialogTitle(), locales.WorkersFormAvatarDialogDescription("Alice"),
		locales.WorkersFormAvatarRandom(), locales.WorkersFormAvatarInitials())
	return f
}

func (f *fixture) ringAt(name string) color.RGBA {
	f.t.Helper()
	r, ok := f.T.Find(name)
	if !ok {
		f.t.Fatalf("no %q; texts %q", name, f.T.Texts())
	}
	return f.T.Image().RGBAAt(int(r.X+r.W/2), int(r.Y+1))
}

func sameColor(got color.RGBA, want ui.Color) bool {
	diff := func(a, b uint8) int { return max(int(a)-int(b), int(b)-int(a)) }
	return diff(got.R, want.R) <= 8 && diff(got.G, want.G) <= 8 && diff(got.B, want.B) <= 8
}

func TestEditAvatarPickerSavesTheChoice(t *testing.T) {
	f := openPicker(t, avatarCrew(6))
	tk := theme.Light()
	if got := f.ringAt(locales.WorkersFormAvatarOption("00001")); !sameColor(got, tk.Accent) {
		t.Fatalf("ring of the current avatar = %v, want the accent %+v", got, tk.Accent)
	}
	if got := f.ringAt(locales.WorkersFormAvatarOption("00004")); sameColor(got, tk.Accent) {
		t.Fatal("an avatar that is not chosen has the selected ring")
	}
	f.click(locales.WorkersFormAvatarOption("00004"))
	f.wantHidden(locales.WorkersFormAvatarDialogTitle())
	if n := len(f.puts(alicePath)); n != 0 {
		t.Fatalf("picking an avatar sent %d updates before saving", n)
	}
	f.click(locales.WorkerDetailSaveInfo())
	if body := f.lastPut(alicePath); body["avatar"] != "mfers/00004" {
		t.Fatalf("PUT body = %v, want the chosen avatar", body)
	}
	if w := f.Fake.Snapshot().Workers[0]; w.Avatar != "mfers/00004" {
		t.Fatalf("saved avatar = %q", w.Avatar)
	}
}

func TestEditAvatarInitialsClearsTheAvatar(t *testing.T) {
	f := openPicker(t, avatarCrew(3))
	f.click(locales.WorkersFormAvatarInitials())
	f.wantHidden(locales.WorkersFormAvatarDialogTitle())
	f.click(locales.WorkerDetailSaveInfo())
	body := f.lastPut(alicePath)
	if avatar, sent := body["avatar"]; !sent || avatar != "" {
		t.Fatalf("PUT body = %v, want an empty avatar to clear it", body)
	}
}

func TestEditAvatarRandomPicksAFreeAvatar(t *testing.T) {
	f := openPicker(t, avatarCrew(3))
	f.click(locales.WorkersFormAvatarRandom())
	f.wantHidden(locales.WorkersFormAvatarDialogTitle())
	f.click(locales.WorkerDetailSaveInfo())
	if body := f.lastPut(alicePath); body["avatar"] != "mfers/00003" {
		t.Fatalf("PUT body = %v, want the only avatar no worker uses", body)
	}
}

func TestEditWithoutAvatarChangeOmitsTheAvatar(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), avatarCrew(3))
	f.click(locales.WorkerDetailEditInfo())
	f.click(locales.WorkersFormAvatarChange())
	f.click(locales.WorkersFormAvatarOption("00001"))
	f.replaceText(locales.WorkersFormDescription(), "Ships every release")
	f.click(locales.WorkerDetailSaveInfo())
	body := f.lastPut(alicePath)
	if _, sent := body["avatar"]; sent {
		t.Fatalf("PUT body = %v, want no avatar for an unchanged one", body)
	}
}

func TestEditAvatarWithoutPacksShowsTheHint(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	f.click(locales.WorkerDetailEditInfo())
	f.wantShown(locales.WorkersFormAvatar(), locales.WorkersFormAvatarNone())
	f.wantHidden(locales.WorkersFormAvatarChange())
}

func TestEditAvatarLoadErrorShowsAndRetries(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), avatarCrew(3))
	f.Fake.FailNext(http.MethodGet, "/api/avatars", http.StatusInternalServerError, map[string]any{"error": "boom"})
	f.click(locales.WorkerDetailEditInfo())
	f.wantShown(locales.WorkersFormAvatarLoadError())
	f.wantHidden(locales.WorkersFormAvatarChange())
	f.click(locales.CommonCancel())
	f.click(locales.WorkerDetailEditInfo())
	f.wantShown(locales.WorkersFormAvatarChange())
}

func TestPickerHeadsEachPackWhenThereAreSeveral(t *testing.T) {
	f := openPicker(t, func(st *fakeapi.State) {
		avatarCrew(3)(st)
		st.AddAvatarPack("robots", api.LocalizedText{"en": "Robots"}, 2)
	})
	f.wantShown("mfers", "Robots", locales.WorkersFormAvatarOption("00002"))
	g := openPicker(t, avatarCrew(3))
	g.wantHidden("mfers")
}

func TestPickerLoadsOnlyTheTilesItShows(t *testing.T) {
	f := openPicker(t, avatarCrew(300))
	opened := len(f.Fake.Calls(http.MethodGet, "/api/avatars/mfers/"))
	if opened == 0 || opened > 120 {
		t.Fatalf("opening the picker fetched %d avatars, want only the visible ones", opened)
	}
	if !f.shows(locales.WorkersFormAvatarOption("00001")) || f.shows(locales.WorkersFormAvatarOption("00300")) {
		t.Fatal("the picker does not start at the top of a long pack")
	}
	r, _ := f.T.Find(locales.WorkersFormAvatarOption("00009"))
	f.T.Scroll(r.X, r.Y, 0, 1200)
	f.settle()
	scrolled := len(f.Fake.Calls(http.MethodGet, "/api/avatars/mfers/"))
	if scrolled <= opened || scrolled > opened+120 {
		t.Fatalf("scrolling fetched %d more avatars, want the ones it showed", scrolled-opened)
	}
}

func TestEditAvatarRefreshesStalePacks(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), crew)
	f.click(locales.WorkerDetailEditInfo())
	f.wantShown(locales.WorkersFormAvatarNone())
	f.click(locales.CommonCancel())
	f.Fake.Update(func(st *fakeapi.State) { st.AddAvatarPack("mfers", nil, 3) })
	f.click(locales.WorkerDetailEditInfo())
	f.wantShown(locales.WorkersFormAvatarNone())
	f.click(locales.CommonCancel())
	f.Fix.Clock.Advance(time.Minute)
	f.click(locales.WorkerDetailEditInfo())
	f.wantShown(locales.WorkersFormAvatarChange())
}
