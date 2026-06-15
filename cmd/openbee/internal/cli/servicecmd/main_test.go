package servicecmd

import (
	"os"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func TestMain(m *testing.M) {
	if err := i18n.Load("en"); err != nil {
		panic("i18n.Load: " + err.Error())
	}
	os.Exit(m.Run())
}
