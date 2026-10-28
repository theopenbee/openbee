package worker

import (
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/theopenbee/openbee/desktop/internal/api"
	"github.com/theopenbee/openbee/desktop/internal/kit"
	"github.com/theopenbee/openbee/desktop/internal/model"
	"github.com/theopenbee/openbee/desktop/internal/theme"
	"github.com/theopenbee/openbee/desktop/locales"
)

const (
	permissionsGap = 16
	scopeRowMin    = 64
	scopeRowPadY   = 16
	scopeRowPadX   = 24
)

type scopeRow struct {
	id                 string
	title, description func() string
}

var scopeRows = []scopeRow{
	{id: "read:workers", title: locales.ScopesReadWorkersTitle, description: locales.ScopesReadWorkersDescription},
	{id: "read:tasks", title: locales.ScopesReadTasksTitle, description: locales.ScopesReadTasksDescription},
	{id: "read:messages", title: locales.ScopesReadMessagesTitle, description: locales.ScopesReadMessagesDescription},
}

type permissionsState struct {
	synced string
	scopes []string
	saving bool
	err    error
}

type permissionsKey struct{}

func (p *Panel) permissions(c *ui.Context, tk theme.Tokens, w api.Worker) {
	box := ui.Column(c).MinWidth(0).Gap(permissionsGap).Label(locales.WorkerDetailPermissions())
	ps := ui.Local(box, permissionsKey{}, func() permissionsState {
		return permissionsState{synced: w.PermissionScopes, scopes: model.ParseScopes(w.PermissionScopes)}
	})
	if ps.synced != w.PermissionScopes {
		ps.synced, ps.scopes = w.PermissionScopes, model.ParseScopes(w.PermissionScopes)
	}
	box.Children(func() {
		errorAlert(c, tk, locales.WorkerDetailPermissionsSaveError(), ps.err)
		card(c, tk).Children(func() {
			for i, scope := range scopeRows {
				on := slices.Contains(ps.scopes, scope.id)
				row := kit.SwitchRow(c, tk, &on, scope.title(), scope.description(), ps.saving).
					MinHeight(scopeRowMin).Padding(scopeRowPadY, scopeRowPadX)
				if i > 0 {
					row.BorderWidth(1, 0, 0, 0).BorderColor(tk.BorderSubtle)
				}
				row.OnChange(func() { p.toggleScope(ps, w.ID, scope.id, on) })
			}
		})
	})
}

func (p *Panel) toggleScope(ps *permissionsState, id, scope string, on bool) {
	prev := ps.scopes
	next := slices.DeleteFunc(slices.Clone(prev), func(s string) bool { return s == scope })
	if on {
		next = append(slices.Clone(prev), scope)
	}
	ps.scopes, ps.saving, ps.err = next, true, nil
	joined := model.JoinScopes(next)
	p.s.UpdateWorker(id, api.UpdateWorkerRequest{PermissionScopes: &joined}, func(_ api.Worker, err error) {
		ps.saving = false
		if err != nil {
			ps.scopes = prev
			ps.err = failure(err)
		}
	})
}
