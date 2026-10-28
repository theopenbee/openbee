package configcmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/AlecAivazis/survey/v2"
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/skillinstall"
)

type engineMapping struct{ name, label string }

var engineMappings = []engineMapping{
	{ai.EngineClaude, "Claude Code"},
	{ai.EngineCodex, "Codex"},
	{ai.EnginePi, "Pi"},
}

func engineLabel(name string) string {
	for _, m := range engineMappings {
		if m.name == name {
			return m.label
		}
	}
	return ""
}

func engineName(label string) string {
	for _, m := range engineMappings {
		if m.label == label {
			return m.name
		}
	}
	return ""
}

func configureEngineExecutable(binaryName, foundMsg, manualMsg, pathMsg string, pathDst *string) error {
	if found, err := exec.LookPath(binaryName); err == nil {
		fmt.Printf(foundMsg+"\n", found)
		*pathDst = found
	} else {
		fmt.Println(manualMsg)
		if err := survey.AskOne(&survey.Input{
			Message: pathMsg,
			Default: *pathDst,
		}, pathDst, survey.WithValidator(executablePathValidator)); err != nil {
			return handleSurveyErr(err)
		}
	}
	return nil
}

func executablePathValidator(val any) error {
	path, _ := val.(string)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("file not found: %s", path)
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", path)
	}
	if info.Mode()&0111 == 0 {
		return fmt.Errorf("file is not executable: %s", path)
	}
	return nil
}

func installBuiltinSkills() {
	results, err := skillinstall.InstallSkillsToDefaults()
	if err != nil {
		fmt.Printf("skills install warning (skipped): %v\n", err)
		return
	}
	for _, r := range results {
		switch r.Action {
		case skillinstall.ActionInstalled:
			fmt.Printf("skill %s installed\n", r.Name)
		case skillinstall.ActionUpdated:
			fmt.Printf("skill %s updated\n", r.Name)
		case skillinstall.ActionUpToDate:
			fmt.Printf("skill %s up to date (skipped)\n", r.Name)
		}
	}
}
