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

# Function to publish a package
publish_package() {
  local pkg_name=$1
  local pkg_dir="$PACKAGES_DIR/$pkg_name"

  echo "Publishing @theopenbee/$pkg_name..."

  local output
  output=$(npm publish "$pkg_dir" --access public 2>&1) && {
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
