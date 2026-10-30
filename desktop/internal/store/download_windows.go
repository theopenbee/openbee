package store

import "os"

const zoneIdentifier = "[ZoneTransfer]\r\nZoneId=3\r\n"

func markDownloaded(path string) error {
	return os.WriteFile(path+":Zone.Identifier", []byte(zoneIdentifier), 0o644)
}
