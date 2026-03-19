#!/bin/bash

set -e

# List of sub-packages to publish first
SUBPACKAGES=(
  "cli-linux-x64"
  "cli-linux-arm64"
  "cli-darwin-x64"
  "cli-darwin-arm64"
  "cli-win32-x64"
  "cli-win32-arm64"
)

# Main package to publish last
MAIN_PACKAGE="cli"

PACKAGES_DIR="$(dirname "$0")/../packages"

# Determine npm dist-tag from version (e.g. "0.0.1-test.1" -> "--tag test")
VERSION=$(node -p "require('$PACKAGES_DIR/cli/package.json').version")
NPM_TAG_FLAG=""
if [[ "$VERSION" == *"-"* ]]; then
  PRERELEASE_ID=$(echo "$VERSION" | sed 's/.*-\([a-zA-Z][a-zA-Z0-9]*\).*/\1/')
  NPM_TAG_FLAG="--tag $PRERELEASE_ID"
  echo "Prerelease version detected: $VERSION (tag: $PRERELEASE_ID)"
fi

# Function to publish a package
publish_package() {
  local pkg_name=$1
  local pkg_dir="$PACKAGES_DIR/$pkg_name"

  echo "Publishing @theopenbee/$pkg_name..."

  local output
  # shellcheck disable=SC2086
  output=$(npm publish "$pkg_dir" --access public --provenance $NPM_TAG_FLAG 2>&1) && {
    echo "$output"
    echo "Successfully published @theopenbee/$pkg_name"
    return 0
  }

  echo "$output"
  if echo "$output" | grep -q "You cannot publish over the previously published versions"; then
    echo "Package @theopenbee/$pkg_name already published (idempotent, treating as success)"
  else
    echo "Error publishing @theopenbee/$pkg_name"
    exit 1
  fi
}

# Publish sub-packages first
for pkg in "${SUBPACKAGES[@]}"; do
  publish_package "$pkg"
done

# Publish main package last
publish_package "$MAIN_PACKAGE"

echo "All packages published successfully"
