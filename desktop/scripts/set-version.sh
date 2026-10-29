#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
  echo "usage: set-version.sh <version>" >&2
  exit 2
fi

version="${1#v}"
case "$version" in
  [0-9]*) ;;
  *) version="0.0.0-$version" ;;
esac

config="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/mygo.json"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

jq --arg version "$version" '.version = $version' "$config" > "$tmp"
cat "$tmp" > "$config"

echo "$version"
