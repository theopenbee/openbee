package ctlcmd

import "github.com/spf13/cobra"

// setIfNonEmpty assigns m[key] = val when val is not the empty string.
func setIfNonEmpty(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}

// setIfPositive assigns m[key] = val when val > 0.
func setIfPositive(m map[string]any, key string, val int) {
	if val > 0 {
		m[key] = val
	}
}

// setIfPositiveInt64 assigns m[key] = val when val > 0.
func setIfPositiveInt64(m map[string]any, key string, val int64) {
	if val > 0 {
		m[key] = val
	}
}

// setIfFlagChanged assigns m[key] = val when the cobra flag has been set on the command line.
func setIfFlagChanged(c *cobra.Command, m map[string]any, flag, key string, val any) {
	if c.Flags().Changed(flag) {
		m[key] = val
	}
}
