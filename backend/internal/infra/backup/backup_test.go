package backup_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/theopenbee/openbee/internal/infra/backup"
	_ "modernc.org/sqlite"
)

func writeDB(t *testing.T, path, value string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE probe (v TEXT)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO probe (v) VALUES (?)`, value)
	require.NoError(t, err)
}

func readDB(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer db.Close()
	var v string
	require.NoError(t, db.QueryRow(`SELECT v FROM probe`).Scan(&v))
	return v
}

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

	writeDB(t, dbPath, "fake-db")
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

	writeDB(t, dbPath, "fake-db")
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

func TestRestoreRoundTrip(t *testing.T) {
	// --- Setup source data ---
	srcDB := filepath.Join(t.TempDir(), "openbee.db")
	srcCfg := filepath.Join(t.TempDir(), "config.yaml")
	srcState := filepath.Join(t.TempDir(), "dot-openbee")

	writeDB(t, srcDB, "fake-db-content")
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

	require.Equal(t, "fake-db-content", readDB(t, dstDB))

	gotCfg, err := os.ReadFile(dstCfg)
	require.NoError(t, err)
	require.Equal(t, "server:\n  port: 8080\n", string(gotCfg))

	gotLog, err := os.ReadFile(filepath.Join(dstState, "openbee.log"))
	require.NoError(t, err)
	require.Equal(t, "log-content", string(gotLog))
}

func TestRestoreBlockedWithoutForce(t *testing.T) {
	srcDB := filepath.Join(t.TempDir(), "openbee.db")
	srcCfg := filepath.Join(t.TempDir(), "config.yaml")
	srcState := filepath.Join(t.TempDir(), "dot-openbee")

	writeDB(t, srcDB, "db")
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

	writeDB(t, srcDB, "db-enc")
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

	require.Equal(t, "db-enc", readDB(t, dstDB))
}

func TestBackupIncludesWritesStillInWAL(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "openbee.db")
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	stateDir := filepath.Join(t.TempDir(), "dot-openbee")
	require.NoError(t, os.WriteFile(cfgPath, []byte("cfg"), 0644))
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	live, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)")
	require.NoError(t, err)
	defer live.Close()
	live.SetMaxOpenConns(1)
	_, err = live.Exec(`CREATE TABLE probe (v TEXT)`)
	require.NoError(t, err)
	_, err = live.Exec(`INSERT INTO probe (v) VALUES ('in-wal')`)
	require.NoError(t, err)
	require.FileExists(t, dbPath+"-wal")

	archivePath, err := backup.Backup(backup.BackupOptions{
		DBPath:     dbPath,
		ConfigPath: cfgPath,
		StateDir:   stateDir,
		OutputDir:  t.TempDir(),
		AppVersion: "0.5.0",
	})
	require.NoError(t, err)

	extractDir := t.TempDir()
	require.NoError(t, backup.UnpackTarGz(archivePath, extractDir))
	require.Equal(t, "in-wal", readDB(t, filepath.Join(extractDir, "openbee.db")))
}

func TestBackupMissingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "openbee.db")
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("cfg"), 0644))

	_, err := backup.Backup(backup.BackupOptions{
		DBPath:     dbPath,
		ConfigPath: cfgPath,
		StateDir:   t.TempDir(),
		OutputDir:  t.TempDir(),
		AppVersion: "0.5.0",
	})
	require.Error(t, err)
	require.NoFileExists(t, dbPath)
}

func TestRestoreRemovesStaleSidecarFiles(t *testing.T) {
	srcDB := filepath.Join(t.TempDir(), "openbee.db")
	srcCfg := filepath.Join(t.TempDir(), "config.yaml")
	srcState := filepath.Join(t.TempDir(), "dot-openbee")
	writeDB(t, srcDB, "restored")
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

	dstDB := filepath.Join(t.TempDir(), "openbee.db")
	writeDB(t, dstDB, "old")
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		require.NoError(t, os.WriteFile(dstDB+suffix, []byte("stale"), 0644))
	}

	err = backup.Restore(backup.RestoreOptions{
		ArchivePath: archivePath,
		DBPath:      dstDB,
		ConfigPath:  filepath.Join(t.TempDir(), "config.yaml"),
		StateDir:    filepath.Join(t.TempDir(), "dot-openbee"),
		AppVersion:  "0.5.0",
		Force:       true,
	})
	require.NoError(t, err)
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		require.NoFileExists(t, dstDB+suffix)
	}
	require.Equal(t, "restored", readDB(t, dstDB))
}

func openWAL(t *testing.T, path, value string) *sql.DB {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS probe (v TEXT)`)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM probe`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO probe (v) VALUES (?)`, value)
	require.NoError(t, err)
	return db
}

func TestBackupSkipsDatabaseInsideStateDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "dot-openbee")
	dbPath := filepath.Join(stateDir, "data", "openbee.db")
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("cfg"), 0644))
	openWAL(t, dbPath, "live")
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "openbee.log"), []byte("log"), 0644))

	archivePath, err := backup.Backup(backup.BackupOptions{
		DBPath:     dbPath,
		ConfigPath: cfgPath,
		StateDir:   stateDir,
		OutputDir:  t.TempDir(),
		AppVersion: "0.5.0",
	})
	require.NoError(t, err)

	extractDir := t.TempDir()
	require.NoError(t, backup.UnpackTarGz(archivePath, extractDir))
	require.Equal(t, "live", readDB(t, filepath.Join(extractDir, "openbee.db")))
	require.FileExists(t, filepath.Join(extractDir, "dot-openbee", "openbee.log"))
	for _, suffix := range []string{"", "-wal", "-shm"} {
		require.NoFileExists(t, filepath.Join(extractDir, "dot-openbee", "data", "openbee.db"+suffix))
	}
}

func TestRestoreSkipsDatabaseInsideStateDir(t *testing.T) {
	srcState := filepath.Join(t.TempDir(), "dot-openbee")
	require.NoError(t, os.MkdirAll(filepath.Join(srcState, "data"), 0755))
	writeDB(t, filepath.Join(srcState, "data", "openbee.db"), "stale copy")
	srcDB := filepath.Join(t.TempDir(), "openbee.db")
	writeDB(t, srcDB, "snapshot")
	srcCfg := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(srcCfg, []byte("cfg"), 0644))
	archivePath, err := backup.Backup(backup.BackupOptions{
		DBPath:     srcDB,
		ConfigPath: srcCfg,
		StateDir:   srcState,
		OutputDir:  t.TempDir(),
		AppVersion: "0.5.0",
	})
	require.NoError(t, err)

	dstState := filepath.Join(t.TempDir(), "dot-openbee")
	dstDB := filepath.Join(dstState, "data", "openbee.db")
	err = backup.Restore(backup.RestoreOptions{
		ArchivePath: archivePath,
		DBPath:      dstDB,
		ConfigPath:  filepath.Join(t.TempDir(), "config.yaml"),
		StateDir:    dstState,
		AppVersion:  "0.5.0",
	})
	require.NoError(t, err)
	require.Equal(t, "snapshot", readDB(t, dstDB))
}

func TestRestoreRefusesDatabaseInUse(t *testing.T) {
	srcDB := filepath.Join(t.TempDir(), "openbee.db")
	writeDB(t, srcDB, "restored")
	srcCfg := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(srcCfg, []byte("cfg"), 0644))
	archivePath, err := backup.Backup(backup.BackupOptions{
		DBPath:     srcDB,
		ConfigPath: srcCfg,
		StateDir:   t.TempDir(),
		OutputDir:  t.TempDir(),
		AppVersion: "0.5.0",
	})
	require.NoError(t, err)

	dstDB := filepath.Join(t.TempDir(), "openbee.db")
	live := openWAL(t, dstDB, "running")
	opts := backup.RestoreOptions{
		ArchivePath: archivePath,
		DBPath:      dstDB,
		ConfigPath:  filepath.Join(t.TempDir(), "config.yaml"),
		StateDir:    filepath.Join(t.TempDir(), "dot-openbee"),
		AppVersion:  "0.5.0",
		Force:       true,
	}
	err = backup.Restore(opts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "in use")
	var v string
	require.NoError(t, live.QueryRow(`SELECT v FROM probe`).Scan(&v))
	require.Equal(t, "running", v)

	require.NoError(t, live.Close())
	require.NoError(t, backup.Restore(opts))
	require.Equal(t, "restored", readDB(t, dstDB))
}
