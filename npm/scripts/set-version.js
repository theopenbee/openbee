#!/usr/bin/env node

const fs = require('fs');
const path = require('path');

// Get version argument
const arg = process.argv[2];
if (!arg) {
  console.error('Error: version argument required');
  process.exit(1);
}

// Strip leading 'v' from the argument
const semver = arg.startsWith('v') ? arg.slice(1) : arg;

// Validate semver format
if (!/^\d+\.\d+\.\d+/.test(semver)) {
  console.error(`Error: invalid version format "${semver}", expected X.Y.Z`);
  process.exit(1);
}

// List of package directories
const packages = [
  'cli',
  'cli-linux-x64',
  'cli-linux-arm64',
  'cli-darwin-x64',
  'cli-darwin-arm64',
  'cli-win32-x64',
  'cli-win32-arm64'
];

const packagesDir = path.join(__dirname, '..', 'packages');

// Update each package.json
packages.forEach(pkg => {
  const pkgPath = path.join(packagesDir, pkg, 'package.json');

  try {
    const content = fs.readFileSync(pkgPath, 'utf8');
    const packageJson = JSON.parse(content);

    // Set version
    packageJson.version = semver;

    // If this is the main package, update optionalDependencies
    if (pkg === 'cli') {
      if (packageJson.optionalDependencies) {
        Object.keys(packageJson.optionalDependencies).forEach(key => {
          packageJson.optionalDependencies[key] = semver;
        });
      }
    }

    // Write back with 2-space indent and trailing newline
    fs.writeFileSync(pkgPath, JSON.stringify(packageJson, null, 2) + '\n');
  } catch (error) {
    console.error(`Error updating ${pkgPath}: ${error.message}`);
    process.exit(1);
  }
});

console.log(`Updated 7 package.json files to version ${semver}`);
process.exit(0);
