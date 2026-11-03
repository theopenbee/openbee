package model

import (
	"maps"
	"strings"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

type CreateForm struct {
	Name        string
	Engine      string
	Description string
	Constraints string
	WorkDir     string
	EngineArgs  map[string]string
	Scopes      []string
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
	return req
}

type EditForm struct {
	Name        string
	Description string
	WorkDir     string
	Engine      string
	EngineArgs  map[string]string
}

func EditUpdate(w api.Worker, f EditForm) (api.UpdateWorkerRequest, bool) {
	workDir := strings.TrimSpace(f.WorkDir)
	nameChanged := f.Name != w.Name
	workDirChanged := workDir != w.WorkDir
	engineChanged := f.Engine != w.Engine
	changed := nameChanged ||
		workDirChanged ||
		engineChanged ||
		f.Description != w.Description ||
		!maps.Equal(CleanEngineArgs(f.EngineArgs), w.EngineArgs)
	if !changed {
		return api.UpdateWorkerRequest{}, false
	}
	args := maps.Clone(f.EngineArgs)
	if args == nil {
		args = map[string]string{}
	}
	req := api.UpdateWorkerRequest{
		Description: &f.Description,
		EngineArgs:  &args,
	}
	if engineChanged {
		req.Engine = &f.Engine
	}
	if nameChanged {
		req.Name = &f.Name
	}
	if workDirChanged {
		req.WorkDir = &workDir
	}
	return req, true
}
