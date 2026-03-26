// cmd/openbee/skill.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/skill"
)

func globalSkillsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot determine home dir:", err)
		os.Exit(1)
	}
	return filepath.Join(home, ".claude", "skills")
}

func newSkillManager() *skill.Manager {
	return skill.NewManager(openbeeStateDir(), globalSkillsDir())
}

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage Claude Code skills",
}

var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all skills (global view)",
	RunE: func(cmd *cobra.Command, args []string) error {
		m := newSkillManager()
		skills, err := m.List()
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tSOURCE\tVERSION")
		for _, s := range skills {
			fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, s.Source, s.ActiveVersion)
		}
		return w.Flush()
	},
}

var skillCreateDesc string
var skillCreateContent string

var skillCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new managed skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		content := skillCreateContent
		if content == "" {
			content = fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n# %s\n", name, skillCreateDesc, name)
		}
		m := newSkillManager()
		if err := m.Create(name, skillCreateDesc, content); err != nil {
			return err
		}
		fmt.Printf("Skill %q created (v1).\n", name)
		return nil
	},
}

var skillEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Edit skill content (opens $EDITOR, saves as new version)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		m := newSkillManager()

		cfg, err := m.LoadConfig()
		if err != nil {
			return err
		}
		entry, ok := cfg.Skills[name]
		if !ok {
			return fmt.Errorf("skill %q not found", name)
		}

		// Read current latest version content.
		registryRoot := filepath.Join(openbeeStateDir(), "skills")
		current, err := os.ReadFile(filepath.Join(registryRoot, name, entry.LatestVersion, "SKILL.md"))
		if err != nil {
			return fmt.Errorf("read current content: %w", err)
		}

		newContent, err := openInEditor(current)
		if err != nil {
			return err
		}
		if string(newContent) == string(current) {
			fmt.Println("No changes made.")
			return nil
		}

		if err := m.Edit(name, string(newContent)); err != nil {
			return err
		}
		newCfg, _ := m.LoadConfig()
		fmt.Printf("Skill %q saved as %s. Global still uses %s.\n",
			name, newCfg.Skills[name].LatestVersion, newCfg.Skills[name].GlobalVersion)
		fmt.Printf("To promote: openbee skill use %s %s --global\n", name, newCfg.Skills[name].LatestVersion)
		return nil
	},
}

var skillDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a managed skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m := newSkillManager()
		if err := m.Delete(args[0]); err != nil {
			return err
		}
		fmt.Printf("Skill %q deleted.\n", args[0])
		return nil
	},
}

var skillVersionsCmd = &cobra.Command{
	Use:   "versions <name>",
	Short: "List version history for a skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m := newSkillManager()
		versions, err := m.Versions(args[0])
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "VERSION\tCREATED AT")
		// Collect and sort version keys numerically.
		keys := make([]string, 0, len(versions))
		for v := range versions {
			keys = append(keys, v)
		}
		slices.SortFunc(keys, func(a, b string) int {
			na, _ := strconv.Atoi(strings.TrimPrefix(a, "v"))
			nb, _ := strconv.Atoi(strings.TrimPrefix(b, "v"))
			return na - nb
		})
		for _, v := range keys {
			e := versions[v]
			fmt.Fprintf(w, "%s\t%s\n", v, e.CreatedAt.Format("2006-01-02 15:04:05"))
		}
		return w.Flush()
	},
}

var skillUseWorker string
var skillUseGlobal bool

var skillUseCmd = &cobra.Command{
	Use:   "use <name> <version>",
	Short: "Switch active version (default: global)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, version := args[0], args[1]
		m := newSkillManager()
		if skillUseWorker != "" {
			return fmt.Errorf("--worker requires running with a config file; use the API instead")
		}
		if err := m.UseGlobal(name, version); err != nil {
			return err
		}
		fmt.Printf("Global skill %q now uses %s.\n", name, version)
		return nil
	},
}

var skillAdoptCmd = &cobra.Command{
	Use:   "adopt <name>",
	Short: "Adopt an externally-placed skill into openbee management",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m := newSkillManager()
		if err := m.AdoptGlobal(args[0]); err != nil {
			return err
		}
		fmt.Printf("Skill %q is now managed by openbee (v1).\n", args[0])
		return nil
	},
}

func init() {
	skillCreateCmd.Flags().StringVar(&skillCreateDesc, "description", "", "Skill description")
	skillCreateCmd.Flags().StringVar(&skillCreateContent, "content", "", "Initial SKILL.md content (default: generated template)")
	skillUseCmd.Flags().StringVar(&skillUseWorker, "worker", "", "Worker ID for worker-scoped version switch")
	skillUseCmd.Flags().BoolVar(&skillUseGlobal, "global", true, "Switch global version (default)")

	skillCmd.AddCommand(skillListCmd)
	skillCmd.AddCommand(skillCreateCmd)
	skillCmd.AddCommand(skillEditCmd)
	skillCmd.AddCommand(skillDeleteCmd)
	skillCmd.AddCommand(skillVersionsCmd)
	skillCmd.AddCommand(skillUseCmd)
	skillCmd.AddCommand(skillAdoptCmd)
	rootCmd.AddCommand(skillCmd)
}

// openInEditor writes content to a temp file, opens $EDITOR, and returns the modified content.
func openInEditor(content []byte) ([]byte, error) {
	f, err := os.CreateTemp("", "openbee-skill-*.md")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(content); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	return runEditor(editor, f.Name())
}
