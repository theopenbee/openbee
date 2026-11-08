#!/usr/bin/env bash
set -euo pipefail

case "${1:-}" in
  bundled) prefix="v" ;;
  client) prefix="desktop-client-v" ;;
  *)
    echo "usage: enable-updates.sh <bundled|client>" >&2
    exit 2
    ;;
esac

config="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/mygo.json"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

jq --arg prefix "$prefix" '.updates = {
  publicKey: "IBlc151rw9wKka6pSv26biYT+1HIXVtsFAD1SkCPl/A=",
  github: "theopenbee/openbee",
  tagPrefix: $prefix,
  changelog: "../CHANGELOG.md",
  deltas: 0
}' "$config" > "$tmp"
cat "$tmp" > "$config"
