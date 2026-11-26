package model

import (
	"maps"
	"strings"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

type CreateForm struct {
	Name         string
	Engine       string
	Description  string
	Constraints  string
	WorkDir      string
	EngineArgs   map[string]string
	EngineModels map[string]string
	Scopes       []string
}

func (f CreateForm) AdvancedCount() int {
	count := 0
	for _, text := range []string{f.Constraints, f.WorkDir, f.EngineArgs[f.Engine]} {
		if strings.TrimSpace(text) != "" {
			count++
		}
	}
	if len(f.Scopes) > 0 {
		count++
	}
	return count
}

func (f CreateForm) Request() api.CreateWorkerRequest {
	req := api.CreateWorkerRequest{
		Name:             strings.TrimSpace(f.Name),
		Engine:           f.Engine,
		Description:      f.Description,
		Constraints:      f.Constraints,
		WorkDir:          f.WorkDir,
		PermissionScopes: JoinScopes(f.Scopes),
	}
	if args := CleanEngineArgs(f.EngineArgs); len(args) > 0 {
		req.EngineArgs = args
	}
	if name := strings.TrimSpace(f.EngineModels[f.Engine]); f.Engine != "" && name != "" {
		req.EngineModels = map[string]string{f.Engine: name}
	}
	return req
}

type EditForm struct {
	Name                string
	Description         string
	WorkDir             string
	Engine              string
	EngineArgs          map[string]string
	EngineModels        map[string]string
	InitialEngineModels map[string]string
	Avatar              string
}

func NewEditForm(w api.Worker) EditForm {
	return EditForm{Name: w.Name, Description: w.Description, WorkDir: w.WorkDir, Engine: w.Engine,
		EngineArgs: cloneMap(w.EngineArgs), EngineModels: cloneMap(w.EngineModels), InitialEngineModels: cloneMap(w.EngineModels),
		Avatar: w.Avatar}
}

func cloneMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return maps.Clone(m)
}

func EditUpdate(w api.Worker, f EditForm) (api.UpdateWorkerRequest, bool) {
	workDir := strings.TrimSpace(f.WorkDir)
	nameChanged := f.Name != w.Name
	workDirChanged := workDir != w.WorkDir
	engineChanged := f.Engine != w.Engine
	avatarChanged := f.Avatar != w.Avatar
	models := ChangedEngineModels(f.InitialEngineModels, f.EngineModels)
	changed := nameChanged ||
		workDirChanged ||
		engineChanged ||
		avatarChanged ||
		len(models) > 0 ||
		f.Description != w.Description ||
		!maps.Equal(CleanEngineArgs(f.EngineArgs), w.EngineArgs)
	if !changed {
		return api.UpdateWorkerRequest{}, false
	}
	args := cloneMap(f.EngineArgs)
	req := api.UpdateWorkerRequest{
		Description: &f.Description,
		EngineArgs:  &args,
	}
	if engineChanged {
		req.Engine = &f.Engine
	}
	if len(models) > 0 {
		req.EngineModels = &models
	}
	if nameChanged {
		req.Name = &f.Name
	}
	if workDirChanged {
		req.WorkDir = &workDir
	}
	if avatarChanged {
		req.Avatar = &f.Avatar
	}
	return req, true
}
