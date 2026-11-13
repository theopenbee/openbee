package rpc_test

import (
	"os"
	"testing"

	"github.com/theopenbee/openbee/internal/codemode"
	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func TestMain(m *testing.M) {
	codemode.MaybeRunChild()
	if err := i18n.Load("en"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
