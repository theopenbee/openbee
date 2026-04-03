package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
)

// Manifest describes the contents of a backup archive.
type Manifest struct {
	Version        string      `json:"version"`
	OpenbeeVersion string      `json:"openbee_version"`
	CreatedAt      string      `json:"created_at"`
	Files          []FileEntry `json:"files"`
}

// FileEntry records the archive-relative path and SHA256 checksum of one file.
type FileEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// WriteManifest serialises m as indented JSON to path.
func WriteManifest(path string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ReadManifest deserialises a manifest from path.
func ReadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// SHA256File returns the hex-encoded SHA256 digest of the file at path.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
