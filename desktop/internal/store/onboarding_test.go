package store_test

import (
	"net/http"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/store"
	"github.com/theopenbee/openbee/desktop/internal/store/storetest"
)

const (
	onboardingPath = "/api/onboarding"
	skipPath       = "/api/onboarding/skip"
)

func freshServer(st *fakeapi.State) { st.OnboardingSkipped = false }

func onboarding(t *testing.T, seed func(st *fakeapi.State)) (*store.Store, *fakeapi.Server, *storetest.Fixture) {
	t.Helper()
	st, fake, fix := signedIn(t, seed)
	if st.OnboardingKnown() {
		t.Fatal("onboarding status known before it was fetched")
	}
	st.Onboarding.Ensure()
	fix.Settle(t)
	if !st.OnboardingKnown() {
		t.Fatal("onboarding status unknown after the fetch settled")
	}
	return st, fake, fix
}

func TestOnboardingPendingOnFreshServer(t *testing.T) {
	st, fake, _ := onboarding(t, freshServer)
	if !st.OnboardingPending() {
		t.Fatal("a server without engines and without a skip is not pending")
	}
	if n := len(fake.Calls(http.MethodGet, onboardingPath)); n != 1 {
		t.Fatalf("onboarding reads = %d, want 1", n)
	}
}

func TestOnboardingNotPendingOnceSkipped(t *testing.T) {
	st, _, _ := onboarding(t, nil)
	if st.OnboardingPending() {
		t.Fatal("a server that skipped onboarding is pending")
	}
}

func TestOnboardingErrorCountsAsDone(t *testing.T) {
	st, fake, fix := signedIn(t, freshServer)
	fake.FailNext(http.MethodGet, onboardingPath, http.StatusInternalServerError, map[string]any{"error": "boom"})
	st.Onboarding.Ensure()
	fix.Settle(t)
	if !st.OnboardingKnown() || st.OnboardingPending() {
		t.Fatalf("known=%v pending=%v after a failed fetch, want known and not pending", st.OnboardingKnown(), st.OnboardingPending())
	}
}

func TestSkipOnboardingPostsAndFinishes(t *testing.T) {
	st, fake, fix := onboarding(t, freshServer)
	if err := call(t, fix, st.SkipOnboarding); err != nil {
		t.Fatalf("SkipOnboarding: %v", err)
	}
	if n := len(fake.Calls(http.MethodPost, skipPath)); n != 1 {
		t.Fatalf("skip posts = %d, want 1", n)
	}
	if st.OnboardingPending() {
		t.Fatal("onboarding still pending after a skip")
	}
	if !fake.Snapshot().OnboardingSkipped {
		t.Fatal("the server did not record the skip")
	}
}

func TestSkipOnboardingFailureKeepsPending(t *testing.T) {
	st, fake, fix := onboarding(t, freshServer)
	fake.FailNext(http.MethodPost, skipPath, http.StatusInternalServerError, map[string]any{"error": "disk full"})
	if err := call(t, fix, st.SkipOnboarding); err == nil {
		t.Fatal("SkipOnboarding succeeded against a failing server")
	}
	if !st.OnboardingPending() {
		t.Fatal("a failed skip cleared the pending onboarding")
	}
}

func TestFinishOnboardingStaysLocal(t *testing.T) {
	st, fake, fix := onboarding(t, freshServer)
	st.FinishOnboarding()
	fix.Settle(t)
	if st.OnboardingPending() {
		t.Fatal("onboarding still pending after finishing it")
	}
	if n := len(fake.Calls(http.MethodPost, skipPath)); n != 0 {
		t.Fatalf("finishing posted %d skips, want none", n)
	}
	if fake.Snapshot().OnboardingSkipped {
		t.Fatal("finishing locally recorded a skip on the server")
	}
}

func TestOnboardingRefetchedForTheNextSession(t *testing.T) {
	st, fake, fix := onboarding(t, freshServer)
	st.FinishOnboarding()
	st.Logout()
	fix.Settle(t)
	if st.OnboardingKnown() {
		t.Fatal("onboarding status survived signing out")
	}
	if err := call(t, fix, func(done func(error)) { st.Login("admin", "secret", done) }); err != nil {
		t.Fatalf("Login: %v", err)
	}
	st.Onboarding.Ensure()
	fix.Settle(t)
	if !st.OnboardingPending() {
		t.Fatal("the next session did not refetch the pending onboarding")
	}
	if n := len(fake.Calls(http.MethodGet, onboardingPath)); n != 2 {
		t.Fatalf("onboarding reads = %d, want 2", n)
	}
}
