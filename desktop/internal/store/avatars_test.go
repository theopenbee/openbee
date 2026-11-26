package store_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
)

const (
	firstAvatar     = "mfers/00001"
	firstAvatarPath = "/api/avatars/mfers/00001"
	avatarsPath     = "/api/avatars"
)

func avatarTeam(st *fakeapi.State) {
	team(st)
	refs := st.AddAvatarPack("mfers", api.LocalizedText{"en": "mfers"}, 4)
	st.Workers[0].Avatar = refs[0]
}

func avatarRequests(fake *fakeapi.Server, path string) int {
	return len(fake.Calls(http.MethodGet, path))
}

func TestAvatarImageLoadsOnceAndCaches(t *testing.T) {
	st, fake, fix := loaded(t, avatarTeam)
	if st.AvatarImage("") != nil {
		t.Fatal("an empty reference has an image")
	}
	if st.AvatarImage(firstAvatar) != nil || st.AvatarImage(firstAvatar) != nil {
		t.Fatal("an avatar is ready before it loaded")
	}
	fix.Settle(t)
	src := st.AvatarImage(firstAvatar)
	bitmap, ok := src.(*ui.Bitmap)
	if !ok {
		t.Fatalf("avatar image = %T, want a bitmap", src)
	}
	if w, h := bitmap.Size(); w != 128 || h != 128 {
		t.Fatalf("avatar bitmap = %dx%d, want the 128px picture as served", w, h)
	}
	if st.AvatarImage(firstAvatar) != src {
		t.Fatal("a second lookup decoded the avatar again")
	}
	fix.Settle(t)
	if n := avatarRequests(fake, firstAvatarPath); n != 1 {
		t.Fatalf("avatar fetched %d times, want once", n)
	}
}

func TestAvatarImageFailureRetriesLater(t *testing.T) {
	st, fake, fix := loaded(t, avatarTeam)
	fake.FailNext(http.MethodGet, firstAvatarPath, http.StatusServiceUnavailable, map[string]any{"error": "down"})
	st.AvatarImage(firstAvatar)
	fix.Settle(t)
	if st.AvatarImage(firstAvatar) != nil {
		t.Fatal("a failed avatar has an image")
	}
	fix.Settle(t)
	if n := avatarRequests(fake, firstAvatarPath); n != 1 {
		t.Fatalf("avatar fetched %d times right after failing, want 1", n)
	}
	fix.Clock.Advance(31 * time.Second)
	st.AvatarImage(firstAvatar)
	fix.Settle(t)
	if st.AvatarImage(firstAvatar) == nil {
		t.Fatal("the avatar did not load on the retry")
	}
	if n := avatarRequests(fake, firstAvatarPath); n != 2 {
		t.Fatalf("avatar fetched %d times, want 2", n)
	}
}

func TestAvatarImagesResetWithTheSession(t *testing.T) {
	st, fake, fix := loaded(t, avatarTeam)
	st.AvatarImage(firstAvatar)
	fix.Settle(t)
	st.Logout()
	fix.Settle(t)
	if st.AvatarImage(firstAvatar) != nil {
		t.Fatal("an avatar outlived the session")
	}
	fix.Settle(t)
	if err := call(t, fix, func(done func(error)) { st.Login("admin", "secret", done) }); err != nil {
		t.Fatal(err)
	}
	st.AvatarImage(firstAvatar)
	fix.Settle(t)
	if st.AvatarImage(firstAvatar) == nil {
		t.Fatal("the avatar did not load in the new session")
	}
	if n := avatarRequests(fake, firstAvatarPath); n != 2 {
		t.Fatalf("avatar fetched %d times across two sessions, want 2", n)
	}
}

func TestAvatarsResource(t *testing.T) {
	st, _, fix := loaded(t, avatarTeam)
	st.Avatars.Ensure()
	fix.Settle(t)
	if !st.Avatars.Loaded || len(st.Avatars.Data) != 1 || len(st.Avatars.Data[0].Avatars) != 4 {
		t.Fatalf("avatars = %+v, err %v", st.Avatars.Data, st.Avatars.Err)
	}
}

func TestAvatarsOnAnOlderServerAreEmpty(t *testing.T) {
	st, fake, fix := loaded(t, team)
	fake.FailNext(http.MethodGet, avatarsPath, http.StatusNotFound, map[string]any{"error": "not found"})
	st.Avatars.Ensure()
	fix.Settle(t)
	if !st.Avatars.Loaded || st.Avatars.Err != nil || len(st.Avatars.Data) != 0 {
		t.Fatalf("avatars on an older server = %+v loaded %v err %v, want an empty list", st.Avatars.Data, st.Avatars.Loaded, st.Avatars.Err)
	}
}

func TestAvatarPluginToggleRefreshesWorkersAndPacks(t *testing.T) {
	p, fake, fix, st := activePlugins(t, avatarTeam)
	st.Avatars.Ensure()
	fix.Settle(t)
	workers, packs := avatarRequests(fake, "/api/workers"), avatarRequests(fake, avatarsPath)
	if err := call(t, fix, func(done func(error)) { p.Toggle("mfers", false, done) }); err != nil {
		t.Fatal(err)
	}
	fix.Settle(t)
	if w, _ := st.Worker("w1"); w.Avatar != "" {
		t.Fatalf("worker avatar after disabling its pack = %q, want hidden", w.Avatar)
	}
	if len(st.Avatars.Data) != 0 {
		t.Fatalf("packs after disabling = %+v, want none", st.Avatars.Data)
	}
	if avatarRequests(fake, "/api/workers") == workers || avatarRequests(fake, avatarsPath) == packs {
		t.Fatal("disabling an avatar plugin did not refetch the workers and packs")
	}
	workers = avatarRequests(fake, "/api/workers")
	p.List.Invalidate()
	fix.Settle(t)
	if avatarRequests(fake, "/api/workers") != workers {
		t.Fatal("an unchanged plugin list refetched the workers")
	}
}

func TestChannelPluginToggleKeepsTheWorkers(t *testing.T) {
	p, fake, fix, _ := activePlugins(t, func(st *fakeapi.State) {
		avatarTeam(st)
		pluginCatalog(st)
	})
	workers := avatarRequests(fake, "/api/workers")
	if err := call(t, fix, func(done func(error)) { p.Toggle("telegram", false, done) }); err != nil {
		t.Fatal(err)
	}
	fix.Settle(t)
	if n := avatarRequests(fake, "/api/workers"); n != workers {
		t.Fatalf("toggling a channel fetched the workers %d more times", n-workers)
	}
}
