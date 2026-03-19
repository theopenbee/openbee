# NPM Distribution Design for openbee

**Date:** 2026-03-18
**Status:** Approved
**Author:** 毛毛 (openbee dev worker)

---

## Overview

Distribute the `openbee` Go CLI binary via NPM using the `optionalDependencies` multi-platform package pattern. Users can install globally (`npm install -g @openbee/cli`) or run without installing (`npx @openbee/cli`).

---

## Requirements

| Requirement | Detail |
|---|---|
| Usage modes | `npm install -g @openbee/cli` + `npx @openbee/cli` |
| Binary distribution | Bundled directly in NPM packages (no download at install time) |
| Package name | `@openbee/cli` (scoped) |
| Supported platforms | linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64 |

---

## Chosen Approach: optionalDependencies Multi-Platform Packages

This is the industry-standard pattern used by esbuild, prettier, turbo, and other tools that ship native binaries via NPM. The main package declares platform-specific packages as `optionalDependencies`; npm/pnpm/yarn install only the one matching the current platform.

---

## Section 1: Package Structure

### NPM Package Inventory (7 packages total)

| Package | Contents | Approx. Size |
|---|---|---|
| `@openbee/cli` | JS wrapper + bin entrypoint | ~5 KB |
| `@openbee/cli-linux-x64` | `openbee-linux-amd64` binary | ~20–50 MB |
| `@openbee/cli-linux-arm64` | `openbee-linux-arm64` binary | ~20–50 MB |
| `@openbee/cli-darwin-x64` | `openbee-darwin-amd64` binary | ~20–50 MB |
| `@openbee/cli-darwin-arm64` | `openbee-darwin-arm64` binary | ~20–50 MB |
| `@openbee/cli-win32-x64` | `openbee-windows-amd64.exe` binary | ~20–50 MB |
| `@openbee/cli-win32-arm64` | `openbee-windows-arm64.exe` binary | ~20–50 MB |

### Repository Directory Layout

New `npm/` directory added to the openbee repository root:

```
npm/
├── packages/
│   ├── cli/                          # @openbee/cli (main package)
│   │   ├── package.json
│   │   ├── bin/
│   │   │   └── openbee               # Node.js wrapper script (executable)
│   │   └── lib/
│   │       └── index.js              # Platform detection + binary execution logic
│   ├── cli-linux-x64/                # @openbee/cli-linux-x64
│   │   ├── package.json
│   │   └── bin/
│   │       └── openbee               # Binary (copied during build)
│   ├── cli-linux-arm64/
│   ├── cli-darwin-x64/
│   ├── cli-darwin-arm64/
│   ├── cli-win32-x64/
│   └── cli-win32-arm64/
└── scripts/
    ├── set-version.js                # Updates version in all package.json files
    └── publish.sh                    # Publishes all packages in correct order
```

### Main Package `package.json`

```json
{
  "name": "@openbee/cli",
  "version": "0.0.0",
  "description": "openbee CLI — distributed via NPM",
  "bin": { "openbee": "./bin/openbee" },
  "files": ["bin/", "lib/"],
  "optionalDependencies": {
    "@openbee/cli-linux-x64":    "0.0.0",
    "@openbee/cli-linux-arm64":  "0.0.0",
    "@openbee/cli-darwin-x64":   "0.0.0",
    "@openbee/cli-darwin-arm64": "0.0.0",
    "@openbee/cli-win32-x64":    "0.0.0",
    "@openbee/cli-win32-arm64":  "0.0.0"
  },
  "engines": { "node": ">=14" }
}
```

### Platform Sub-Package `package.json` (example: linux-x64)

```json
{
  "name": "@openbee/cli-linux-x64",
  "version": "0.0.0",
  "os": ["linux"],
  "cpu": ["x64"],
  "files": ["bin/"],
  "bin": { "openbee": "./bin/openbee" }
}
```

The `os` and `cpu` fields instruct npm to skip installation on non-matching platforms.

---

## Section 2: JS Wrapper and Runtime Logic

### Platform Detection and Binary Execution

`npm/packages/cli/bin/openbee` (Node.js shebang script):

```
1. Detect current platform: process.platform (linux/darwin/win32) + process.arch (x64/arm64)
2. Map to sub-package name: e.g. darwin + arm64 → @openbee/cli-darwin-arm64
3. Resolve absolute path to binary inside the sub-package via require.resolve()
4. Execute binary with child_process.spawnSync(), forwarding all argv and stdio
5. process.exit() with the binary's exit code
```

### Platform Mapping Table

| Node `platform/arch` | NPM sub-package | Binary filename |
|---|---|---|
| `linux/x64` | `@openbee/cli-linux-x64` | `openbee` |
| `linux/arm64` | `@openbee/cli-linux-arm64` | `openbee` |
| `darwin/x64` | `@openbee/cli-darwin-x64` | `openbee` |
| `darwin/arm64` | `@openbee/cli-darwin-arm64` | `openbee` |
| `win32/x64` | `@openbee/cli-win32-x64` | `openbee.exe` |
| `win32/arm64` | `@openbee/cli-win32-arm64` | `openbee.exe` |

### Error Handling

If the platform sub-package is not installed (`require.resolve` throws), print a clear error:

```
openbee: unsupported platform linux/arm64, or optional dependency
@openbee/cli-linux-arm64 is not installed.

Try: npm install @openbee/cli-linux-arm64
```

Exit with code 1.

### `npx` User Flow

```
npx @openbee/cli --help
  │
  ├─ npm downloads @openbee/cli (~5 KB)
  ├─ npm detects platform, downloads matching sub-package only (~30 MB)
  └─ bin/openbee wrapper runs → spawns Go binary → output to user
```

The experience is transparent: identical to running the native binary.

---

## Section 3: Release Workflow and CI/CD

### Version Strategy

NPM package version mirrors the Go project version exactly:
`git tag v1.2.3` → all 7 NPM packages publish as `1.2.3`.

The `set-version.js` script updates all `package.json` files atomically before publishing.

### Makefile Additions

```makefile
npm-prepare: release           ## Copy dist/ binaries into npm package dirs and set version
	cp dist/openbee-linux-amd64        npm/packages/cli-linux-x64/bin/openbee
	cp dist/openbee-linux-arm64        npm/packages/cli-linux-arm64/bin/openbee
	cp dist/openbee-darwin-amd64       npm/packages/cli-darwin-x64/bin/openbee
	cp dist/openbee-darwin-arm64       npm/packages/cli-darwin-arm64/bin/openbee
	cp dist/openbee-windows-amd64.exe  npm/packages/cli-win32-x64/bin/openbee.exe
	cp dist/openbee-windows-arm64.exe  npm/packages/cli-win32-arm64/bin/openbee.exe
	node npm/scripts/set-version.js $(VERSION)

npm-publish: npm-prepare       ## Publish all npm packages to registry
	bash npm/scripts/publish.sh
```

### GitHub Actions Workflow

Trigger: push of a `v*` tag (e.g. `v1.0.0`)

```yaml
# .github/workflows/npm-release.yml
on:
  push:
    tags: ['v*']

jobs:
  go-release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: make release          # build 6-platform binaries → dist/
      - run: make npm-prepare      # copy binaries to npm/packages/*/bin/
      - uses: actions/upload-artifact@v4
        with:
          name: npm-packages
          path: npm/packages/

  npm-publish:
    needs: go-release
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with: { name: npm-packages, path: npm/packages/ }
      - uses: actions/setup-node@v4
        with: { registry-url: 'https://registry.npmjs.org' }
      - run: bash npm/scripts/publish.sh   # publishes sub-packages first, then main
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
```

### Publish Order

The `publish.sh` script publishes sub-packages before the main package to ensure `optionalDependencies` resolve correctly at install time:

```
1. Publish @openbee/cli-linux-x64
2. Publish @openbee/cli-linux-arm64
3. Publish @openbee/cli-darwin-x64
4. Publish @openbee/cli-darwin-arm64
5. Publish @openbee/cli-win32-x64
6. Publish @openbee/cli-win32-arm64
7. Publish @openbee/cli  (main package)
```

### Final User-Facing Commands

```bash
# Global install
npm install -g @openbee/cli
openbee --help

# Run without installing (latest)
npx @openbee/cli --help

# Pin to specific version
npx @openbee/cli@1.2.3 --help
```

---

## Out of Scope

- Windows support in `install.sh` (existing shell script remains unchanged)
- Homebrew tap / other package managers
- Automated NPM token rotation

---

## Files to Create / Modify

| Path | Action |
|---|---|
| `npm/packages/cli/package.json` | Create |
| `npm/packages/cli/bin/openbee` | Create |
| `npm/packages/cli/lib/index.js` | Create |
| `npm/packages/cli-*/package.json` | Create ×6 |
| `npm/scripts/set-version.js` | Create |
| `npm/scripts/publish.sh` | Create |
| `Makefile` | Modify (add `npm-prepare`, `npm-publish` targets) |
| `.github/workflows/npm-release.yml` | Create |
