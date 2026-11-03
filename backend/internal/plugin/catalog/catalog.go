package catalog

import "github.com/theopenbee/openbee/internal/plugin"

const (
	SchemaVersion = 1
	PackageName   = "@theopenbee/plugins"
)

var DefaultRegistries = []string{"https://registry.npmjs.org", "https://registry.npmmirror.com"}

type Vocab struct {
	ID   string               `json:"id"`
	Name plugin.LocalizedText `json:"name"`
}

type Entry struct {
	Category          string               `json:"category"`
	ID                string               `json:"id"`
	Package           string               `json:"package"`
	Version           string               `json:"version"`
	Official          bool                 `json:"official"`
	Name              plugin.LocalizedText `json:"name"`
	Description       plugin.LocalizedText `json:"description,omitempty"`
	ProtocolVersion   int                  `json:"protocol_version"`
	License           string               `json:"license"`
	Repository        string               `json:"repository"`
	Maintainers       []string             `json:"maintainers"`
	Tags              []string             `json:"tags"`
	MinOpenbeeVersion string               `json:"min_openbee_version,omitempty"`
	Deprecated        plugin.LocalizedText `json:"deprecated,omitempty"`
	PublishedAt       string               `json:"published_at,omitempty"`
	Integrity         string               `json:"integrity"`
	Tarball           string               `json:"tarball"`
}

type Catalog struct {
	SchemaVersion int     `json:"schema_version"`
	Categories    []Vocab `json:"categories"`
	Tags          []Vocab `json:"tags"`
	Plugins       []Entry `json:"plugins"`
}
