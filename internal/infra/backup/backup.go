package backup

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/theopenbee/openbee/internal/infra/utils"
	"golang.org/x/sync/errgroup"
	_ "modernc.org/sqlite"
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

	var eg errgroup.Group
	eg.Go(func() error {
		if err := snapshotDB(opts.DBPath, filepath.Join(tmp, "openbee.db")); err != nil {
			return fmt.Errorf("snapshot database: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		if err := utils.CopyFile(opts.ConfigPath, filepath.Join(tmp, "config.yaml")); err != nil {
			return fmt.Errorf("copy config: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		if err := copyDir(opts.StateDir, filepath.Join(tmp, "dot-openbee"), databaseFiles(opts.StateDir, opts.DBPath)); err != nil {
			return fmt.Errorf("copy state dir: %w", err)
		}
		return nil
	})
	if err := eg.Wait(); err != nil {
		return "", err
	}

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

	ts := now.Format("20060102-150405")
	baseName := fmt.Sprintf("openbee-backup-%s.tar.gz", ts)
	tarPath := filepath.Join(os.TempDir(), baseName)
	if err := PackTarGz(tarPath, tmp); err != nil {
		os.Remove(tarPath)
		return "", fmt.Errorf("pack archive: %w", err)
	}

	var finalName string
	if opts.Password != "" {
		encName := baseName + encExt
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
		if err := utils.MoveFile(tarPath, finalPath); err != nil {
			os.Remove(tarPath)
			return "", fmt.Errorf("move archive: %w", err)
		}
		finalName = finalPath
	}

	return finalName, nil
}

func snapshotDB(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", src+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`VACUUM INTO ?`, dst)
	return err
}

var dbSidecarSuffixes = []string{"-wal", "-shm", "-journal"}

func databaseFiles(stateDir, dbPath string) map[string]bool {
	dir, err := resolvePath(stateDir)
	if err != nil {
		return nil
	}
	db, err := resolvePath(dbPath)
	if err != nil {
		return nil
	}
	rel, err := filepath.Rel(dir, db)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	files := map[string]bool{rel: true}
	for _, suffix := range dbSidecarSuffixes {
		files[rel+suffix] = true
	}
	return files
}

func resolvePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return abs, nil
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

// copyDir recursively copies srcDir to dstDir.
// Symlinks are copied as symlinks; their targets are not followed.
func copyDir(srcDir, dstDir string, skip map[string]bool) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if skip[rel] {
			return nil
		}
		dst := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(target, dst)
		}
		return utils.CopyFile(path, dst)
	})
}

// hashDir returns FileEntry records for every regular file under dir,
// with paths relative to dir.
func hashDir(dir string) ([]FileEntry, error) {
	var entries []FileEntry
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
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
