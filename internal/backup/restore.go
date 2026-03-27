package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RestoreOptions configures a Restore call.
type RestoreOptions struct {
	ArchivePath string // path to .tar.gz or .tar.gz.enc
	DBPath      string // destination for openbee.db
	ConfigPath  string // destination for config.yaml
	StateDir    string // destination for ~/.openbee/
	AppVersion  string // running openbee version (for compatibility check)
	Force       bool   // overwrite existing data without error
	Password    string // required when ArchivePath ends in .enc
}

// Restore extracts a backup archive and copies files to their configured locations.
// It returns an error if data already exists at the destinations and Force is false,
// or if the archive is encrypted and Password is empty/wrong.
func Restore(opts RestoreOptions) error {
	// 1. Check existing data.
	if !opts.Force {
		if _, err := os.Stat(opts.DBPath); err == nil {
			return fmt.Errorf("destination database %s already exists; use --force to overwrite", opts.DBPath)
		}
	}

	// 2. Decrypt if needed.
	tarPath := opts.ArchivePath
	if strings.HasSuffix(opts.ArchivePath, ".enc") {
		if opts.Password == "" {
			return fmt.Errorf("archive is encrypted; provide --password")
		}
		tmp, err := os.CreateTemp("", "openbee-restore-*.tar.gz")
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
		tmp.Close()
		tarPath = tmp.Name()
		defer os.Remove(tarPath)

		if err := DecryptFile(opts.ArchivePath, tarPath, opts.Password); err != nil {
			return err // already contains "incorrect password or corrupted file"
		}
	}

	// 3. Unpack to a temp dir.
	extractDir, err := os.MkdirTemp("", "openbee-restore-extract-*")
	if err != nil {
		return fmt.Errorf("create extract dir: %w", err)
	}
	defer os.RemoveAll(extractDir)

	if err := UnpackTarGz(tarPath, extractDir); err != nil {
		return fmt.Errorf("unpack archive: %w", err)
	}

	// 4. Read and validate manifest.
	m, err := ReadManifest(filepath.Join(extractDir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	if m.OpenbeeVersion != opts.AppVersion {
		fmt.Fprintf(os.Stderr, "warning: backup was created with openbee %s, current version is %s\n",
			m.OpenbeeVersion, opts.AppVersion)
	}

	// Verify checksums from manifest.
	for _, fe := range m.Files {
		path := filepath.Join(extractDir, filepath.FromSlash(fe.Path))
		sum, err := SHA256File(path)
		if err != nil {
			return fmt.Errorf("verify %s: %w", fe.Path, err)
		}
		if sum != fe.SHA256 {
			return fmt.Errorf("checksum mismatch for %s: archive may be corrupted", fe.Path)
		}
	}

	// 5. Restore files.
	if err := os.MkdirAll(filepath.Dir(opts.DBPath), 0755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}
	if err := copyFile(filepath.Join(extractDir, "openbee.db"), opts.DBPath); err != nil {
		return fmt.Errorf("restore database: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(opts.ConfigPath), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := copyFile(filepath.Join(extractDir, "config.yaml"), opts.ConfigPath); err != nil {
		return fmt.Errorf("restore config: %w", err)
	}

	if err := copyDir(filepath.Join(extractDir, "dot-openbee"), opts.StateDir); err != nil {
		return fmt.Errorf("restore state dir: %w", err)
	}

	return nil
}
