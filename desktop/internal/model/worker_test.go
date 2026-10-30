package model

import (
	"maps"
	"reflect"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func TestAdvancedCount(t *testing.T) {
	tests := []struct {
		name string
		form CreateForm
		want int
	}{
		{"zero when nothing is set", CreateForm{Engine: "claude"}, 0},
		{
			"ignores whitespace-only text",
			CreateForm{Engine: "claude", Constraints: "  ", WorkDir: "\n", EngineArgs: map[string]string{"claude": " "}},
			0,
		},
		{"counts each filled text setting", CreateForm{Engine: "claude", Constraints: "be brief", WorkDir: "/tmp/w"}, 2},
		{"counts engine args", CreateForm{Engine: "claude", EngineArgs: map[string]string{"claude": "--effort high"}}, 1},
		{
			"counts only the selected engine args",
			CreateForm{Engine: "pi", EngineArgs: map[string]string{"claude": "--effort high"}},
			0,
		},
		{
			"counts permissions once however many scopes are selected",
			CreateForm{Engine: "claude", Scopes: []string{"read:workers", "read:tasks"}},
			1,
		},
		{
			"counts every setting when all are filled",
			CreateForm{
				Engine:      "claude",
				Constraints: "c",
				WorkDir:     "/w",
				EngineArgs:  map[string]string{"claude": "--x"},
				Scopes:      []string{"read:tasks"},
			},
			4,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.form.AdvancedCount(); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCreateFormRequestMinimal(t *testing.T) {
	got := CreateForm{Name: "  sam  ", Engine: "pi", Description: "helper"}.Request()
	want := api.CreateWorkerRequest{Name: "sam", Engine: "pi", Description: "helper"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestCreateFormRequestFull(t *testing.T) {
	got := CreateForm{
		Name:        "sam",
		Engine:      "codex",
		Description: "d",
		Constraints: "be brief",
		WorkDir:     "/work/sam",
		EngineArgs:  map[string]string{"claude": "", "codex": "--x", "pi": "  "},
		Scopes:      []string{"read:tasks", "read:workers"},
	}.Request()
	want := api.CreateWorkerRequest{
		Name:             "sam",
		Engine:           "codex",
		Description:      "d",
		Constraints:      "be brief",
		WorkDir:          "/work/sam",
		PermissionScopes: "read:tasks,read:workers",
		EngineArgs:       map[string]string{"codex": "--x"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestCreateFormRequestOmitsBlankEngineArgs(t *testing.T) {
	got := CreateForm{Name: "sam", Engine: "claude", EngineArgs: map[string]string{"claude": " ", "pi": ""}}.Request()
	if got.EngineArgs != nil {
		t.Fatalf("got %#v, want nil", got.EngineArgs)
	}
}

func existingWorker(mutate ...func(*api.Worker)) api.Worker {
	w := api.Worker{
		ID:          "w1",
		Name:        "sam",
		Description: "helper",
		WorkDir:     "/work/sam",
		Engine:      "claude",
		EngineArgs:  map[string]string{"claude": "--effort high"},
	}
	for _, f := range mutate {
		f(&w)
	}
	return w
}

func unchangedEdit(w api.Worker) EditForm {
	return EditForm{
		Name:        w.Name,
		Description: w.Description,
		WorkDir:     w.WorkDir,
		Engine:      w.Engine,
		EngineArgs:  maps.Clone(w.EngineArgs),
	}
}

func TestEditUpdateUnchanged(t *testing.T) {
	w := existingWorker()
	got, ok := EditUpdate(w, unchangedEdit(w))
	if ok || !reflect.DeepEqual(got, api.UpdateWorkerRequest{}) {
		t.Fatalf("got (%#v, %v), want an empty request and false", got, ok)
	}
}

func TestEditUpdateUnchangedIgnoresBlankArgsAndWorkDirPadding(t *testing.T) {
	w := existingWorker()
	f := unchangedEdit(w)
	f.WorkDir = "  /work/sam \n"
	f.EngineArgs["pi"] = "  "
	if _, ok := EditUpdate(w, f); ok {
		t.Fatal("blank engine args and work dir padding must not count as changes")
	}
}

func TestEditUpdateOnlyChanged(t *testing.T) {
	w := existingWorker()
	f := unchangedEdit(w)
	f.Description = "new helper"
	f.WorkDir = "  /work/other  "
	got, ok := EditUpdate(w, f)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if got.Name != nil {
		t.Fatalf("Name = %q, want nil for an unchanged name", *got.Name)
	}
	if got.WorkDir == nil || *got.WorkDir != "/work/other" {
		t.Fatalf("WorkDir = %v, want the trimmed path", got.WorkDir)
	}
	if got.Description == nil || *got.Description != "new helper" {
		t.Fatalf("Description = %v, want %q", got.Description, "new helper")
	}
	if got.Engine != nil {
		t.Fatalf("Engine = %q, want nil for an unchanged engine", *got.Engine)
	}
	if got.EngineArgs == nil || !maps.Equal(*got.EngineArgs, w.EngineArgs) {
		t.Fatalf("EngineArgs = %v, want the full edited map", got.EngineArgs)
	}
	if got.Constraints != nil || got.PermissionScopes != nil {
		t.Fatalf("Constraints and PermissionScopes must stay nil, got %#v", got)
	}
}

func TestEditUpdateNameChange(t *testing.T) {
	w := existingWorker()
	f := unchangedEdit(w)
	f.Name = "sam2"
	got, ok := EditUpdate(w, f)
	if !ok || got.Name == nil || *got.Name != "sam2" {
		t.Fatalf("got (%#v, %v), want Name sam2", got, ok)
	}
	if got.WorkDir != nil {
		t.Fatalf("WorkDir = %q, want nil for an unchanged work dir", *got.WorkDir)
	}
}

func TestEditUpdateEngineChange(t *testing.T) {
	w := existingWorker()
	f := unchangedEdit(w)
	f.Engine = "pi"
	got, ok := EditUpdate(w, f)
	if !ok || got.Engine == nil || *got.Engine != "pi" {
		t.Fatalf("got (%#v, %v), want Engine pi", got, ok)
	}
}

func TestEditUpdateKeepsAnEmptyEngine(t *testing.T) {
	w := existingWorker(func(w *api.Worker) { w.Engine = "" })
	f := unchangedEdit(w)
	f.Description = "new helper"
	got, ok := EditUpdate(w, f)
	if !ok || got.Engine != nil {
		t.Fatalf("got (%#v, %v), want the follow-the-system-default engine left out", got, ok)
	}
}

func TestEditUpdateKeepsAnUninstalledEngine(t *testing.T) {
	w := existingWorker(func(w *api.Worker) { w.Engine = "gone" })
	f := unchangedEdit(w)
	f.Name = "sam2"
	got, ok := EditUpdate(w, f)
	if !ok || got.Engine != nil {
		t.Fatalf("got (%#v, %v), want the stored engine left out", got, ok)
	}
}

func TestEditUpdateEngineArgsSendsTheFullEditedMap(t *testing.T) {
	w := existingWorker()
	f := unchangedEdit(w)
	f.EngineArgs = map[string]string{"claude": "", "pi": "--x"}
	got, ok := EditUpdate(w, f)
	if !ok || got.EngineArgs == nil {
		t.Fatalf("got (%#v, %v), want engine args", got, ok)
	}
	if want := (map[string]string{"claude": "", "pi": "--x"}); !maps.Equal(*got.EngineArgs, want) {
		t.Fatalf("EngineArgs = %v, want %v", *got.EngineArgs, want)
	}
}

func TestEditUpdateClearedEngineArgsSendsAnEmptyObject(t *testing.T) {
	w := existingWorker()
	f := unchangedEdit(w)
	f.EngineArgs = nil
	got, ok := EditUpdate(w, f)
	if !ok || got.EngineArgs == nil || *got.EngineArgs == nil || len(*got.EngineArgs) != 0 {
		t.Fatalf("got (%#v, %v), want an empty non-nil map", got, ok)
	}
}

func TestEditUpdateWorkerWithoutArgs(t *testing.T) {
	w := existingWorker(func(w *api.Worker) { w.EngineArgs = nil })
	f := unchangedEdit(w)
	f.EngineArgs = map[string]string{"claude": "  "}
	if _, ok := EditUpdate(w, f); ok {
		t.Fatal("a blank-only args map equals a worker without args")
	}
}

func TestEditUpdateDoesNotAliasTheForm(t *testing.T) {
	w := existingWorker()
	f := unchangedEdit(w)
	f.EngineArgs["pi"] = "--x"
	got, _ := EditUpdate(w, f)
	f.EngineArgs["pi"] = "changed"
	if (*got.EngineArgs)["pi"] != "--x" {
		t.Fatalf("request aliases the form map: %v", *got.EngineArgs)
	}
}
