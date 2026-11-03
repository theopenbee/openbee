package settings

import "github.com/theopenbee/openbee/desktop/internal/kit"

func StateTone(state string) kit.Tone {
	return stateLooks[state].tone
}
