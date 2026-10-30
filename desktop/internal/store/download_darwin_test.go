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
	var local string
	st.MediaFile("m1_report.pdf", func(p string, err error) {
		if err != nil {
			t.Errorf("MediaFile: %v", err)
		}
		local = p
	})
	fix.Settle(t)
	buf := make([]byte, 256)
	n, err := unix.Getxattr(local, "com.apple.quarantine", buf)
	if err != nil {
		t.Fatalf("quarantine attribute of %s: %v", local, err)
	}
	if value := string(buf[:n]); !strings.HasPrefix(value, "0081;") || !strings.HasSuffix(value, ";OpenBee;") {
		t.Fatalf("quarantine attribute = %q, want 0081;<time>;OpenBee;", value)
	}
}
