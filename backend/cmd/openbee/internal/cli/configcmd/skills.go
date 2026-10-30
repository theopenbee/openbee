package configcmd

import (
	"fmt"

	"github.com/theopenbee/openbee/internal/infra/skillinstall"
)

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
