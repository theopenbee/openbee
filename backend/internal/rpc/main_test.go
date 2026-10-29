package rpc_test

import (
	"os"
	"testing"

	"github.com/theopenbee/openbee/internal/codemode"
)

func TestMain(m *testing.M) {
	codemode.MaybeRunChild()
	os.Exit(m.Run())
}
