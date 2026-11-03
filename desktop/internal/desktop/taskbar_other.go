//go:build !windows

package desktop

func setOverlay(uintptr, []byte, string) error {
	return nil
}
