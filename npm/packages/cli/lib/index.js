'use strict';

const path = require('path');

const PLATFORM_MAP = {
  'linux-x64':    '@theopenbee/cli-linux-x64',
  'linux-arm64':  '@theopenbee/cli-linux-arm64',
  'darwin-x64':   '@theopenbee/cli-darwin-x64',
  'darwin-arm64': '@theopenbee/cli-darwin-arm64',
  'win32-x64':    '@theopenbee/cli-win32-x64',
  'win32-arm64':  '@theopenbee/cli-win32-arm64',
};

const BINARY_NAME = {
  'win32-x64':   'openbee.exe',
  'win32-arm64': 'openbee.exe',
};

function getBinaryPath() {
  const key = `${process.platform}-${process.arch}`;
  const pkgName = PLATFORM_MAP[key];

  if (!pkgName) {
    throw new Error(`openbee: unsupported platform ${process.platform}/${process.arch}`);
  }

  let pkgDir;
  try {
    pkgDir = path.dirname(require.resolve(`${pkgName}/package.json`));
  } catch (e) {
    throw new Error(
      `openbee: optional dependency ${pkgName} is not installed. Try: npm install ${pkgName}`
    );
  }

  const binaryName = BINARY_NAME[key] || 'openbee';
  return path.join(pkgDir, 'bin', binaryName);
}

module.exports = { getBinaryPath };
