# Project Rename: robobee/core -> theopenbee/openbee

## Overview

Rename the project from `github.com/theopenbee/openbee` to `github.com/theopenbee/openbee`, including all code references, binary names, user data directories, and external service URLs.

## Rename Mapping

| Category | Old | New |
|----------|-----|-----|
| Go module | `github.com/theopenbee/openbee` | `github.com/theopenbee/openbee` |
| Binary | `robobee` | `openbee` |
| Command dir | `cmd/robobee/` | `cmd/openbee/` |
| CLI Use field | `"robobee"` | `"openbee"` |
| User data dir | `~/.robobee/` | `~/.openbee/` |
| Database file | `robobee.db` | `openbee.db` |
| GitHub org | `robobeedev` | `theopenbee` |
| Project name | `robobee` | `openbee` |
| Claude download URL | `cc-download.robobee.dev` | New domain TBD |
| CLAUDE.md ref | `@.robobee.md` | `@.openbee.md` |
| Brand name | `RoboBee` | `OpenBee` |

## Execution Order

1. Rename `go.mod` module declaration
2. Rename directories: `cmd/robobee/` -> `cmd/openbee/`, `.robobee.md` -> `.openbee.md` (if exists)
3. Global text replacements (longest strings first to avoid partial matches):
   - `github.com/theopenbee/openbee` -> `github.com/theopenbee/openbee`
   - `robobeedev` -> `theopenbee`
   - `cc-download.robobee.dev` -> new domain
   - `~/.robobee` -> `~/.openbee`
   - `robobee.db` -> `openbee.db`
   - `robobee` -> `openbee` (remaining references in binary names, scripts, configs)
   - `RoboBee` -> `OpenBee` (brand name in comments/docs)
4. Delete `go.sum`, run `go mod tidy`
5. Verify: `go build ./...` and `go test ./...`

## Scope

- ~72 files affected
- ~51 Go source files with import updates
- 12+ documentation files
- Build configs: Makefile, .goreleaser.yml
- Install script: install.sh
- Config files: config.example.yaml, config.yaml.tmpl

## Notes

- No backward compatibility — clean cut
- Replacement order matters: long strings before short to prevent partial matches
- External services (CDN, Homebrew tap, Scoop bucket) must be set up separately
