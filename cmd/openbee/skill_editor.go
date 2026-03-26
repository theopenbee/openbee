// cmd/openbee/skill_editor.go
package main

import (
	"os"
	"os/exec"
)

func runEditor(editor, path string) ([]byte, error) {
	cmd := exec.Command(editor, path) //nolint:gosec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
