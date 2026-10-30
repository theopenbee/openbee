package store_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
)

const (
	systemPath    = "/api/system-configs"
	engineKey     = "default_engine"
	beeArgsKey    = "engine_args_bee"
	globalArgsKey = "engine_args_global"
)

func withSystemConfigs(st *fakeapi.State) {
	team(st)
	st.SystemConfigs = map[string]string{engineKey: "claude", globalArgsKey: `{"claude":"--model opus"}`}
}

func TestSaveSystemConfigsWritesEachKeyAndRefetches(t *testing.T) {
	st, fake, fix := loaded(t, withSystemConfigs)
	st.SystemConfigs.Ensure()
	fix.Settle(t)
	before := len(fake.Calls(http.MethodGet, systemPath))

	var seen string
	st.SaveSystemConfigs(map[string]string{engineKey: "pi", beeArgsKey: `{"pi":"--x"}`}, func(err error) {
		if err != nil {
			t.Errorf("SaveSystemConfigs: %v", err)
		}
		seen = st.SystemConfigs.Data[engineKey] + "|" + st.SystemConfigs.Data[beeArgsKey]
	})
	fix.Settle(t)

	var body map[string]string
	for key, want := range map[string]string{engineKey: "pi", beeArgsKey: `{"pi":"--x"}`} {
		calls := fake.Calls(http.MethodPut, systemPath+"/"+key)
		if len(calls) != 1 {
			t.Fatalf("PUT %s calls = %d, want 1", key, len(calls))
		}
		lastBody(t, fake, http.MethodPut, systemPath+"/"+key, &body)
		if body["value"] != want {
			t.Fatalf("PUT %s value = %q, want %q", key, body["value"], want)
		}
	}
	if n := len(fake.Calls(http.MethodPut, systemPath+"/"+globalArgsKey)); n != 0 {
		t.Fatalf("unchanged key was written %d times", n)
	}
	if n := len(fake.Calls(http.MethodGet, systemPath)); n != before+1 {
		t.Fatalf("system config reads = %d, want %d (one refetch)", n, before+1)
	}
	if seen != `pi|{"pi":"--x"}` {
		t.Fatalf("data when done ran = %q, want the refetched values", seen)
	}
	if got := st.SystemConfigs.Data[globalArgsKey]; got != `{"claude":"--model opus"}` {
		t.Fatalf("untouched key = %q, want it kept", got)
	}
}

func TestSaveSystemConfigsReportsFirstErrorAndStillRefetches(t *testing.T) {
	st, fake, fix := loaded(t, withSystemConfigs)
	st.SystemConfigs.Ensure()
	fix.Settle(t)
	before := len(fake.Calls(http.MethodGet, systemPath))
	fake.FailNext(http.MethodPut, systemPath+"/"+beeArgsKey, http.StatusInternalServerError, map[string]string{"error": "bee boom"})
	fake.FailNext(http.MethodPut, systemPath+"/"+engineKey, http.StatusBadRequest, map[string]string{"error": "engine boom"})

	err := call(t, fix, func(done func(error)) {
		st.SaveSystemConfigs(map[string]string{engineKey: "pi", beeArgsKey: `{"pi":"--x"}`, globalArgsKey: ""}, done)
	})
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
		t.Fatalf("error = %v, want the 400 of the first key in name order", err)
	}
	if n := len(fake.Calls(http.MethodPut, systemPath+"/"+globalArgsKey)); n != 1 {
		t.Fatalf("healthy key writes = %d, want it written despite the failures", n)
	}
	if n := len(fake.Calls(http.MethodGet, systemPath)); n != before+1 {
		t.Fatalf("system config reads = %d, want %d", n, before+1)
	}
	if got := st.SystemConfigs.Data[globalArgsKey]; got != "" {
		t.Fatalf("healthy write not visible after refetch: %q", got)
	}
}

func TestSaveSystemConfigsWithoutChangesSendsNothing(t *testing.T) {
	st, fake, fix := loaded(t, withSystemConfigs)
	if err := call(t, fix, func(done func(error)) { st.SaveSystemConfigs(nil, done) }); err != nil {
		t.Fatalf("SaveSystemConfigs: %v", err)
	}
	if n := len(fake.Calls(http.MethodPut, systemPath)); n != 0 {
		t.Fatalf("PUT calls = %d, want none", n)
	}
}

func TestSaveSystemConfigsCanceledBySignOut(t *testing.T) {
	st, _, fix := loaded(t, withSystemConfigs)
	var got error
	calls := 0
	st.SaveSystemConfigs(map[string]string{engineKey: "pi"}, func(err error) {
		got = err
		calls++
	})
	st.Logout()
	fix.Settle(t)
	if calls != 1 || !errors.Is(got, context.Canceled) {
		t.Fatalf("done calls = %d err = %v, want one context.Canceled", calls, got)
	}
}
