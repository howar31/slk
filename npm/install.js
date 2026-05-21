#!/usr/bin/env node
"use strict";

const crypto = require("crypto");
const fs = require("fs");
const path = require("path");
const os = require("os");
const { pipeline } = require("stream/promises");
const { createWriteStream, mkdirSync, rmSync } = require("fs");
const { spawnSync } = require("child_process");
const { Readable } = require("stream");
const { getPlatform } = require("./platform");

const INSTALL_DIR = path.join(__dirname, "bin");

function downloadUrl(version, artifact) {
  return `https://github.com/howar31/slk/releases/download/v${version}/${artifact}`;
}

function checksumUrl(version) {
  return `https://github.com/howar31/slk/releases/download/v${version}/checksums.txt`;
}

async function download(url, dest) {
  const res = await fetch(url, { redirect: "follow" });
  if (!res.ok) {
    throw new Error(`Failed to download ${url}: ${res.status} ${res.statusText}`);
  }
  if (!res.body) {
    throw new Error(`Failed to download ${url}: empty body`);
  }
  const file = createWriteStream(dest);
  await pipeline(Readable.fromWeb(res.body), file);
}

function extractTarGz(archivePath, destDir) {
  const r = spawnSync("tar", ["xf", archivePath, "-C", destDir], { stdio: "pipe" });
  if (r.status !== 0) {
    throw new Error(`tar failed: ${(r.stderr || "").toString()}`);
  }
}

function sha256(filePath) {
  const buf = fs.readFileSync(filePath);
  return crypto.createHash("sha256").update(buf).digest("hex").toLowerCase();
}

function lookupChecksum(checksumsFile, artifact) {
  const lines = fs.readFileSync(checksumsFile, "utf8").split(/\r?\n/);
  for (const line of lines) {
    const m = line.trim().match(/^([0-9a-f]+)\s+(\S+)$/i);
    if (m && m[2] === artifact) return m[1].toLowerCase();
  }
  throw new Error(`Checksum for ${artifact} not found in checksums.txt`);
}

async function main() {
  const platform = getPlatform();
  const { version } = require("./package.json");

  const binPath = path.join(INSTALL_DIR, platform.binary);
  const versionFile = path.join(INSTALL_DIR, ".version");
  if (fs.existsSync(binPath) && fs.existsSync(versionFile)) {
    const installed = fs.readFileSync(versionFile, "utf8").trim();
    if (installed === version) {
      console.error(`slk v${version} is already installed, skipping.`);
      return;
    }
    console.error(`Upgrading slk from v${installed} to v${version}`);
  }

  if (fs.existsSync(INSTALL_DIR)) rmSync(INSTALL_DIR, { recursive: true, force: true });
  mkdirSync(INSTALL_DIR, { recursive: true });

  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "slk-"));
  const tarPath = path.join(tmpDir, platform.artifact);
  const sumsPath = path.join(tmpDir, "checksums.txt");

  try {
    const tarUrl = downloadUrl(version, platform.artifact);
    const sumsUrlStr = checksumUrl(version);
    console.error(`Downloading ${tarUrl}`);
    await download(tarUrl, tarPath);
    console.error(`Downloading ${sumsUrlStr}`);
    await download(sumsUrlStr, sumsPath);

    const expected = lookupChecksum(sumsPath, platform.artifact);
    const actual = sha256(tarPath);
    if (expected !== actual) {
      throw new Error(
        `SHA256 mismatch for ${platform.artifact}\n  expected: ${expected}\n  actual:   ${actual}`
      );
    }
    console.error("Checksum verified.");

    console.error(`Extracting to ${INSTALL_DIR}`);
    extractTarGz(tarPath, INSTALL_DIR);
    fs.chmodSync(binPath, 0o755);

    fs.writeFileSync(versionFile, version);
    console.error(`slk v${version} installed.`);
  } finally {
    rmSync(tmpDir, { recursive: true, force: true });
  }
}

main().catch((err) => {
  console.error(`Error installing slk: ${err.message}`);
  process.exit(1);
});
