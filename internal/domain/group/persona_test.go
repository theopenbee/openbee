package group

import (
	"strings"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/model"
)

func TestBuildPersona_IncludesNameDescriptionAndConstraints(t *testing.T) {
	g := model.Group{Name: "data-team", Description: "owns ETL", Constraints: "no drops"}
	members := []model.MemberBrief{
		{ID: "w1", Name: "alice", Description: "fetcher"},
		{ID: "w2", Name: "bob", Description: "transformer"},
	}
	out := BuildPersona(g, members)
	for _, want := range []string{"data-team", "owns ETL", "no drops", "alice", "fetcher", "bob", "transformer"} {
		if !strings.Contains(out, want) {
			t.Errorf("persona missing %q:\n%s", want, out)
		}
	}
}

func TestBuildPersona_EmptyMembers(t *testing.T) {
	g := model.Group{Name: "empty", Description: "lonely"}
	out := BuildPersona(g, nil)
	if !strings.Contains(out, "(no members)") {
		t.Errorf("expected '(no members)' marker:\n%s", out)
	}
}
