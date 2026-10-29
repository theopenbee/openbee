#!/usr/bin/env bash
set -euo pipefail

if [ -z "${MACOS_CERTIFICATE:-}" ]; then
  echo "::warning::MACOS_CERTIFICATE is not set, so the macOS package is ad-hoc signed and not notarized"
  exit 0
fi

for name in MACOS_CERTIFICATE_PASSWORD MACOS_SIGNING_IDENTITY APPLE_ID APPLE_TEAM_ID APPLE_APP_PASSWORD; do
  if [ -z "${!name:-}" ]; then
    echo "::error::$name must be set when MACOS_CERTIFICATE is set"
    exit 1
  fi
done

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workdir="${RUNNER_TEMP:-$(mktemp -d)}"
keychain="$workdir/signing.keychain-db"
certificate="$workdir/certificate.p12"
password="$(openssl rand -base64 24)"

security create-keychain -p "$password" "$keychain"
security set-keychain-settings -lut 21600 "$keychain"
security unlock-keychain -p "$password" "$keychain"

echo "$MACOS_CERTIFICATE" | base64 --decode > "$certificate"
security import "$certificate" -k "$keychain" -P "$MACOS_CERTIFICATE_PASSWORD" -T /usr/bin/codesign
rm -f "$certificate"
security set-key-partition-list -S apple-tool:,apple: -s -k "$password" "$keychain"
security list-keychains -d user -s "$keychain" login.keychain

xcrun notarytool store-credentials notary --keychain "$keychain" \
  --apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" --password "$APPLE_APP_PASSWORD"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
jq --arg identity "$MACOS_SIGNING_IDENTITY" --arg keychain "$keychain" \
  '.macos.signingIdentity = $identity | .macos.notarize = {keychainProfile: "notary", keychain: $keychain}' \
  "$root/mygo.json" > "$tmp"
cat "$tmp" > "$root/mygo.json"

echo "macOS signing and notarization configured"
