#!/usr/bin/env node
"use strict";

const path = require("path");
const fs = require("fs");
const { spawnSync } = require("child_process");
const { getPlatform } = require("./platform");

const platform = getPlatform();
const binPath = path.join(__dirname, "bin", platform.binary);

if (!fs.existsSync(binPath)) {
  console.error(`slk binary not found at ${binPath}\nRunning install...`);
  const installResult = spawnSync(process.execPath, [path.join(__dirname, "install.js")], {
    cwd: __dirname,
    stdio: "inherit",
  });
  if (installResult.status !== 0) {
    process.exit(installResult.status ?? 1);
  }
}

const result = spawnSync(binPath, process.argv.slice(2), {
  cwd: process.cwd(),
  stdio: "inherit",
});

if (result.error) {
  console.error(`Error running slk: ${result.error.message}`);
  process.exit(1);
}

process.exit(result.status ?? 1);
