package settings_test

import (
	"net/http"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"github.com/theopenbee/openbee/desktop/internal/views/settings"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	engineModelsAll = "engine_models_global"
	engineModelsBee = "engine_models_bee"
)

func globalModelInput(engine string) string {
	return locales.SystemSettingsModelsSectionTitle() + ": " + locales.SystemSettingsGlobalModelsSectionTitle() + ": " + engine
}

func beeModelInput(engine string) string {
	return locales.SystemSettingsModelsSectionTitle() + ": " + locales.SystemSettingsBeeModelsSectionTitle() + ": " + engine
}

func withModels(st *fakeapi.State) {
	configured(st)
	st.SystemConfigs[engineModelsAll] = `{"pi":"deepseek/deepseek-flash"}`
	st.SystemConfigs[engineModelsBee] = `{"claude":"opus"}`
	st.Models = map[string][]api.EngineModel{"pi": {{ID: "deepseek/deepseek-flash"}, {ID: "deepseek/deepseek-v4-pro"}}}
}

func TestSystemShowsSavedModels(t *testing.T) {
	f := open(t, wideW, settings.SectionSystem, withModels)
	f.wantShown(locales.SystemSettingsModelsSectionTitle(), locales.SystemSettingsModelsSectionDescription(),
		locales.SystemSettingsGlobalModelsSectionDescription(), locales.SystemSettingsBeeModelsSectionDescription(),
		globalModelInput(claudeName), globalModelInput(piName), beeModelInput(codexName))
	if got := f.fieldText(beeModelInput(claudeName)); got != "opus" {
		t.Fatalf("bee claude model = %q, want the saved model", got)
	}
	if got := f.fieldText(globalModelInput(piName)); got != "deepseek/deepseek-flash" {
		t.Fatalf("global pi model = %q, want the saved model", got)
	}
}

func TestSystemLoadsModelListsOnlyWhenOpened(t *testing.T) {
	f := open(t, wideW, settings.SectionSystem, withModels)
	if n := len(f.Fake.Calls(http.MethodGet, "/api/engines/")); n != 0 {
		t.Fatalf("opening settings listed models %d times", n)
	}
	f.typeInto(globalModelInput(piName), "deepseek")
	if n := f.reads("/api/engines/pi/models"); n != 1 {
		t.Fatalf("pi models read %d times after focusing its field, want 1", n)
	}
	f.wantShown("deepseek/deepseek-v4-pro")
	if n := len(f.Fake.Calls(http.MethodGet, "/api/engines/claude/")); n != 0 {
		t.Fatalf("claude models were listed %d times without opening its field", n)
	}
}

func TestSystemSavesChangedModels(t *testing.T) {
	f := open(t, wideW, settings.SectionSystem, withModels)
	f.typeInto(globalModelInput(claudeName), " sonnet ")
	f.typeInto(beeModelInput(claudeName), "")
	f.clickIn(locales.SystemSettingsSave())
	got := f.puts()
	want := map[string]string{
		systemPath + "/" + engineModelsAll: `{"claude":"sonnet","pi":"deepseek/deepseek-flash"}`,
		systemPath + "/" + engineModelsBee: `{}`,
	}
	if len(got) != len(want) {
		t.Fatalf("PUTs = %v, want %v", got, want)
	}
	for path, value := range want {
		if got[path] != value {
			t.Fatalf("PUT %s = %q, want %q (all: %v)", path, got[path], value, got)
		}
	}
	f.wantShown(locales.SystemSettingsSaved())
}

func TestSystemSingleEngineModelHelp(t *testing.T) {
	f := open(t, wideW, settings.SectionSystem, func(st *fakeapi.State) {
		withModels(st)
		st.Engines = fakeapi.Engines("pi")
	})
	f.wantShown(globalModelInput(locales.WorkersFormModel()), beeModelInput(locales.WorkersFormModel()),
		locales.WorkersFormModelHelper(piName), locales.WorkersFormModelHelperDefault())
}
