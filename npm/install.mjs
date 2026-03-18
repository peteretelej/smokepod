import { fileURLToPath } from 'node:url';
import path from 'node:path';
import process from 'node:process';

const PACKAGE_ROOT = path.dirname(fileURLToPath(import.meta.url));
const RELEASE_REPOSITORY = 'https://github.com/peteretelej/smokepod/releases/download';

export const CHECKSUM_FILE = 'checksums.txt';

export function getReleasePlatform(platform = process.platform, arch = process.arch) {
  const platformMap = new Map([
    ['linux', 'linux'],
    ['darwin', 'darwin'],
    ['win32', 'windows']
  ]);
  const archMap = new Map([
    ['x64', 'amd64'],
    ['arm64', 'arm64']
  ]);

  const os = platformMap.get(platform);
  if (!os) {
    throw new Error(`unsupported platform: ${platform}`);
  }

  const normalizedArch = archMap.get(arch);
  if (!normalizedArch) {
    throw new Error(`unsupported architecture: ${arch}`);
  }

  return { os, arch: normalizedArch };
}

export function getArtifactName(target) {
  if (!target || !target.os || !target.arch) {
    throw new Error('target must include os and arch');
  }

  if (target.os === 'windows') {
    return `smokepod_${target.os}_${target.arch}.exe`;
  }

  return `smokepod_${target.os}_${target.arch}`;
}

export function getReleaseTag(version) {
  return `v${version}`;
}

export function getReleaseUrls(version, target) {
  const tag = getReleaseTag(version);
  const artifactName = getArtifactName(target);

  return {
    binaryUrl: `${RELEASE_REPOSITORY}/${tag}/${artifactName}`,
    checksumsUrl: `${RELEASE_REPOSITORY}/${tag}/${CHECKSUM_FILE}`
  };
}

export function getVendorBinaryName(platform = process.platform) {
  return platform === 'win32' ? 'smokepod.exe' : 'smokepod';
}

export function getVendorPath(platform = process.platform) {
  return path.join('vendor', getVendorBinaryName(platform));
}

export function formatInstallError({ detectedPlatform, detectedArch, attemptedSource, reason, version }) {
  const installVersion = version || '0.0.0-dev';

  return [
    'smokepod install failed',
    `Platform: ${detectedPlatform || 'unknown'}`,
    `Architecture: ${detectedArch || 'unknown'}`,
    `Tried: ${attemptedSource || 'unknown'}`,
    `Reason: ${reason || 'unknown error'}`,
    'Retry: SMOKEPOD_BINARY=/absolute/path/to/smokepod npm install',
    `Fallback: go install github.com/peteretelej/smokepod/cmd/smokepod@${getReleaseTag(installVersion)}`
  ].join('\n');
}

export async function main(options) {
  void options;
  throw new Error(
    formatInstallError({
      detectedPlatform: process.platform,
      detectedArch: process.arch,
      attemptedSource: path.join(PACKAGE_ROOT, getVendorPath(process.platform)),
      reason: 'installer not implemented yet in source checkout builds',
      version: '0.0.0-dev'
    })
  );
}

if (process.argv[1] && import.meta.url === new URL(process.argv[1], 'file:').href) {
  await main();
}
