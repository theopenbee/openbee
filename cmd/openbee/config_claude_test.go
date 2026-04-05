package main

import (
	"os"
	"testing"

	"github.com/theopenbee/openbee/internal/infra/i18n"
)

func TestMain(m *testing.M) {
	// Load default locale so that i18n.M fields are populated for all tests.
	if err := i18n.Load("en"); err != nil {
		panic("i18n.Load failed: " + err.Error())
	}
	os.Exit(m.Run())
}
