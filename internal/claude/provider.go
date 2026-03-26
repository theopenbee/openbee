package claude

import (
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2/terminal"
)

// ErrInterrupted is returned when the user cancels an interactive prompt (Ctrl+C).
var ErrInterrupted = errors.New("interrupted")

// HandleSurveyErr converts a survey interrupt into ErrInterrupted and passes
// other errors through unchanged. It is exported so that cmd-layer code can
// share the same sentinel error without duplicating the check.
func HandleSurveyErr(err error) error {
	if errors.Is(err, terminal.InterruptErr) {
		fmt.Println("\nCancelled.")
		return ErrInterrupted
	}
	return err
}
