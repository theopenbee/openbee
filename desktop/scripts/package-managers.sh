#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 4 ]; then
  echo "usage: package-managers.sh <tag> <version> <assets dir> <output dir>" >&2
  exit 2
fi

tag="$1"
version="$2"
assets="$3"
out="$4"
base="https://github.com/theopenbee/openbee/releases/download/$tag"
macintel="openbee-desktop-$version-darwin-amd64.zip"
macarm="openbee-desktop-$version-darwin-arm64.zip"
win64="openbee-desktop-$version-windows-amd64.exe"
winarm="openbee-desktop-$version-windows-arm64.exe"

sha() {
  if [ ! -f "$assets/$1" ]; then
    echo "missing $assets/$1" >&2
    exit 1
  fi
  if command -v sha256sum > /dev/null; then
    sha256sum "$assets/$1" | cut -d' ' -f1
  else
    shasum -a 256 "$assets/$1" | cut -d' ' -f1
  fi
}

macintel_sha="$(sha "$macintel")"
macarm_sha="$(sha "$macarm")"
win64_sha="$(sha "$win64")"
winarm_sha="$(sha "$winarm")"
mkdir -p "$out"

caveats=""
if [ "${NOTARIZED:-false}" != "true" ]; then
  caveats='

  caveats <<~EOS
    OpenBee is not notarized yet. If macOS refuses to open it, run:
      xattr -dr com.apple.quarantine "#{appdir}/OpenBee.app"
  EOS'
fi

cat > "$out/openbee-desktop.rb" <<RUBY
cask "openbee-desktop" do
  arch arm: "arm64", intel: "amd64"

  version "$version"
  sha256 arm:   "$macarm_sha",
         intel: "$macintel_sha"

  url "$base/openbee-desktop-$version-darwin-#{arch}.zip"
  name "OpenBee"
  desc "Desktop app with the OpenBee server built in"
  homepage "https://github.com/theopenbee/openbee"

  livecheck do
    skip "Auto-generated on release."
  end

  depends_on :macos

  app "OpenBee.app"

  zap trash: [
    "~/Library/Application Support/OpenBee",
    "~/Library/Caches/OpenBee",
    "~/Library/Logs/OpenBee",
  ]$caveats
end
RUBY

jq -n \
  --arg version "$version" \
  --arg url64 "$base/$win64#/OpenBee.exe" \
  --arg hash64 "$win64_sha" \
  --arg urlarm "$base/$winarm#/OpenBee.exe" \
  --arg hasharm "$winarm_sha" \
  '{
    version: $version,
    description: "Desktop app with the OpenBee server built in",
    homepage: "https://github.com/theopenbee/openbee",
    license: "AGPL-3.0-only",
    architecture: {
      "64bit": {url: $url64, hash: $hash64},
      arm64: {url: $urlarm, hash: $hasharm}
    },
    shortcuts: [["OpenBee.exe", "OpenBee"]]
  }' > "$out/openbee-desktop.json"

echo "wrote $out/openbee-desktop.rb and $out/openbee-desktop.json"
