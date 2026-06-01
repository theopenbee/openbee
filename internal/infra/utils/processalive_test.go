package utils

import (
	"os"
	"testing"
)

func TestIsProcessAlive_Self(t *testing.T) {
	if !IsProcessAlive(os.Getpid()) {
		t.Fatal("current process must be alive")
	}
}

func TestIsProcessAlive_InvalidPID(t *testing.T) {
	if IsProcessAlive(0) {
		t.Fatal("pid 0 must not be reported alive")
	}
	if IsProcessAlive(-1) {
		t.Fatal("negative pid must not be reported alive")
	}
}

func TestIsProcessAlive_LikelyDeadPID(t *testing.T) {
	const pid = 999999
	if IsProcessAlive(pid) {
		t.Skip("pid 999999 happened to exist")
	}
}
