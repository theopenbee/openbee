// Package buildinfo exposes build-time metadata (version, commit, date)
// injected via -ldflags at compile time. main populates these from its own
// ldflags-injected vars at startup so any layer (e.g. the HTTP API) can read
// them without threading the values through every constructor.
package buildinfo

// Defaults mirror the fallbacks in cmd/openbee/main.go for non-release builds.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Info is an immutable snapshot of the build metadata.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// Set records the build metadata. Called once from main at startup.
func Set(version, commit, date string) {
	if version != "" {
		Version = version
	}
	if commit != "" {
		Commit = commit
	}
	if date != "" {
		Date = date
	}
}

// Get returns the current build metadata snapshot.
func Get() Info {
	return Info{Version: Version, Commit: Commit, Date: Date}
}
