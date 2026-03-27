package backup_test

import (
	"os"
	"path/filepath"
	"strings"
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
