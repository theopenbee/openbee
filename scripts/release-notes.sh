#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: release-notes.sh <tag> <changelog>" >&2
  exit 2
fi

tag="$1"
changelog="$2"
version="${tag#v}"
base="https://github.com/theopenbee/openbee/releases/download/$tag"
desktop="$base/openbee-desktop-$version"
client="$base/openbee-desktop-client-$version"
server="$base/openbee-$version"
package="$base/openbee_${version}"

if ! grep -qF "## [$version]" "$changelog"; then
  echo "$changelog has no \"## [$version]\" section" >&2
  exit 1
fi

entries() {
  awk -v head="## [$version]" -v kind="$1" '
    /^## / { inside = index($0, head) == 1; next }
    inside && /^### / { current = substr($0, 5); next }
    inside && current == kind && NF { print }
  ' "$changelog"
}

for section in "Fixed:🐞 Bug fixes" "Added:✨ New features" "Changed:🚀 Improvements" \
  "Removed:🗑️ Removed" "Deprecated:⚠️ Deprecated" "Security:🔒 Security"; do
  items="$(entries "${section%%:*}")"
  if [ -n "$items" ]; then
    printf '<details>\n<summary>%s</summary>\n\n%s\n\n</details>\n\n' "${section#*:}" "$items"
  fi
done

cat <<MARKDOWN
## Downloads

### Windows

**Installer (recommended)**

- [64-bit (most PCs)]($desktop-windows-amd64-setup.exe) | [ARM64 (rare)]($desktop-windows-arm64-setup.exe)

**Portable single file**

- [64-bit]($desktop-windows-amd64.exe) | [ARM64]($desktop-windows-arm64.exe)

### macOS

- [Apple silicon]($desktop-darwin-arm64.dmg) | [Intel]($desktop-darwin-amd64.dmg)

### Linux

**DEB (Debian, Ubuntu): install with \`sudo apt install ./<file>.deb\`**

- [64-bit]($desktop-linux-amd64.deb) | [ARM64]($desktop-linux-arm64.deb)

**Archive: extract and run**

- [64-bit]($desktop-linux-amd64.tar.gz) | [ARM64]($desktop-linux-arm64.tar.gz)

### Client-only app (no built-in server, connects to a remote one)

- Windows: [64-bit]($client-windows-amd64-setup.exe) | [ARM64]($client-windows-arm64-setup.exe)
- macOS: [Apple silicon]($client-darwin-arm64.dmg) | [Intel]($client-darwin-amd64.dmg)
- Linux DEB: [64-bit]($client-linux-amd64.deb) | [ARM64]($client-linux-arm64.deb)

### Server (command line, for servers)

- npm: \`npm install -g @theopenbee/cli\`
- macOS: [Apple silicon]($server-darwin-arm64.tar.gz) | [Intel]($server-darwin-amd64.tar.gz)
- Linux: [64-bit]($server-linux-amd64.tar.gz) | [ARM64]($server-linux-arm64.tar.gz)
- Linux DEB: [64-bit](${package}_linux_amd64.deb) | [ARM64](${package}_linux_arm64.deb)
- Linux RPM: [64-bit](${package}_linux_amd64.rpm) | [ARM64](${package}_linux_arm64.rpm)
- Windows: [64-bit]($server-windows-amd64.zip) | [ARM64]($server-windows-arm64.zip)

Server checksums: [checksums.txt]($base/checksums.txt)
MARKDOWN
