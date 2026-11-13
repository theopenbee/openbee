package rpc

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/theopenbee/openbee/internal/platform"
)

const (
	maxOptions      = 10
	maxOptionLength = 100
)

func validateOptions(options []string, content string) error {
	if len(options) == 0 {
		return nil
	}
	if content == "" {
		return errors.New("options need content")
	}
	if len(options) > maxOptions {
		return fmt.Errorf("at most %d options are allowed", maxOptions)
	}
	for _, o := range options {
		if strings.TrimSpace(o) == "" {
			return errors.New("options must not be empty")
		}
		if strings.ContainsAny(o, "\r\n") {
			return errors.New("options must be a single line")
		}
		if utf8.RuneCountInString(o) > maxOptionLength {
			return fmt.Errorf("each option must be at most %d characters", maxOptionLength)
		}
	}
	return nil
}

func (s *Server) optionButtons(workerID string, options []string) []platform.Button {
	prefix := ""
	if workerID != "" {
		if w, err := s.workerStore.GetByID(workerID); err == nil && w.Name != "" && !strings.ContainsAny(w.Name, " \t\r\n") {
			prefix = "@" + w.Name + " "
		}
	}
	buttons := make([]platform.Button, 0, len(options))
	for _, o := range options {
		label := strings.TrimSpace(o)
		buttons = append(buttons, platform.Button{Label: label, Text: prefix + label})
	}
	return buttons
}

func appendOptionList(content string, options []string) string {
	lines := make([]string, 0, len(options))
	for i, o := range options {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, strings.TrimSpace(o)))
	}
	return content + "\n\n" + strings.Join(lines, "\n")
}
