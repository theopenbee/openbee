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
| `@openbee/cli-linux-x64` | `openbee` binary (linux/amd64) | ~20–50 MB |
| `@openbee/cli-linux-arm64` | `openbee` binary (linux/arm64) | ~20–50 MB |
| `@openbee/cli-darwin-x64` | `openbee` binary (darwin/amd64) | ~20–50 MB |
| `@openbee/cli-darwin-arm64` | `openbee` binary (darwin/arm64) | ~20–50 MB |
| `@openbee/cli-win32-x64` | `openbee.exe` binary (windows/amd64) | ~20–50 MB |
| `@openbee/cli-win32-arm64` | `openbee.exe` binary (windows/arm64) | ~20–50 MB |

### Repository Directory Layout

New `npm/` directory added to the openbee repository root:

```
npm/
├── packages/
│   ├── cli/                          # @openbee/cli (main package)
│   │   ├── package.json
│   │   ├── bin/
│   │   │   └── openbee               # Node.js wrapper script (executable, shebang: #!/usr/bin/env node)
│   │   └── lib/
│   │       └── index.js              # Optional programmatic API — exports getBinaryPath() for tooling that needs the binary path
│   ├── cli-linux-x64/                # @openbee/cli-linux-x64
│   │   ├── package.json
│   │   └── bin/
│   │       └── openbee               # Binary (copied during build, chmod +x applied)
│   ├── cli-linux-arm64/
│   ├── cli-darwin-x64/
│   ├── cli-darwin-arm64/
│   ├── cli-win32-x64/                # @openbee/cli-win32-x64
│   │   ├── package.json
│   │   └── bin/
│   │       └── openbee.exe           # Binary (copied during build)
│   └── cli-win32-arm64/
└── scripts/
    ├── set-version.js                # Updates version in all 7 package.json files
    └── publish.sh                    # Publishes all 7 packages in correct order
```

### Main Package `package.json`

```json
{
  "name": "@openbee/cli",
  "version": "0.0.0",
  "description": "openbee CLI",
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

All `optionalDependencies` versions are set to **exact versions** (no `^` or `~`) by `set-version.js` at release time. This prevents npm from selecting a mismatched binary version.

### Platform Sub-Package `package.json` (example: linux-x64)

```json
{
  "name": "@openbee/cli-linux-x64",
  "version": "0.0.0",
  "description": "openbee linux/x64 binary",
  "os": ["linux"],
  "cpu": ["x64"],
  "files": ["bin/"]
}
```

**Important:** Sub-packages do **not** have a `bin` field. A `bin` field on sub-packages would cause npm to create a second `openbee` symlink pointing directly to the raw binary (bypassing the JS wrapper), causing breakage in some environments.

---

## Section 2: JS Wrapper and Runtime Logic

### `bin/openbee` — Wrapper Script

```javascript
#!/usr/bin/env node
'use strict';

const { spawnSync } = require('child_process');
const path = require('path');

const PLATFORM_MAP = {
  'linux-x64':    '@openbee/cli-linux-x64',
  'linux-arm64':  '@openbee/cli-linux-arm64',
  'darwin-x64':   '@openbee/cli-darwin-x64',
  'darwin-arm64': '@openbee/cli-darwin-arm64',
  'win32-x64':    '@openbee/cli-win32-x64',
  'win32-arm64':  '@openbee/cli-win32-arm64',
};

const BINARY_NAME = {
  'win32-x64':   'openbee.exe',
  'win32-arm64': 'openbee.exe',
};

const key = `${process.platform}-${process.arch}`;
const pkgName = PLATFORM_MAP[key];

if (!pkgName) {
  process.stderr.write(
    `openbee: unsupported platform ${process.platform}/${process.arch}\n`
  );
  process.exit(1);
}

let pkgDir;
try {
  // require.resolve finds the package.json, giving us the package root
  pkgDir = path.dirname(require.resolve(`${pkgName}/package.json`));
} catch (e) {
  process.stderr.write(
    `openbee: optional dependency ${pkgName} is not installed.\n` +
    `Try: npm install ${pkgName}\n`
  );
  process.exit(1);
}

const binaryName = BINARY_NAME[key] || 'openbee';
const binaryPath = path.join(pkgDir, 'bin', binaryName);

const result = spawnSync(binaryPath, process.argv.slice(2), { stdio: 'inherit' });

if (result.error) {
  process.stderr.write(`openbee: failed to launch binary: ${result.error.message}\n`);
  process.exit(1);
}

process.exit(result.status ?? 1);
```

The `require.resolve('<pkg>/package.json')` pattern is the standard approach used by esbuild and others: it locates the installed package's `package.json`, and `path.dirname()` gives the package root, from which we can derive the binary path predictably.

### Platform Mapping Table

| Node `platform/arch` | NPM sub-package | Binary filename |
|---|---|---|
| `linux/x64` | `@openbee/cli-linux-x64` | `openbee` |
| `linux/arm64` | `@openbee/cli-linux-arm64` | `openbee` |
| `darwin/x64` | `@openbee/cli-darwin-x64` | `openbee` |
| `darwin/arm64` | `@openbee/cli-darwin-arm64` | `openbee` |
| `win32/x64` | `@openbee/cli-win32-x64` | `openbee.exe` |
| `win32/arm64` | `@openbee/cli-win32-arm64` | `openbee.exe` |

### `npx` User Flow

```
npx @openbee/cli --help
  │
  ├─ npm downloads @openbee/cli (~5 KB)
  ├─ npm detects platform, downloads matching sub-package only (~30 MB)
  └─ bin/openbee wrapper runs → spawns Go binary → output to user
```

---

## Section 3: Release Workflow and CI/CD

### Version Strategy

NPM package version mirrors the Go project version exactly:
`git tag v1.2.3` → all 7 NPM packages publish as `1.2.3` (strip leading `v`).

### `npm/scripts/set-version.js`

Called as `node npm/scripts/set-version.js <version>` (version may include leading `v`, e.g. `v1.2.3`).

Behavior:
1. Strip leading `v` from the argument → `semver` (e.g. `1.2.3`)
2. Validate it matches `/^\d+\.\d+\.\d+/` — exit 1 with error message if not
3. For each of the 7 `npm/packages/*/package.json` files:
   - Read JSON
   - Set `version` to `semver`
   - If file is the main package, set each value in `optionalDependencies` to the exact `semver` string
   - Write back (2-space indent, trailing newline)
4. Print: `Updated 7 package.json files to version ${semver}`
5. Exit 0

### `npm/scripts/publish.sh`

Behavior:
- Uses `npm publish --access public` for each package (all packages are under the `@openbee` scope which defaults to private)
- Publishes sub-packages first (in any order), then `@openbee/cli` last
- If a package version is already published (npm exits with "You cannot publish over the previously published versions"), treat it as a success (idempotent, safe for reruns)
- Fails fast on any other error
- Expects `NODE_AUTH_TOKEN` env var to be set (used by npm's built-in auth)

### Makefile Additions

GoReleaser archives follow the naming template `openbee-{version}-{os}-{arch}` (from `.goreleaser.yml`). In CI, after downloading and extracting each archive into a same-named subdirectory under `dist/`, the binary for linux/amd64 is at `dist/openbee-{version}-linux-amd64/openbee`. The `npm-prepare` target uses `find` with hyphen-based glob patterns that match this structure.

```makefile
npm-prepare:  ## Copy extracted dist/ binaries into npm package dirs and set version
	@VERSION=$(shell git describe --tags --always --dirty | sed 's/^v//'); \
	mkdir -p \
	    npm/packages/cli-linux-x64/bin \
	    npm/packages/cli-linux-arm64/bin \
	    npm/packages/cli-darwin-x64/bin \
	    npm/packages/cli-darwin-arm64/bin \
	    npm/packages/cli-win32-x64/bin \
	    npm/packages/cli-win32-arm64/bin; \
	cp $$(find dist -path '*/openbee*linux*amd64*' -name 'openbee') \
	    npm/packages/cli-linux-x64/bin/openbee; \
	cp $$(find dist -path '*/openbee*linux*arm64*' -name 'openbee') \
	    npm/packages/cli-linux-arm64/bin/openbee; \
	cp $$(find dist -path '*/openbee*darwin*amd64*' -name 'openbee') \
	    npm/packages/cli-darwin-x64/bin/openbee; \
	cp $$(find dist -path '*/openbee*darwin*arm64*' -name 'openbee') \
	    npm/packages/cli-darwin-arm64/bin/openbee; \
	cp $$(find dist -path '*/openbee*windows*amd64*' -name 'openbee.exe') \
	    npm/packages/cli-win32-x64/bin/openbee.exe; \
	cp $$(find dist -path '*/openbee*windows*arm64*' -name 'openbee.exe') \
	    npm/packages/cli-win32-arm64/bin/openbee.exe; \
	chmod +x \
	    npm/packages/cli-linux-x64/bin/openbee \
	    npm/packages/cli-linux-arm64/bin/openbee \
	    npm/packages/cli-darwin-x64/bin/openbee \
	    npm/packages/cli-darwin-arm64/bin/openbee; \
	node npm/scripts/set-version.js $$VERSION

npm-publish: npm-prepare  ## Publish all npm packages to registry
	bash npm/scripts/publish.sh
```

Note: `npm-prepare` depends on GoReleaser having already run (i.e., `dist/` is populated). It does **not** call `make release` to avoid double-building. In CI, GoReleaser runs first; locally, developers run `make release` before `make npm-prepare`.

### GitHub Actions: Integrate into Existing `release.yml`

The npm publish steps are added as a **new job** inside the existing `.github/workflows/release.yml`, not a separate workflow file. This avoids two workflows racing on the same tag.

```yaml
# Addition to .github/workflows/release.yml

  npm-publish:
    needs: release          # runs after the existing GoReleaser job
    runs-on: ubuntu-latest
    # Job-level permissions override the workflow-level block for this job only.
    # This restricts GITHUB_TOKEN to read-only for npm-publish (npm uses NODE_AUTH_TOKEN, not GITHUB_TOKEN).
    permissions:
      contents: read
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Node.js
        uses: actions/setup-node@v4
        with:
          node-version: 22
          registry-url: 'https://registry.npmjs.org'

      - name: Download release binaries from GitHub Release
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          VERSION="${GITHUB_REF_NAME}"   # e.g. v1.2.3
          mkdir -p dist
          gh release download "${VERSION}" \
            --pattern 'openbee-*-linux-amd64.tar.gz' \
            --pattern 'openbee-*-linux-arm64.tar.gz' \
            --pattern 'openbee-*-darwin-amd64.tar.gz' \
            --pattern 'openbee-*-darwin-arm64.tar.gz' \
            --pattern 'openbee-*-windows-amd64.zip' \
            --pattern 'openbee-*-windows-arm64.zip' \
            --dir dist/

      - name: Extract binaries from archives
        # Extract each archive into a same-named subdirectory to avoid filename
        # collisions between platforms (each archive's root-level binary is named 'openbee').
        # Result: dist/openbee-1.2.3-linux-amd64/openbee, dist/openbee-1.2.3-darwin-arm64/openbee, etc.
        run: |
          cd dist
          for f in *.tar.gz; do
            dir="${f%.tar.gz}"
            mkdir -p "$dir"
            tar -xzf "$f" -C "$dir"
          done
          for f in *.zip; do
            dir="${f%.zip}"
            mkdir -p "$dir"
            unzip -o "$f" -d "$dir"
          done

      - name: Prepare npm packages
        run: make npm-prepare

      - name: Publish to NPM
        run: bash npm/scripts/publish.sh
        env:
          NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
```

**Why download from GitHub Release instead of using GoReleaser's `dist/` directly?**
GoReleaser runs in the `release` job on its own runner. Artifacts are not shared between jobs by default. Downloading from the just-created GitHub Release (which GoReleaser publishes) is the cleanest, officially-supported way to consume GoReleaser outputs in a downstream job without relying on upload-artifact.

---

## Final User-Facing Commands

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

- Changes to `install.sh` (CDN-based shell installer remains unchanged)
- Homebrew/Scoop (handled by GoReleaser)
- Automated NPM token rotation

---

## Files to Create / Modify

| Path | Action |
|---|---|
| `npm/packages/cli/package.json` | Create |
| `npm/packages/cli/bin/openbee` | Create (JS wrapper, chmod +x) |
| `npm/packages/cli/lib/index.js` | Create (optional — exports `getBinaryPath()` for programmatic use) |
| `npm/packages/cli-linux-x64/package.json` | Create |
| `npm/packages/cli-linux-arm64/package.json` | Create |
| `npm/packages/cli-darwin-x64/package.json` | Create |
| `npm/packages/cli-darwin-arm64/package.json` | Create |
| `npm/packages/cli-win32-x64/package.json` | Create |
| `npm/packages/cli-win32-arm64/package.json` | Create |
| `npm/scripts/set-version.js` | Create |
| `npm/scripts/publish.sh` | Create |
| `Makefile` | Modify (add `npm-prepare`, `npm-publish` targets) |
| `.github/workflows/release.yml` | Modify (add `npm-publish` job) |
