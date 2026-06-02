package configcmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/AlecAivazis/survey/v2"
	ai "github.com/theopenbee/openbee/internal/ai"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/skillinstall"
)

type engineMapping struct{ name, label string }

func engineMappings() []engineMapping {
	return []engineMapping{
		{ai.EngineClaude, i18n.M.Prompt.OptionEngineClaude},
		{ai.EngineCodex, i18n.M.Prompt.OptionEngineCodex},
		{ai.EnginePi, i18n.M.Prompt.OptionEnginePi},
	}
}

func engineLabel(name string) string {
	for _, m := range engineMappings() {
		if m.name == name {
			return m.label
		}
	}
	return ""
}

func engineName(label string) string {
	for _, m := range engineMappings() {
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
		return fmt.Errorf(i18n.M.Validate.FileNotFound, path)
	}
	if info.IsDir() {
		return fmt.Errorf(i18n.M.Validate.PathIsDir, path)
	}
	if info.Mode()&0111 == 0 {
		return fmt.Errorf(i18n.M.Validate.FileNotExec, path)
	}
	return nil
}

func installBuiltinSkills() {
	results, err := skillinstall.InstallSkillsToDefaults()
	if err != nil {
		fmt.Printf(i18n.M.Output.Config.SkillsInstallWarning+"\n", err)
		return
	}
	for _, r := range results {
		switch r.Action {
		case skillinstall.ActionInstalled:
			fmt.Printf(i18n.M.Output.Config.SkillInstalled+"\n", r.Name)
		case skillinstall.ActionUpdated:
			fmt.Printf(i18n.M.Output.Config.SkillUpdated+"\n", r.Name)
		case skillinstall.ActionUpToDate:
			fmt.Printf(i18n.M.Output.Config.SkillUpToDate+"\n", r.Name)
		}
	}
}
