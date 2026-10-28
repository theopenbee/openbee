package plugincmd

import (
	"os"

	"github.com/AlecAivazis/survey/v2"
	"golang.org/x/term"
)

type Prompter interface {
	Interactive() bool
	Confirm(message string, def bool) (bool, error)
	Input(message, help, def string, validate func(string) error) (string, error)
	Password(message, help string) (string, error)
	MultiSelect(message string, options []string) ([]int, error)
}

type surveyPrompter struct{}

func (surveyPrompter) Interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func (surveyPrompter) Confirm(message string, def bool) (bool, error) {
	v := def
	err := survey.AskOne(&survey.Confirm{Message: message, Default: def}, &v)
	return v, err
}

func (surveyPrompter) Input(message, help, def string, validate func(string) error) (string, error) {
	var v string
	var opts []survey.AskOpt
	if validate != nil {
		opts = append(opts, survey.WithValidator(func(ans any) error {
			s, _ := ans.(string)
			return validate(s)
		}))
	}
	err := survey.AskOne(&survey.Input{Message: message, Help: help, Default: def}, &v, opts...)
	return v, err
}

func (surveyPrompter) Password(message, help string) (string, error) {
	var v string
	err := survey.AskOne(&survey.Password{Message: message, Help: help}, &v)
	return v, err
}

func (surveyPrompter) MultiSelect(message string, options []string) ([]int, error) {
	var v []int
	err := survey.AskOne(&survey.MultiSelect{Message: message, Options: options, PageSize: 15}, &v)
	return v, err
}
