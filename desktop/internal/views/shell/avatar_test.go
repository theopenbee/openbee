package shell_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

func TestRowsShowWorkerAvatarImages(t *testing.T) {
	now := time.Now()
	workers := team()
	workers[1].Status = "working"
	f := newFixture(t, wideW, wideH, route.Chat("bee"), func(st *fakeapi.State) {
		seed(workers, conversation("w1", said("worker", "done", now), 0), conversation("w2", said("worker", "busy", now), 0))(st)
		refs := st.AddAvatarPack("mfers", nil, 4)
		st.Workers[0].Avatar = refs[0]
		st.Workers[1].Avatar = refs[1]
	})
	f.wantShown("Alice", "Bob", "Carol", locales.StatusesWorking())
	if f.T.HasText("AL") || f.T.HasText("BO") {
		t.Fatalf("rows with avatar images still show initials; texts %q", f.T.Texts())
	}
	if !f.T.HasText("CA") {
		t.Fatal("a worker without an avatar lost its initials")
	}
	for _, path := range []string{"/api/avatars/mfers/00001", "/api/avatars/mfers/00002"} {
		if n := len(f.Fake.Calls(http.MethodGet, path)); n != 1 {
			t.Fatalf("GET %s ran %d times, want once", path, n)
		}
	}
}

func TestRowAvatarFallsBackToInitialsWhenTheImageFails(t *testing.T) {
	f := newFixture(t, wideW, wideH, route.ChatPath, nil)
	f.Fake.Update(func(st *fakeapi.State) {
		seed(team())(st)
		refs := st.AddAvatarPack("mfers", nil, 2)
		st.Workers[0].Avatar = refs[0]
	})
	f.Fake.FailNext(http.MethodGet, "/api/avatars/mfers/00001", http.StatusInternalServerError, map[string]any{"error": "boom"})
	f.Store.Workers.Invalidate()
	f.Store.Conversations.Invalidate()
	f.Settle()
	f.wantShown("Alice")
	if !f.T.HasText("AL") {
		t.Fatalf("a worker whose image failed does not show initials; texts %q", f.T.Texts())
	}
	if n := len(f.Fake.Calls(http.MethodGet, "/api/avatars/mfers/00001")); n != 1 {
		t.Fatalf("the failed image was fetched %d times, want once", n)
	}
}
