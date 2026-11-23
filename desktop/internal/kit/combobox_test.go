package kit_test

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/theme"
)

var modelOptions = []kit.ComboOption{{Value: "opus", Detail: "Claude Opus"}, {Value: "sonnet"}, {Value: "haiku"}, {Value: "openai/gpt-5"}}

type comboHarness struct {
	value            string
	opts             kit.ComboOpts
	changes, submits int
}

func newCombo(opts kit.ComboOpts) (*comboHarness, *ui.Tester) {
	h := &comboHarness{opts: opts}
	tt := newTester(500, 500, func(c *ui.Context, tk theme.Tokens) {
		kit.Combobox(c, tk, &h.value, "Model", "Engine default", h.opts).Width(320).
			OnChange(func() { h.changes++ }).OnSubmit(func() { h.submits++ })
	})
	return h, tt
}

func typeInto(t *testing.T, tt *ui.Tester, text string) {
	t.Helper()
	if err := tt.Click("Model"); err != nil {
		t.Fatal(err)
	}
	tt.Command("selectAll")
	tt.Type(text)
	settle(tt)
}

func TestComboboxListsOptionsAndChoosesOne(t *testing.T) {
	h, tt := newCombo(kit.ComboOpts{Options: modelOptions})
	if tt.HasText("sonnet") {
		t.Fatal("the options show before the field opens")
	}
	if err := tt.Click("Model"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	for _, text := range []string{"opus", "Claude Opus", "sonnet", "haiku", "openai/gpt-5"} {
		if !tt.HasText(text) {
			t.Fatalf("the open field does not list %q", text)
		}
	}
	if err := tt.Click("sonnet"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if h.value != "sonnet" || h.changes != 1 {
		t.Fatalf("value = %q after %d changes, want sonnet after one", h.value, h.changes)
	}
	if tt.HasText("haiku") {
		t.Error("the options stay open after choosing")
	}
	tt.Type("-4")
	settle(tt)
	if h.value != "sonnet-4" {
		t.Fatalf("value = %q after typing on, want the caret after the chosen model", h.value)
	}
}

func TestComboboxFiltersAndEnterKeepsTheTypedText(t *testing.T) {
	h, tt := newCombo(kit.ComboOpts{Options: modelOptions})
	typeInto(t, tt, "claude")
	if !tt.HasText("opus") || tt.HasText("haiku") {
		t.Fatalf("filtered texts = %q, want the model whose name matches", tt.Texts())
	}
	typeInto(t, tt, "gpt")
	if !tt.HasText("openai/gpt-5") || tt.HasText("sonnet") {
		t.Fatalf("filtered texts = %q, want only the gpt model", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	settle(tt)
	if h.value != "gpt" || h.submits != 1 {
		t.Fatalf("value = %q, submits = %d after Enter, want the typed gpt submitted", h.value, h.submits)
	}
	if tt.HasText("openai/gpt-5") {
		t.Error("the options stay open after Enter")
	}
}

func TestComboboxEnterTakesAnExactMatch(t *testing.T) {
	h, tt := newCombo(kit.ComboOpts{Options: []kit.ComboOption{{Value: "sonnet-4"}, {Value: "Sonnet"}, {Value: "sonnet"}}})
	typeInto(t, tt, "sonnet ")
	tt.Key(0, ui.KeyEnter)
	settle(tt)
	if h.value != "sonnet" || h.submits != 0 {
		t.Fatalf("value = %q, submits = %d after Enter, want the exact option taken without submitting", h.value, h.submits)
	}
	if tt.HasText("sonnet-4") {
		t.Error("the options stay open after taking the exact match")
	}
}

func TestComboboxArrowsAndEnterTakeAnOption(t *testing.T) {
	h, tt := newCombo(kit.ComboOpts{Options: modelOptions})
	typeInto(t, tt, "o")
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyDown)
	settle(tt)
	tt.Key(0, ui.KeyEnter)
	settle(tt)
	if h.value != "openai/gpt-5" || h.submits != 0 {
		t.Fatalf("value = %q, submits = %d after Down, Down, Enter, want the second match taken without submitting", h.value, h.submits)
	}
	if tt.HasText("sonnet") {
		t.Error("the options stay open after choosing with the arrows")
	}
}

func TestComboboxKeepsFreeText(t *testing.T) {
	h, tt := newCombo(kit.ComboOpts{Options: modelOptions, NoMatches: "No matching models"})
	typeInto(t, tt, "deepseek/flash")
	if h.value != "deepseek/flash" {
		t.Fatalf("value = %q, want the typed text", h.value)
	}
	if !tt.HasText("No matching models") || tt.HasText("opus") {
		t.Fatalf("texts = %q, want the no-match note alone", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	settle(tt)
	if h.value != "deepseek/flash" || h.submits != 1 {
		t.Fatalf("value = %q, submits = %d after Enter, want the typed text submitted", h.value, h.submits)
	}
	typeInto(t, tt, "")
	if tt.HasText("No matching models") {
		t.Error("the no-match note shows for an empty field")
	}
}

func TestComboboxWithoutOptionsShowsNoNote(t *testing.T) {
	h, tt := newCombo(kit.ComboOpts{NoMatches: "No matching models"})
	typeInto(t, tt, "anything")
	if h.value != "anything" || tt.HasText("No matching models") {
		t.Fatalf("value = %q, texts = %q, want plain typing without a note", h.value, tt.Texts())
	}
}

func TestComboboxShowsLoadingAndFailure(t *testing.T) {
	h, tt := newCombo(kit.ComboOpts{Loading: "Loading models", NoMatches: "No matching models"})
	if err := tt.Click("Model"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if !tt.HasText("Loading models") {
		t.Fatalf("texts = %q, want the loading note", tt.Texts())
	}
	h.opts = kit.ComboOpts{Failed: "Could not load the models", NoMatches: "No matching models"}
	tt.Type("opus")
	settle(tt)
	if h.value != "opus" || !tt.HasText("Could not load the models") || tt.HasText("Loading models") {
		t.Fatalf("value = %q, texts = %q, want typing kept and the failure shown", h.value, tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	settle(tt)
	if tt.HasText("Could not load the models") {
		t.Error("Escape leaves the failure note open")
	}
}

type comboGallery struct {
	model, loading, failed string
}

func (g *comboGallery) view(c *ui.Context) {
	tk := theme.Apply(c)
	ui.Row(c).Fill().Padding(32).Gap(32).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Width(360).Children(func() {
			kit.Field(c, tk, "Model", "", "", func() {
				kit.Combobox(c, tk, &g.model, "Model", "Engine default", kit.ComboOpts{Options: modelOptions, Help: "Leave blank to use Claude Code's default model.", NoMatches: "No matching models."})
			})
		})
		ui.Column(c).Width(360).Children(func() {
			kit.Field(c, tk, "Loading model", "", "", func() {
				kit.Combobox(c, tk, &g.loading, "Loading model", "Engine default", kit.ComboOpts{Loading: "Loading models…"})
			})
		})
		ui.Column(c).Width(360).Children(func() {
			kit.Field(c, tk, "Failed model", "", "", func() {
				kit.Combobox(c, tk, &g.failed, "Failed model", "Engine default",
					kit.ComboOpts{Failed: "Couldn't load the model list. You can still type a model."})
			})
		})
	})
}

func comboShots() []overlayShot {
	open := func(label string) func(t *testing.T, tt *ui.Tester) {
		return func(t *testing.T, tt *ui.Tester) {
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
		}
	}
	return []overlayShot{
		{name: "kit-combobox", w: 1280, h: 520, view: (&comboGallery{model: "opus"}).view, texts: []string{"Model"}, prepare: open("Model")},
		{name: "kit-combobox-loading", w: 1280, h: 520, view: (&comboGallery{}).view, texts: []string{"Loading model"}, prepare: open("Loading model")},
		{name: "kit-combobox-failed", w: 1280, h: 520, view: (&comboGallery{}).view, texts: []string{"Failed model"}, prepare: open("Failed model")},
	}
}
