#!/usr/bin/env bash
set -euo pipefail

if [ -z "${WINDOWS_CERTIFICATE:-}" ]; then
  echo "::warning::WINDOWS_CERTIFICATE is not set, so the Windows packages are not code signed"
  exit 0
fi

if [ -z "${WINDOWS_CERTIFICATE_PASSWORD:-}" ]; then
  echo "::error::WINDOWS_CERTIFICATE_PASSWORD must be set when WINDOWS_CERTIFICATE is set"
  exit 1
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workdir="${RUNNER_TEMP:-$(mktemp -d)}"
certificate="$workdir/code-signing.pfx"

echo "$WINDOWS_CERTIFICATE" | base64 --decode > "$certificate"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
jq --arg certificate "$certificate" '.windows.certificate = $certificate' "$root/mygo.json" > "$tmp"
cat "$tmp" > "$root/mygo.json"

echo "Windows code signing configured"
