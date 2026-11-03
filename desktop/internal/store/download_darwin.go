package store

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

const quarantineAttr = "com.apple.quarantine"

func markDownloaded(path string) error {
	value := fmt.Sprintf("0081;%x;OpenBee;", time.Now().Unix())
	return unix.Setxattr(path, quarantineAttr, []byte(value), 0)
}
