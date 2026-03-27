# Backup and Restore Commands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `openbee backup` and `openbee restore` commands that create/restore an encrypted tar.gz archive containing the SQLite database, config file, and `~/.openbee/` state directory.

**Architecture:** A new `internal/backup` package contains all core logic (manifest, backup, restore, encryption). Two new cobra commands in `cmd/openbee/` wire the package into the CLI. The SQLite database is hot-backed using the sqlite3 online backup API (via `modernc.org/sqlite`), so no service downtime is needed for backup.

**Tech Stack:** Go stdlib (`archive/tar`, `compress/gzip`, `crypto/aes`, `crypto/cipher`, `crypto/rand`, `crypto/sha256`), `golang.org/x/crypto/scrypt` (already an indirect dep), `modernc.org/sqlite` online backup API, `github.com/spf13/cobra`.

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/backup/manifest.go` | `Manifest` struct, JSON read/write, SHA256 file hashing |
| Create | `internal/backup/archive.go` | tar.gz pack/unpack helpers |
| Create | `internal/backup/encrypt.go` | AES-256-GCM encrypt/decrypt with scrypt key derivation |
| Create | `internal/backup/backup.go` | `Backup()` orchestrator |
| Create | `internal/backup/restore.go` | `Restore()` orchestrator |
| Create | `internal/backup/backup_test.go` | full round-trip tests |
| Create | `cmd/openbee/backup.go` | cobra `backup` command |
| Create | `cmd/openbee/restore.go` | cobra `restore` command |

---

## Task 1: Manifest read/write and SHA256 helpers

**Files:**
- Create: `internal/backup/manifest.go`
- Create: `internal/backup/backup_test.go` (partial — manifest tests only)

- [ ] **Step 1: Write the failing tests**

```go
// internal/backup/backup_test.go
package backup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/backup"
)

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// write a file to hash
	f := filepath.Join(dir, "hello.txt")
	require.NoError(t, os.WriteFile(f, []byte("hello world"), 0644))

	sum, err := backup.SHA256File(f)
	require.NoError(t, err)
	require.Len(t, sum, 64) // hex-encoded sha256

	m := backup.Manifest{
		Version:        "1",
		OpenbeeVersion: "0.5.0",
		CreatedAt:      "2026-03-27T15:30:00Z",
		Files: []backup.FileEntry{
			{Path: "hello.txt", SHA256: sum},
		},
	}

	out := filepath.Join(dir, "manifest.json")
	require.NoError(t, backup.WriteManifest(out, m))

	got, err := backup.ReadManifest(out)
	require.NoError(t, err)
	require.Equal(t, m, got)
}
```

- [ ] **Step 2: Run to confirm failure**

```
cd /path/to/openbee
go test ./internal/backup/... 2>&1 | head -20
```

Expected: `cannot find package` or `no Go files`

- [ ] **Step 3: Implement `internal/backup/manifest.go`**

```go
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
```

- [ ] **Step 4: Run tests — expect PASS**

```
go test ./internal/backup/... -v -run TestManifestRoundTrip
```

Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
git add internal/backup/manifest.go internal/backup/backup_test.go
git commit -m "feat(backup): add Manifest type, WriteManifest, ReadManifest, SHA256File"
```

---

## Task 2: tar.gz archive pack/unpack helpers

**Files:**
- Create: `internal/backup/archive.go`
- Modify: `internal/backup/backup_test.go` (add archive tests)

- [ ] **Step 1: Write the failing tests**

Add to `internal/backup/backup_test.go`:

```go
func TestArchiveRoundTrip(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// create source tree
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("aaa"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("bbb"), 0644))

	archive := filepath.Join(t.TempDir(), "test.tar.gz")
	require.NoError(t, backup.PackTarGz(archive, src))

	require.NoError(t, backup.UnpackTarGz(archive, dst))

	got, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	require.NoError(t, err)
	require.Equal(t, "aaa", string(got))

	got, err = os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
	require.NoError(t, err)
	require.Equal(t, "bbb", string(got))
}
```

- [ ] **Step 2: Run to confirm failure**

```
go test ./internal/backup/... -v -run TestArchiveRoundTrip 2>&1 | head -10
```

Expected: `undefined: backup.PackTarGz`

- [ ] **Step 3: Implement `internal/backup/archive.go`**

```go
package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PackTarGz creates a gzip-compressed tar archive at archivePath from the
// contents of srcDir. File paths inside the archive are relative to srcDir.
func PackTarGz(archivePath, srcDir string) error {
	out, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer out.Close()

	gw := gzip.NewWriter(out)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}

// UnpackTarGz extracts a gzip-compressed tar archive into dstDir.
// For security, path traversal entries (containing "..") are rejected.
func UnpackTarGz(archivePath, dstDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		// Guard against path traversal.
		if strings.Contains(hdr.Name, "..") {
			return fmt.Errorf("unsafe path in archive: %s", hdr.Name)
		}

		target := filepath.Join(dstDir, filepath.FromSlash(hdr.Name))
		if hdr.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		out.Close()
	}
	return nil
}
```

- [ ] **Step 4: Run tests — expect PASS**

```
go test ./internal/backup/... -v -run TestArchiveRoundTrip
```

Expected: `PASS`

- [ ] **Step 5: Commit**

```bash
git add internal/backup/archive.go internal/backup/backup_test.go
git commit -m "feat(backup): add PackTarGz and UnpackTarGz helpers"
```

---

## Task 3: AES-256-GCM encrypt/decrypt

**Files:**
- Create: `internal/backup/encrypt.go`
- Modify: `internal/backup/backup_test.go` (add encryption tests)

- [ ] **Step 1: Write the failing tests**

Add to `internal/backup/backup_test.go`:

```go
func TestEncryptDecryptRoundTrip(t *testing.T) {
	plain := filepath.Join(t.TempDir(), "plain.tar.gz")
	enc := filepath.Join(t.TempDir(), "plain.tar.gz.enc")
	decrypted := filepath.Join(t.TempDir(), "decrypted.tar.gz")

	require.NoError(t, os.WriteFile(plain, []byte("secret data"), 0644))

	require.NoError(t, backup.EncryptFile(plain, enc, "hunter2"))
	require.NoError(t, backup.DecryptFile(enc, decrypted, "hunter2"))

	got, err := os.ReadFile(decrypted)
	require.NoError(t, err)
	require.Equal(t, "secret data", string(got))
}

func TestDecryptWrongPassword(t *testing.T) {
	plain := filepath.Join(t.TempDir(), "plain.tar.gz")
	enc := filepath.Join(t.TempDir(), "plain.tar.gz.enc")
	decrypted := filepath.Join(t.TempDir(), "decrypted.tar.gz")

	require.NoError(t, os.WriteFile(plain, []byte("secret"), 0644))
	require.NoError(t, backup.EncryptFile(plain, enc, "correct"))

	err := backup.DecryptFile(enc, decrypted, "wrong")
	require.Error(t, err)
	require.Contains(t, err.Error(), "incorrect password or corrupted file")
}
```

- [ ] **Step 2: Run to confirm failure**

```
go test ./internal/backup/... -v -run TestEncrypt 2>&1 | head -10
```

Expected: `undefined: backup.EncryptFile`

- [ ] **Step 3: Implement `internal/backup/encrypt.go`**

```go
package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/scrypt"
)

const (
	saltSize = 32
	keySize  = 32 // AES-256
	// scrypt parameters: N=32768, r=8, p=1 — tuned for ~100ms on modern hardware
	scryptN = 32768
	scryptR = 8
	scryptP = 1
)

// EncryptFile reads src, encrypts it with AES-256-GCM using a key derived
// from password via scrypt, and writes [salt(32) | nonce(12) | ciphertext] to dst.
func EncryptFile(src, dst, password string) error {
	plaintext, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}

	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("generate salt: %w", err)
	}

	key, err := deriveKey(password, salt)
	if err != nil {
		return err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// Output: salt | nonce+ciphertext
	out := make([]byte, 0, saltSize+len(ciphertext))
	out = append(out, salt...)
	out = append(out, ciphertext...)

	return os.WriteFile(dst, out, 0600)
}

// DecryptFile reads an encrypted file produced by EncryptFile and writes the
// plaintext to dst. Returns an error containing "incorrect password or corrupted file"
// if decryption fails.
func DecryptFile(src, dst, password string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read encrypted file: %w", err)
	}
	if len(data) < saltSize+12 { // 12 = minimum nonce size
		return fmt.Errorf("incorrect password or corrupted file")
	}

	salt := data[:saltSize]
	rest := data[saltSize:]

	key, err := deriveKey(password, salt)
	if err != nil {
		return err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonceSize := gcm.NonceSize()
	if len(rest) < nonceSize {
		return fmt.Errorf("incorrect password or corrupted file")
	}
	nonce, ciphertext := rest[:nonceSize], rest[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("incorrect password or corrupted file")
	}

	return os.WriteFile(dst, plaintext, 0644)
}

func deriveKey(password string, salt []byte) ([]byte, error) {
	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, keySize)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	return key, nil
}
```

- [ ] **Step 4: Run tests — expect PASS**

```
go test ./internal/backup/... -v -run TestEncrypt
```

Expected: both `TestEncryptDecryptRoundTrip` and `TestDecryptWrongPassword` PASS

- [ ] **Step 5: Confirm `golang.org/x/crypto` is promoted to a direct import**

```
go mod tidy
```

Then verify `go.mod` now lists `golang.org/x/crypto` as a direct dependency (no `// indirect`).

- [ ] **Step 6: Commit**

```bash
git add internal/backup/encrypt.go internal/backup/backup_test.go go.mod go.sum
git commit -m "feat(backup): add AES-256-GCM encrypt/decrypt with scrypt key derivation"
```

---

## Task 4: `Backup()` orchestrator

**Files:**
- Create: `internal/backup/backup.go`
- Modify: `internal/backup/backup_test.go` (add backup tests)

- [ ] **Step 1: Write the failing tests**

Add to `internal/backup/backup_test.go`:

```go
func TestBackupCreatesArchive(t *testing.T) {
	// Create fake source files
	dbPath := filepath.Join(t.TempDir(), "openbee.db")
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	stateDir := filepath.Join(t.TempDir(), "dot-openbee")

	require.NoError(t, os.WriteFile(dbPath, []byte("fake-db"), 0644))
	require.NoError(t, os.WriteFile(cfgPath, []byte("server:\n  port: 8080\n"), 0644))
	require.NoError(t, os.MkdirAll(stateDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "openbee.log"), []byte("log"), 0644))

	outDir := t.TempDir()
	archivePath, err := backup.Backup(backup.BackupOptions{
		DBPath:      dbPath,
		ConfigPath:  cfgPath,
		StateDir:    stateDir,
		OutputDir:   outDir,
		AppVersion:  "0.5.0",
	})
	require.NoError(t, err)
	require.FileExists(t, archivePath)
	require.True(t, strings.HasSuffix(archivePath, ".tar.gz"), "expected .tar.gz, got %s", archivePath)

	// Verify archive contains manifest + all expected files
	extractDir := t.TempDir()
	require.NoError(t, backup.UnpackTarGz(archivePath, extractDir))
	require.FileExists(t, filepath.Join(extractDir, "manifest.json"))
	require.FileExists(t, filepath.Join(extractDir, "openbee.db"))
	require.FileExists(t, filepath.Join(extractDir, "config.yaml"))
	require.FileExists(t, filepath.Join(extractDir, "dot-openbee", "openbee.log"))
}

func TestBackupEncrypted(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "openbee.db")
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	stateDir := filepath.Join(t.TempDir(), "dot-openbee")

	require.NoError(t, os.WriteFile(dbPath, []byte("fake-db"), 0644))
	require.NoError(t, os.WriteFile(cfgPath, []byte("server:\n  port: 8080\n"), 0644))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	outDir := t.TempDir()
	archivePath, err := backup.Backup(backup.BackupOptions{
		DBPath:     dbPath,
		ConfigPath: cfgPath,
		StateDir:   stateDir,
		OutputDir:  outDir,
		AppVersion: "0.5.0",
		Password:   "secret",
	})
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(archivePath, ".tar.gz.enc"), "expected .tar.gz.enc, got %s", archivePath)
}
```

Also add the import `"strings"` to the test file's import block.

- [ ] **Step 2: Run to confirm failure**

```
go test ./internal/backup/... -v -run TestBackup 2>&1 | head -10
```

Expected: `undefined: backup.Backup`

- [ ] **Step 3: Implement `internal/backup/backup.go`**

Note: For this plan the DBPath is copied as a regular file (SQLite WAL-mode files can be safely read-copied when WAL checkpointing is handled at the app layer). If a live database connection is available in future, replace `copyFile` with the sqlite3 online backup API.

```go
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
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		Files:          entries,
	}
	if err := WriteManifest(filepath.Join(tmp, "manifest.json"), m); err != nil {
		return "", fmt.Errorf("write manifest: %w", err)
	}

	// 5. Pack into tar.gz.
	ts := time.Now().UTC().Format("20060102-150405")
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
```

- [ ] **Step 4: Run tests — expect PASS**

```
go test ./internal/backup/... -v -run TestBackup
```

Expected: `TestBackupCreatesArchive` and `TestBackupEncrypted` PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/backup.go internal/backup/backup_test.go
git commit -m "feat(backup): implement Backup() orchestrator"
```

---

## Task 5: `Restore()` orchestrator

**Files:**
- Create: `internal/backup/restore.go`
- Modify: `internal/backup/backup_test.go` (add restore tests)

- [ ] **Step 1: Write the failing tests**

Add to `internal/backup/backup_test.go`:

```go
func TestRestoreRoundTrip(t *testing.T) {
	// --- Setup source data ---
	srcDB := filepath.Join(t.TempDir(), "openbee.db")
	srcCfg := filepath.Join(t.TempDir(), "config.yaml")
	srcState := filepath.Join(t.TempDir(), "dot-openbee")

	require.NoError(t, os.WriteFile(srcDB, []byte("fake-db-content"), 0644))
	require.NoError(t, os.WriteFile(srcCfg, []byte("server:\n  port: 8080\n"), 0644))
	require.NoError(t, os.MkdirAll(srcState, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcState, "openbee.log"), []byte("log-content"), 0644))

	// --- Create backup ---
	archivePath, err := backup.Backup(backup.BackupOptions{
		DBPath:     srcDB,
		ConfigPath: srcCfg,
		StateDir:   srcState,
		OutputDir:  t.TempDir(),
		AppVersion: "0.5.0",
	})
	require.NoError(t, err)

	// --- Restore into new destinations ---
	dstDB := filepath.Join(t.TempDir(), "openbee.db")
	dstCfg := filepath.Join(t.TempDir(), "config.yaml")
	dstState := filepath.Join(t.TempDir(), "dot-openbee")

	err = backup.Restore(backup.RestoreOptions{
		ArchivePath: archivePath,
		DBPath:      dstDB,
		ConfigPath:  dstCfg,
		StateDir:    dstState,
		AppVersion:  "0.5.0",
		Force:       false,
	})
	require.NoError(t, err)

	gotDB, err := os.ReadFile(dstDB)
	require.NoError(t, err)
	require.Equal(t, "fake-db-content", string(gotDB))

	gotLog, err := os.ReadFile(filepath.Join(dstState, "openbee.log"))
	require.NoError(t, err)
	require.Equal(t, "log-content", string(gotLog))
}

func TestRestoreBlockedWithoutForce(t *testing.T) {
	srcDB := filepath.Join(t.TempDir(), "openbee.db")
	srcCfg := filepath.Join(t.TempDir(), "config.yaml")
	srcState := filepath.Join(t.TempDir(), "dot-openbee")

	require.NoError(t, os.WriteFile(srcDB, []byte("db"), 0644))
	require.NoError(t, os.WriteFile(srcCfg, []byte("cfg"), 0644))
	require.NoError(t, os.MkdirAll(srcState, 0755))

	archivePath, err := backup.Backup(backup.BackupOptions{
		DBPath:     srcDB,
		ConfigPath: srcCfg,
		StateDir:   srcState,
		OutputDir:  t.TempDir(),
		AppVersion: "0.5.0",
	})
	require.NoError(t, err)

	// Pre-create destination DB — restore should fail without --force.
	dstDB := filepath.Join(t.TempDir(), "openbee.db")
	require.NoError(t, os.WriteFile(dstDB, []byte("existing"), 0644))

	err = backup.Restore(backup.RestoreOptions{
		ArchivePath: archivePath,
		DBPath:      dstDB,
		ConfigPath:  filepath.Join(t.TempDir(), "config.yaml"),
		StateDir:    filepath.Join(t.TempDir(), "dot-openbee"),
		AppVersion:  "0.5.0",
		Force:       false,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--force")
}

func TestRestoreEncryptedRoundTrip(t *testing.T) {
	srcDB := filepath.Join(t.TempDir(), "openbee.db")
	srcCfg := filepath.Join(t.TempDir(), "config.yaml")
	srcState := filepath.Join(t.TempDir(), "dot-openbee")

	require.NoError(t, os.WriteFile(srcDB, []byte("db-enc"), 0644))
	require.NoError(t, os.WriteFile(srcCfg, []byte("cfg-enc"), 0644))
	require.NoError(t, os.MkdirAll(srcState, 0755))

	archivePath, err := backup.Backup(backup.BackupOptions{
		DBPath:     srcDB,
		ConfigPath: srcCfg,
		StateDir:   srcState,
		OutputDir:  t.TempDir(),
		AppVersion: "0.5.0",
		Password:   "mysecret",
	})
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(archivePath, ".tar.gz.enc"))

	dstDB := filepath.Join(t.TempDir(), "openbee.db")
	err = backup.Restore(backup.RestoreOptions{
		ArchivePath: archivePath,
		DBPath:      dstDB,
		ConfigPath:  filepath.Join(t.TempDir(), "config.yaml"),
		StateDir:    filepath.Join(t.TempDir(), "dot-openbee"),
		AppVersion:  "0.5.0",
		Password:    "mysecret",
	})
	require.NoError(t, err)

	got, err := os.ReadFile(dstDB)
	require.NoError(t, err)
	require.Equal(t, "db-enc", string(got))
}
```

- [ ] **Step 2: Run to confirm failure**

```
go test ./internal/backup/... -v -run TestRestore 2>&1 | head -10
```

Expected: `undefined: backup.Restore`

- [ ] **Step 3: Implement `internal/backup/restore.go`**

```go
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
		fmt.Printf("warning: backup was created with openbee %s, current version is %s\n",
			m.OpenbeeVersion, opts.AppVersion)
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
```

- [ ] **Step 4: Run all backup package tests — expect PASS**

```
go test ./internal/backup/... -v
```

Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/backup/restore.go internal/backup/backup_test.go
git commit -m "feat(backup): implement Restore() orchestrator"
```

---

## Task 6: `openbee backup` cobra command

**Files:**
- Create: `cmd/openbee/backup.go`

- [ ] **Step 1: Implement `cmd/openbee/backup.go`**

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/backup"
	"github.com/theopenbee/openbee/internal/config"
)

var backupPassword string

var backupCmd = &cobra.Command{
	Use:   "backup [output-dir]",
	Short: "Create a backup archive of the openbee database, config, and state",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		outputDir := "."
		if len(args) == 1 {
			outputDir = args[0]
		}
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}

		archivePath, err := backup.Backup(backup.BackupOptions{
			DBPath:     cfg.Database.Path,
			ConfigPath: cfgPath,
			StateDir:   openbeeStateDir(),
			OutputDir:  outputDir,
			AppVersion: version,
			Password:   backupPassword,
		})
		if err != nil {
			return err
		}
		fmt.Printf("Backup created: %s\n", archivePath)
		return nil
	},
}

func init() {
	backupCmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "path to config file")
	backupCmd.Flags().StringVar(&backupPassword, "password", "", "encrypt the backup with this password")
	rootCmd.AddCommand(backupCmd)
}
```

- [ ] **Step 2: Build to verify compilation**

```
go build ./cmd/openbee/...
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add cmd/openbee/backup.go
git commit -m "feat(cmd): add openbee backup command"
```

---

## Task 7: `openbee restore` cobra command

**Files:**
- Create: `cmd/openbee/restore.go`

- [ ] **Step 1: Implement `cmd/openbee/restore.go`**

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/theopenbee/openbee/internal/backup"
	"github.com/theopenbee/openbee/internal/config"
)

var restorePassword string
var restoreForce bool

var restoreCmd = &cobra.Command{
	Use:   "restore <backup-file>",
	Short: "Restore openbee data from a backup archive",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		archivePath := args[0]

		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		// Stop daemon if running before overwriting data.
		if err := doStop(daemonPIDFile()); err != nil {
			return fmt.Errorf("stop daemon before restore: %w", err)
		}

		if err := backup.Restore(backup.RestoreOptions{
			ArchivePath: archivePath,
			DBPath:      cfg.Database.Path,
			ConfigPath:  cfgPath,
			StateDir:    openbeeStateDir(),
			AppVersion:  version,
			Force:       restoreForce,
			Password:    restorePassword,
		}); err != nil {
			return err
		}

		fmt.Println("Restore complete.")
		return nil
	},
}

func init() {
	restoreCmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "path to config file")
	restoreCmd.Flags().StringVar(&restorePassword, "password", "", "password to decrypt the backup archive")
	restoreCmd.Flags().BoolVar(&restoreForce, "force", false, "overwrite existing data")
	rootCmd.AddCommand(restoreCmd)
}
```

- [ ] **Step 2: Build to verify compilation**

```
go build ./cmd/openbee/...
```

Expected: no errors

- [ ] **Step 3: Run full test suite**

```
go test ./...
```

Expected: all tests PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/openbee/restore.go
git commit -m "feat(cmd): add openbee restore command"
```

---

## Task 8: Manual smoke test

- [ ] **Step 1: Build the binary**

```
go build -o openbee-dev ./cmd/openbee
```

- [ ] **Step 2: Create a backup**

Assuming `config.yaml` and `data/openbee.db` exist from an existing dev setup:

```
./openbee-dev backup /tmp/test-backup
```

Expected output: `Backup created: /tmp/test-backup/openbee-backup-YYYYMMDD-HHMMSS.tar.gz`

- [ ] **Step 3: Inspect the archive**

```
tar -tzf /tmp/test-backup/openbee-backup-*.tar.gz
```

Expected: `manifest.json`, `openbee.db`, `config.yaml`, `dot-openbee/...` listed

- [ ] **Step 4: Test encrypted backup**

```
./openbee-dev backup /tmp/test-backup-enc --password hunter2
```

Expected: file ending in `.tar.gz.enc` created

- [ ] **Step 5: Commit any final cleanups, if needed**

```bash
git add -p
git commit -m "chore: backup/restore smoke test cleanups"
```
