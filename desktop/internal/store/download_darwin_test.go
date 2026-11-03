package store_test

import (
	"strings"
	"testing"

	"github.com/theopenbee/openbee/desktop/internal/fakeapi"
	"golang.org/x/sys/unix"
)

func TestDownloadedMediaIsQuarantined(t *testing.T) {
	st, _, fix := loaded(t, func(st *fakeapi.State) {
		team(st)
		st.Media["m1_report.pdf"] = []byte("%PDF-1.7")
	})
	var locals []string
	collect := func(p string, err error) {
		if err != nil {
			t.Errorf("fetch media: %v", err)
		}
		locals = append(locals, p)
	}
	st.MediaFile("m1_report.pdf", collect)
	st.SaveMedia("m1_report.pdf", collect)
	fix.Settle(t)
	if len(locals) != 2 || locals[0] == locals[1] {
		t.Fatalf("local files = %q, want a cached and a saved copy", locals)
	}
	for _, local := range locals {
		buf := make([]byte, 256)
		n, err := unix.Getxattr(local, "com.apple.quarantine", buf)
		if err != nil {
			t.Fatalf("quarantine attribute of %s: %v", local, err)
		}
		if value := string(buf[:n]); !strings.HasPrefix(value, "0081;") || !strings.HasSuffix(value, ";OpenBee;") {
			t.Fatalf("quarantine attribute of %s = %q, want 0081;<time>;OpenBee;", local, value)
		}
	}
}
