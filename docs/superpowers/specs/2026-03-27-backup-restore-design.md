# Backup and Restore Commands Design

**Date:** 2026-03-27
**Status:** Approved

## Overview

Add `openbee backup` and `openbee restore` commands to support both server migration and disaster recovery scenarios. Backups are stored as a single compressed archive containing the database, configuration, and state directory. Encryption is supported via a `--password` flag.

## Use Cases

- **Server migration:** export all data from one machine and import on another
- **Disaster recovery:** periodically create backups to restore from in case of data loss

## Command Interface

```
# Create a backup (output path optional, defaults to current directory)
openbee backup [output-path] [--password <pwd>]

# Restore from a backup
openbee restore <file> [--force] [--password <pwd>]
```

Output filenames:
- Unencrypted: `openbee-backup-20260327-153000.tar.gz`
- Encrypted: `openbee-backup-20260327-153000.tar.gz.enc`

## Backup Scope

| Item | Source | Notes |
|------|--------|-------|
| `openbee.db` | configured `database.path` | hot-backed via SQLite backup API |
| `config.yaml` | config file path used at startup | |
| `dot-openbee/` | `~/.openbee/` | daemon state directory |

## Archive Structure

```
openbee-backup-20260327-153000.tar.gz
├── manifest.json
├── openbee.db
├── config.yaml
└── dot-openbee/
    ├── openbee.log
    └── ...
```

`manifest.json` schema:
```json
{
  "version": "1",
  "openbee_version": "0.5.0",
  "created_at": "2026-03-27T15:30:00Z",
  "files": [
    {"path": "openbee.db", "sha256": "abc123..."},
    {"path": "config.yaml", "sha256": "def456..."},
    {"path": "dot-openbee/openbee.log", "sha256": "ghi789..."}
  ]
}
```

## Encryption

When `--password` is provided:
- The `.tar.gz` is encrypted with **AES-256-GCM**
- The key is derived from the password using **scrypt** (N=32768, r=8, p=1)
- A random 32-byte salt is prepended to the output file
- Output extension: `.tar.gz.enc`

## Backup Flow

1. Create a temporary working directory
2. Hot-backup the SQLite database using the SQLite backup API (safe while service is running)
3. Copy `config.yaml` and `~/.openbee/` into the temp directory
4. Write `manifest.json` with SHA256 checksums for all included files
5. Create a `.tar.gz` archive from the temp directory
6. If `--password` provided: encrypt with AES-256-GCM, output `.tar.gz.enc`
7. Move the archive to the output path; clean up temp directory

## Restore Flow

1. Detect file type by extension; if `.enc`, decrypt using `--password`
2. Read `manifest.json` and check openbee version compatibility — warn if mismatched, do not block
3. If target locations already contain data and `--force` is not set, exit with error
4. If the openbee service is running, stop it (`openbee stop`) before proceeding
5. Restore files to their original locations:
   - `openbee.db` → configured `database.path`
   - `config.yaml` → config file path
   - `dot-openbee/` → `~/.openbee/`
6. Print success confirmation

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Backup fails mid-way | Clean up temp files; no partial archive left |
| Wrong password on decrypt | Exit with "incorrect password or corrupted file" |
| Version mismatch in manifest | Print warning; continue restore |
| Service fails to stop before restore | Exit with error; do not overwrite data |
| Target has existing data, no `--force` | Exit with error; prompt user to add `--force` |

## Code Structure

```
internal/backup/
├── backup.go    # Backup() — creates the archive
├── restore.go   # Restore() — extracts and restores
└── manifest.go  # manifest read/write, checksum helpers

cmd/openbee/
├── backup.go    # cobra `backup` command
└── restore.go   # cobra `restore` command
```

## Testing

Unit tests in `internal/backup/`:
- Full backup → restore round-trip (unencrypted)
- Full backup → restore round-trip (encrypted with `--password`)
- Manifest write and read
- SHA256 checksum verification
- Restore blocked without `--force` when data exists
- Restore with wrong password returns correct error

Cobra command layer is not unit-tested; covered by the internal package tests.
