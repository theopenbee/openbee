package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/AlecAivazis/survey/v2"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

// configureCodexExecutable handles Codex engine setup:
// 1. Auto-detect codex in PATH
// 2. If not found: manual path input
// 3. Prompt for timeout
func configureCodexExecutable(vals *configValues) error {
	if codexPath, err := exec.LookPath("codex"); err == nil {
		fmt.Printf(i18n.M.Output.Config.CodexFound+"\n", codexPath)
		vals.CodexPath = codexPath
	} else {
		fmt.Println(i18n.M.Output.Config.CodexManualEntry)
		if err := promptCodexManualPath(vals); err != nil {
			return err
		}
	}

	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.CodexTimeout,
		Default: vals.CodexTimeout,
	}, &vals.CodexTimeout); err != nil {
		return handleSurveyErr(err)
	}

	return nil
}

func promptCodexManualPath(vals *configValues) error {
	if err := survey.AskOne(&survey.Input{
		Message: i18n.M.Prompt.CodexPath,
		Default: vals.CodexPath,
	}, &vals.CodexPath, survey.WithValidator(func(val any) error {
		path, _ := val.(string)
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf(i18n.M.Validate.FileNotFound, path)
		}
		if info.IsDir() {
			return fmt.Errorf(i18n.M.Validate.PathIsDir, path)
		}
		if info.Mode()&0111 == 0 {
			return fmt.Errorf(i18n.M.Validate.FileNotExec, path)
		}
		return nil
	})); err != nil {
		return handleSurveyErr(err)
	}
	return nil
}
