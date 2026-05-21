"use strict";

const os = require("os");
const { supportedPlatforms } = require("./package.json");

/**
 * Map Node.js os.type() and os.arch() to a key in supportedPlatforms.
 */
function getPlatformKey() {
  const rawOs = os.type();
  const rawArch = os.arch();

  let osType;
  switch (rawOs) {
    case "Darwin":
      osType = "darwin";
      break;
    case "Linux":
      osType = "linux";
      break;
    default:
      throw new Error(
        `Unsupported operating system: ${rawOs}. ` +
        `slk currently ships binaries for darwin and linux only.`
      );
  }

  let arch;
  switch (rawArch) {
    case "x64":
      arch = "x64";
      break;
    case "arm64":
      arch = "arm64";
      break;
    default:
      throw new Error(
        `Unsupported architecture: ${rawArch}. ` +
        `slk currently ships binaries for arm64 and x64 only.`
      );
  }

  const key = `${osType}-${arch}`;
  if (!supportedPlatforms[key]) {
    throw new Error(
      `Unsupported platform: ${key}. ` +
      `Supported: ${Object.keys(supportedPlatforms).join(", ")}`
    );
  }
  return key;
}

function getPlatform() {
  return supportedPlatforms[getPlatformKey()];
}

module.exports = { getPlatform, getPlatformKey };
