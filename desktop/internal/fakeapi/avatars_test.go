package fakeapi_test

import (
	"bytes"
	"image/png"
	"net/http"
	"slices"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
)

func TestAvatarPacksAndImages(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	packs, err := s.client.Avatars(c)
	if err != nil || len(packs) != 0 {
		t.Fatalf("Avatars without packs = %+v, %v", packs, err)
	}
	var refs []string
	s.fake.Update(func(st *fakeapi.State) {
		refs = st.AddAvatarPack("mfers", api.LocalizedText{"en": "mfers"}, 3)
		st.AddAvatarPack("blank", nil, 1)
		st.Plugins[len(st.Plugins)-1].Enabled = false
	})
	if !slices.Equal(refs, []string{"mfers/00001", "mfers/00002", "mfers/00003"}) {
		t.Fatalf("refs = %q", refs)
	}
	packs, err = s.client.Avatars(c)
	if err != nil || len(packs) != 1 || packs[0].ID != "mfers" || !slices.Equal(packs[0].Avatars, refs) {
		t.Fatalf("Avatars = %+v, %v; want only the enabled mfers pack", packs, err)
	}
	data, err := s.client.AvatarImage(c, "mfers/00002")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != img.Bounds().Dy() {
		t.Fatalf("avatar image = %v, %v; want a square PNG", img.Bounds(), err)
	}
	if bytes.Equal(data, fakeapi.AvatarPNG("mfers/00001")) {
		t.Fatal("two avatars share the same picture")
	}
	for _, ref := range []string{"mfers/99999", "blank/00001"} {
		if _, err := s.client.AvatarImage(c, ref); apiError(t, err).Code != "avatar_not_found" {
			t.Fatalf("AvatarImage(%s): %v, want avatar_not_found", ref, err)
		}
	}
	req, _ := http.NewRequestWithContext(c, http.MethodGet, s.fake.URL+"/api/avatars/mfers/00001", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous avatar request = %d, want 401", resp.StatusCode)
	}
}

func TestWorkerAvatars(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	if w := mustWorker(t, s, "Plain"); w.Avatar != "" {
		t.Fatalf("worker without packs got avatar %q", w.Avatar)
	}
	var refs []string
	s.fake.Update(func(st *fakeapi.State) { refs = st.AddAvatarPack("mfers", nil, 2) })
	first := mustWorker(t, s, "First")
	second := mustWorker(t, s, "Second")
	if !slices.Contains(refs, first.Avatar) || !slices.Contains(refs, second.Avatar) || first.Avatar == second.Avatar {
		t.Fatalf("new workers got %q and %q, want two different pack avatars", first.Avatar, second.Avatar)
	}
	third := mustWorker(t, s, "Third")
	if !slices.Contains(refs, third.Avatar) {
		t.Fatalf("worker after the pack ran out got %q, want any pack avatar", third.Avatar)
	}
	picked, err := s.client.CreateWorker(c, api.CreateWorkerRequest{Name: "Picked", Avatar: "mfers/00002"})
	if err != nil || picked.Avatar != "mfers/00002" {
		t.Fatalf("create with an avatar = %q, %v", picked.Avatar, err)
	}
	if _, err := s.client.CreateWorker(c, api.CreateWorkerRequest{Name: "Bad", Avatar: "mfers/nope"}); apiError(t, err).Code != "worker_validation" {
		t.Fatalf("create with an unknown avatar: %v", err)
	}

	set := "mfers/00001"
	w, err := s.client.UpdateWorker(c, picked.ID, api.UpdateWorkerRequest{Avatar: &set})
	if err != nil || w.Avatar != set {
		t.Fatalf("set avatar = %q, %v", w.Avatar, err)
	}
	name := "Picked again"
	if w, err = s.client.UpdateWorker(c, picked.ID, api.UpdateWorkerRequest{Name: &name}); err != nil || w.Avatar != set {
		t.Fatalf("an update without avatar changed it to %q, %v", w.Avatar, err)
	}
	bad := "ghost/00001"
	if _, err := s.client.UpdateWorker(c, picked.ID, api.UpdateWorkerRequest{Avatar: &bad}); apiError(t, err).Code != "worker_validation" {
		t.Fatalf("set an unknown avatar: %v", err)
	}
	if err := s.client.DisablePlugin(c, "mfers"); err != nil {
		t.Fatal(err)
	}
	if w, _ = s.client.Worker(c, picked.ID); w.Avatar != "" {
		t.Fatalf("avatar of a disabled pack = %q, want hidden", w.Avatar)
	}
	if err := s.client.EnablePlugin(c, "mfers"); err != nil {
		t.Fatal(err)
	}
	if w, _ = s.client.Worker(c, picked.ID); w.Avatar != set {
		t.Fatalf("avatar after re-enabling = %q, want %q", w.Avatar, set)
	}
	none := ""
	if w, err = s.client.UpdateWorker(c, picked.ID, api.UpdateWorkerRequest{Avatar: &none}); err != nil || w.Avatar != "" {
		t.Fatalf("clear avatar = %q, %v", w.Avatar, err)
	}
}

func TestAvatarPluginRules(t *testing.T) {
	s := signedIn(t)
	c := ctx(t)
	s.fake.Update(func(st *fakeapi.State) {
		st.Catalog.Plugins = []api.CatalogPlugin{{
			Category: "avatar", ID: "mfers", Package: "@openbee/avatars-mfers", Version: "0.1.0",
			Name: api.LocalizedText{"en": "mfers"}, Installable: true,
		}}
	})
	op, err := s.client.InstallPlugin(c, "@openbee/avatars-mfers@0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	s.fake.FinishOperation(op.ID, true, "")
	pack, err := s.client.Plugin(c, "mfers")
	if err != nil || pack.Type != "avatar" || !pack.Enabled || pack.State != "ready" {
		t.Fatalf("installed avatar plugin = %+v, %v", pack.PluginStatus, err)
	}
	typed := "buddy"
	if err := s.client.UpdatePluginSettings(c, "mfers", api.PluginSettingsUpdate{BotName: &typed}); apiError(t, err).Code != "plugin_bot_name_not_supported" {
		t.Fatalf("avatar bot name: %v", err)
	}
	if err := s.client.DisablePlugin(c, "mfers"); err != nil {
		t.Fatal(err)
	}
	if err := s.client.EnablePlugin(c, "mfers"); err != nil {
		t.Fatal(err)
	}
	if pack, _ = s.client.Plugin(c, "mfers"); pack.State != "ready" {
		t.Fatalf("re-enabled avatar plugin state = %q, want ready", pack.State)
	}
}
