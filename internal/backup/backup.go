package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// BackupOptions configures a Backup call.
type BackupOptions struct {
	DBPath     string // path to openbee.db
	ConfigPath string // path to config.yaml
	StateDir   string // path to ~/.openbee/
	OutputDir  string // directory where the archive is written
	AppVersion string // openbee binary version (for manifest)
	Password   string // if non-empty, encrypt the archive
}

// Backup creates a compressed archive of DBPath, ConfigPath, and StateDir in OutputDir.
// It returns the full path of the created archive file.
// On failure it removes any partially-written output and returns an error.
func Backup(opts BackupOptions) (string, error) {
	now := time.Now().UTC()
	tmp, err := os.MkdirTemp("", "openbee-backup-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	// 1. Copy database.
	if err := copyFile(opts.DBPath, filepath.Join(tmp, "openbee.db")); err != nil {
		return "", fmt.Errorf("copy database: %w", err)
	}

	// 2. Copy config.
	if err := copyFile(opts.ConfigPath, filepath.Join(tmp, "config.yaml")); err != nil {
		return "", fmt.Errorf("copy config: %w", err)
	}

	// 3. Copy state directory.
	if err := copyDir(opts.StateDir, filepath.Join(tmp, "dot-openbee")); err != nil {
		return "", fmt.Errorf("copy state dir: %w", err)
	}

	// 4. Compute checksums and write manifest.
	entries, err := hashDir(tmp)
	if err != nil {
		return "", fmt.Errorf("hash files: %w", err)
	}
	m := Manifest{
		Version:        "1",
		OpenbeeVersion: opts.AppVersion,
		CreatedAt:      now.Format(time.RFC3339),
		Files:          entries,
	}
	if err := WriteManifest(filepath.Join(tmp, "manifest.json"), m); err != nil {
		return "", fmt.Errorf("write manifest: %w", err)
	}

	// 5. Pack into tar.gz.
	ts := now.Format("20060102-150405")
	baseName := fmt.Sprintf("openbee-backup-%s.tar.gz", ts)
	tarPath := filepath.Join(os.TempDir(), baseName)
	if err := PackTarGz(tarPath, tmp); err != nil {
		os.Remove(tarPath)
		return "", fmt.Errorf("pack archive: %w", err)
	}

	// 6. Optionally encrypt.
	var finalName string
	if opts.Password != "" {
		encName := baseName + ".enc"
		encPath := filepath.Join(opts.OutputDir, encName)
		if err := EncryptFile(tarPath, encPath, opts.Password); err != nil {
			os.Remove(tarPath)
			os.Remove(encPath)
			return "", fmt.Errorf("encrypt archive: %w", err)
		}
		os.Remove(tarPath)
		finalName = encPath
	} else {
		finalPath := filepath.Join(opts.OutputDir, baseName)
		if err := os.Rename(tarPath, finalPath); err != nil {
			// Rename across devices may fail; fall back to copy+delete.
			if err2 := copyFile(tarPath, finalPath); err2 != nil {
				os.Remove(tarPath)
				return "", fmt.Errorf("move archive: %w", err2)
			}
			os.Remove(tarPath)
		}
		finalName = finalPath
	}

	return finalName, nil
}

// copyFile copies the file at src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// copyDir recursively copies srcDir to dstDir.
func copyDir(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		return copyFile(path, dst)
	})
}

// hashDir returns FileEntry records for every regular file under dir,
// with paths relative to dir.
func hashDir(dir string) ([]FileEntry, error) {
	var entries []FileEntry
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		sum, err := SHA256File(path)
		if err != nil {
			return err
		}
		entries = append(entries, FileEntry{Path: filepath.ToSlash(rel), SHA256: sum})
		return nil
	})
	return entries, err
}
