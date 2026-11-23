package worker_test

import (
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/views/route"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	claudeModelsPath = "/api/engines/claude/models"
	piModelsPath     = "/api/engines/pi/models"
)

func modelCrew(st *fakeapi.State) {
	crew(st)
	st.Models = map[string][]api.EngineModel{
		"claude": {{ID: "opus", Name: "Claude Opus"}, {ID: "sonnet"}, {ID: "haiku"}},
		"pi":     {{ID: "openai/gpt-5"}, {ID: "deepseek/deepseek-flash", Name: "DeepSeek Flash"}},
	}
	st.Workers[0].EngineModels = map[string]string{"claude": "opus"}
}

func (f *fixture) wantModels(body map[string]any, want map[string]string) {
	f.t.Helper()
	raw, _ := body["engine_models"].(map[string]any)
	got := map[string]string{}
	for engine, name := range raw {
		got[engine], _ = name.(string)
	}
	if !maps.Equal(got, want) || (want == nil) != (body["engine_models"] == nil) {
		f.t.Fatalf("engine_models = %v, want %v; body %v", body["engine_models"], want, body)
	}
}

func (f *fixture) top(text string) float32 {
	f.t.Helper()
	r, ok := f.T.Find(text)
	if !ok {
		f.t.Fatalf("screen does not show %q; texts %q", text, f.T.Texts())
	}
	return r.Y
}

func TestCreateShowsTheModelBelowTheEngine(t *testing.T) {
	f := newPanelFixture(t, modelCrew)
	f.wantShown(locales.WorkersFormModel(), locales.WorkersFormModelHelper(claudeName))
	engine, model, description := f.top(locales.WorkersFormEngineHelper()), f.top(locales.WorkersFormModelHelper(claudeName)),
		f.top(locales.WorkersFormDescriptionOptional())
	if engine >= model || model >= description {
		t.Fatalf("engine helper y=%v, model helper y=%v, description y=%v; want the model between them", engine, model, description)
	}
	f.wantHidden("sonnet")
}

func TestCreateSendsATypedModel(t *testing.T) {
	f := newPanelFixture(t, modelCrew)
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.replaceText(locales.WorkersFormModel(), "  my-own-model ")
	f.wantShown(locales.WorkersFormModelNoMatches())
	f.click(locales.WorkersCreateWorker())
	body := f.lastPost()
	f.wantKeys(body, "description", "engine", "engine_models", "name")
	f.wantModels(body, map[string]string{"claude": "my-own-model"})
	if got := f.Fake.Snapshot().Workers[2].EngineModels; got["claude"] != "my-own-model" {
		t.Fatalf("server models = %v, want the typed model", got)
	}
}

func TestCreateOmitsABlankModel(t *testing.T) {
	f := newPanelFixture(t, modelCrew)
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.replaceText(locales.WorkersFormModel(), "   ")
	f.click(locales.WorkersCreateWorker())
	f.wantKeys(f.lastPost(), "description", "engine", "name")
}

func TestCreatePicksASuggestedModel(t *testing.T) {
	f := newPanelFixture(t, modelCrew)
	f.click(locales.WorkersFormModel())
	f.wantShown("opus", "Claude Opus", "sonnet", "haiku")
	f.wantHidden("openai/gpt-5")
	f.click("sonnet")
	f.wantHidden("haiku")
	if got := f.fieldText(locales.WorkersFormModel()); got != "sonnet" {
		t.Fatalf("model field = %q, want the picked model", got)
	}
	f.replaceText(locales.WorkersFormModel(), "hai")
	f.wantShown("haiku")
	f.wantHidden("sonnet", "opus")
	f.click("haiku")
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.click(locales.WorkersCreateWorker())
	f.wantModels(f.lastPost(), map[string]string{"claude": "haiku"})
	calls := f.Fake.Calls(http.MethodGet, claudeModelsPath)
	if len(calls) != 1 || calls[0].Query != "" {
		t.Fatalf("model list calls = %+v, want one without a worker", calls)
	}
}

func TestCreateModelFollowsTheEngine(t *testing.T) {
	f := newPanelFixture(t, withSystemEngine("claude"))
	f.Fake.Update(func(st *fakeapi.State) { st.Models = map[string][]api.EngineModel{"pi": {{ID: "openai/gpt-5"}}} })
	f.replaceText(locales.WorkersFormModel(), "opus")
	f.click(locales.WorkersFormEngine())
	f.click(piName)
	f.wantShown(locales.WorkersFormModelHelper(piName))
	if got := f.fieldText(locales.WorkersFormModel()); got != "" {
		t.Fatalf("model field = %q after switching engine, want pi's empty model", got)
	}
	f.wantShown("openai/gpt-5")
	f.click("openai/gpt-5")
	f.click(locales.WorkersFormEngine())
	f.click(claudeName)
	if got := f.fieldText(locales.WorkersFormModel()); got != "opus" {
		t.Fatalf("model field = %q after switching back, want claude's model", got)
	}
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.click(locales.WorkersCreateWorker())
	body := f.lastPost()
	f.wantModels(body, map[string]string{"claude": "opus"})
	if body["engine"] != "claude" {
		t.Fatalf("POST body = %v, want claude", body)
	}
	if n := len(f.Fake.Calls(http.MethodGet, claudeModelsPath)); n != 1 {
		t.Fatalf("claude model list calls = %d, want the list cached", n)
	}
}

func TestModelListShowsLoading(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel(api.BeeConversation, ""), modelCrew)
	f.Store.SystemConfigs.Ensure()
	f.settle()
	release := f.Fake.HoldNext(http.MethodGet, claudeModelsPath)
	f.router.Push(route.ChatPanel(api.BeeConversation, route.PanelNew))
	f.T.Advance(motionSettled)
	f.T.Frame()
	if err := f.T.Click(locales.WorkersFormModel()); err != nil {
		t.Fatal(err)
	}
	f.T.Advance(motionSettled)
	f.T.Frame()
	f.wantShown(locales.WorkersFormModelLoading())
	f.wantHidden("sonnet")
	release()
	f.settle()
	f.wantShown("sonnet")
	f.wantHidden(locales.WorkersFormModelLoading())
}

func TestModelListErrorStillTakesTypedText(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel(api.BeeConversation, ""), modelCrew)
	f.Fake.FailNext(http.MethodGet, claudeModelsPath, http.StatusBadGateway, map[string]string{"error": "unknown method \"models\""})
	f.openNew()
	f.replaceText(locales.WorkersFormModel(), "opus-next")
	f.wantShown(locales.WorkersFormModelListError())
	f.wantHidden(locales.WorkersFormModelNoMatches(), "sonnet")
	f.replaceText(locales.WorkersFormName(), "Nova")
	f.click(locales.WorkersCreateWorker())
	f.wantModels(f.lastPost(), map[string]string{"claude": "opus-next"})
}

func TestModelListRetriesWhenTheFormReopens(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel(api.BeeConversation, ""), modelCrew)
	f.Fake.FailNext(http.MethodGet, claudeModelsPath, http.StatusBadGateway, map[string]string{"error": "timed out"})
	f.openNew()
	f.click(locales.WorkersFormModel())
	f.wantShown(locales.WorkersFormModelListError())
	f.click(locales.CommonCancel())
	f.openNew()
	f.click(locales.WorkersFormModel())
	f.wantShown("sonnet")
	f.wantHidden(locales.WorkersFormModelListError())
	if n := len(f.Fake.Calls(http.MethodGet, claudeModelsPath)); n != 2 {
		t.Fatalf("model list calls = %d, want one retry", n)
	}
}

func TestEditShowsAndChangesTheModel(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), modelCrew)
	f.click(locales.WorkerDetailEditInfo())
	f.wantShown(locales.WorkersFormModelHelper(claudeName))
	model, args := f.top(locales.WorkersFormModelHelper(claudeName)), f.top(locales.WorkersFormEngineArgsHelper(claudeName))
	if model >= args || f.top(locales.WorkersFormEngineHelper()) >= model {
		t.Fatal("the model field is not between the engine and the engine args")
	}
	if got := f.fieldText(locales.WorkersFormModel()); got != "opus" {
		t.Fatalf("model field = %q, want the stored model", got)
	}
	f.replaceText(locales.WorkersFormModel(), "son")
	f.click("sonnet")
	f.click(locales.WorkerDetailSaveInfo())
	body := f.lastPut(alicePath)
	if keys := slices.Sorted(maps.Keys(body)); !slices.Equal(keys, []string{"description", "engine_args", "engine_models"}) {
		t.Fatalf("PUT keys = %q, want description, engine_args, engine_models; body %v", keys, body)
	}
	f.wantModels(body, map[string]string{"claude": "sonnet"})
	f.wantHidden(locales.WorkerDetailEditInfoTitle())
	f.wantShown(claudeName + " · sonnet")
	calls := f.Fake.Calls(http.MethodGet, claudeModelsPath)
	if len(calls) != 1 || calls[0].Query != "worker_id=w1" {
		t.Fatalf("model list calls = %+v, want one for w1", calls)
	}
}

func TestEditLeavesAnUnchangedModelOut(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), modelCrew)
	f.click(locales.WorkerDetailEditInfo())
	f.replaceText(locales.WorkersFormDescription(), "Ships every release")
	f.click(locales.WorkerDetailSaveInfo())
	body := f.lastPut(alicePath)
	f.wantModels(body, nil)
	if got := f.Fake.Snapshot().Workers[0].EngineModels; got["claude"] != "opus" {
		t.Fatalf("server models = %v, want opus kept", got)
	}
}

func TestEditClearsTheModel(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), modelCrew)
	f.click(locales.WorkerDetailEditInfo())
	f.replaceText(locales.WorkersFormModel(), "")
	f.click(locales.WorkerDetailSaveInfo())
	f.wantModels(f.lastPut(alicePath), map[string]string{"claude": ""})
	if got := f.Fake.Snapshot().Workers[0].EngineModels; len(got) != 0 {
		t.Fatalf("server models = %v, want none", got)
	}
	f.wantShown(claudeName)
	f.wantHidden(claudeName + " · opus")
}

func TestEditKeepsModelsChangedWhileTheFormIsOpen(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), modelCrew)
	f.click(locales.WorkerDetailEditInfo())
	f.Fake.Update(func(st *fakeapi.State) {
		st.Workers[0].EngineModels = map[string]string{"claude": "opus", "pi": "openai/gpt-5"}
	})
	f.Store.Workers.Invalidate()
	f.settle()
	if got := f.Store.Workers.Data[0].EngineModels["pi"]; got != "openai/gpt-5" {
		t.Fatalf("refetched pi model = %q, want the model set elsewhere", got)
	}
	f.replaceText(locales.WorkersFormModel(), "son")
	f.click("sonnet")
	f.click(locales.WorkerDetailSaveInfo())
	f.wantModels(f.lastPut(alicePath), map[string]string{"claude": "sonnet"})
	if got := f.Fake.Snapshot().Workers[0].EngineModels; !maps.Equal(got, map[string]string{"claude": "sonnet", "pi": "openai/gpt-5"}) {
		t.Fatalf("server models = %v, want pi's model kept", got)
	}
}

func TestEditModelEnterKeepsTheTypedText(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), modelCrew)
	f.click(locales.WorkerDetailEditInfo())
	f.replaceText(locales.WorkersFormModel(), "son")
	f.wantShown("sonnet")
	f.T.Key(0, ui.KeyEnter)
	f.settle()
	f.wantModels(f.lastPut(alicePath), map[string]string{"claude": "son"})
	f.wantHidden(locales.WorkerDetailEditInfoTitle())
}

func TestEditModelFollowsTheEngineSelect(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), func(st *fakeapi.State) {
		modelCrew(st)
		st.Workers[0].EngineModels = map[string]string{"claude": "opus", "pi": "openai/gpt-5"}
	})
	f.click(locales.WorkerDetailEditInfo())
	f.click(locales.WorkersFormEngine())
	f.click(piName)
	f.wantShown(locales.WorkersFormModelHelper(piName))
	if got := f.fieldText(locales.WorkersFormModel()); got != "openai/gpt-5" {
		t.Fatalf("model field = %q after switching to pi, want pi's stored model", got)
	}
	f.click(locales.WorkerDetailSaveInfo())
	body := f.lastPut(alicePath)
	f.wantModels(body, nil)
	if body["engine"] != "pi" {
		t.Fatalf("PUT body = %v, want the pi engine", body)
	}
	f.wantShown(piName + " · openai/gpt-5")
}

func TestEditModelOfTheSystemDefaultEngine(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), func(st *fakeapi.State) {
		modelCrew(st)
		st.Workers[0].Engine = ""
		st.Workers[0].EngineModels = map[string]string{"pi": "openai/gpt-5"}
		st.SystemConfigs = map[string]string{"default_engine": "pi"}
	})
	f.wantShown(piName + " · openai/gpt-5")
	f.click(locales.WorkerDetailEditInfo())
	f.wantShown(locales.WorkersFormEngineSystemDefaultNamed(piName), locales.WorkersFormModelHelper(piName))
	if got := f.fieldText(locales.WorkersFormModel()); got != "openai/gpt-5" {
		t.Fatalf("model field = %q, want the default engine's model", got)
	}
	f.replaceText(locales.WorkersFormModel(), "deepseek")
	f.wantShown("DeepSeek Flash")
	f.click("deepseek/deepseek-flash")
	f.click(locales.WorkerDetailSaveInfo())
	body := f.lastPut(alicePath)
	f.wantModels(body, map[string]string{"pi": "deepseek/deepseek-flash"})
	if _, ok := body["engine"]; ok {
		t.Fatalf("PUT body = %v, want the empty engine left out", body)
	}
	if n := len(f.Fake.Calls(http.MethodGet, piModelsPath)); n != 1 {
		t.Fatalf("pi model list calls = %d, want 1", n)
	}
}

func TestEditModelOfAnUninstalledEngineIsPlainText(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w1", route.PanelConstraints), func(st *fakeapi.State) {
		engineCrew("gone", "pi")(st)
		st.Workers[0].EngineModels = map[string]string{"gone": "old-model"}
	})
	f.wantShown("gone · old-model")
	f.click(locales.WorkerDetailEditInfo())
	f.wantShown(locales.WorkersFormModelHelper("gone"))
	if got := f.fieldText(locales.WorkersFormModel()); got != "old-model" {
		t.Fatalf("model field = %q, want the stored model", got)
	}
	f.wantHidden(locales.WorkersFormModelLoading(), locales.WorkersFormModelListError())
	if n := len(f.Fake.Calls(http.MethodGet, "/api/engines/")); n != 0 {
		t.Fatalf("model list calls = %d for an uninstalled engine, want none", n)
	}
}

func TestProfileHidesTheModelOfAnotherEngine(t *testing.T) {
	f := newFixture(t, wideW, route.ChatPanel("w2", route.PanelConstraints), func(st *fakeapi.State) {
		modelCrew(st)
		st.Workers[1].EngineModels = map[string]string{"claude": "opus"}
	})
	f.wantShown(codexName)
	f.wantHidden(codexName+" · opus", "opus")
}
