//go:build !darwin && !windows

package store

func markDownloaded(string) error {
	return nil
}
