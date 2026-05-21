# slk 分發與安裝體驗 — 實作計畫

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 兩個 PR 把 slk 從「只能 `go install`」升級成 5 條安裝路徑（brew / npm / 預編 binary / `go install` / source），由 GHA on tag 全自動釋出。

**Architecture:** PR 1 立 GHA release pipeline 並用 `v0.1.0-rc1` 隔離驗證 binary 出貨；PR 2 加 brew tap publish、npm scoped wrapper（postinstall 抓 binary）、Gemini extension、skill OpenClaw autoinstall hint、README/SPEC/CLAUDE 三份 SSOT 同步。git tag 是 release 的唯一 trigger 與唯一版本來源。

**Tech Stack:** GitHub Actions、goreleaser v2、Homebrew tap、npm（Node 18+ native fetch）、`actions/attest-build-provenance@v3`。

**Spec:** [docs/superpowers/specs/2026-05-20-slk-distribution-design.md](../specs/2026-05-20-slk-distribution-design.md)

**Workflow rules（每個 commit、tag、secret 動作前必須做的事）：**
- 任何 `git commit` 前先呈現 diff summary 等使用者明確 approve
- 任何 `git push` 與 `git tag` push 前等使用者明確 approve
- Out-of-band 動作（建 repo、加 secret、申請 token）由 **使用者** 手動執行；plan 提供精確指令但不代跑

---

## Task 0：Out-of-band 一次性設定（PR 1 之前）

**Files:** （無 repo 內變更）

**Manual checklist for user。Plan 不代跑，但提供確認指令。**

- [ ] **Step 1：建 Homebrew tap repo**

  使用者執行：
  ```bash
  gh repo create howar31/homebrew-tap --public \
    --description "Homebrew tap for howar31's tools"
  ```

  驗證：
  ```bash
  gh api repos/howar31/homebrew-tap --jq .full_name
  # Expected: howar31/homebrew-tap
  ```

- [ ] **Step 2：確認 npmjs.com `@howar31` scope 可用**

  使用者操作：
  1. https://www.npmjs.com/login 登入
  2. Account settings → Organizations / Profile → 確認 username `howar31` 可作為 scope（personal scope 免費自動可用）

  驗證：
  ```bash
  curl -sS https://registry.npmjs.org/-/user/org.couchdb.user:howar31 -o /dev/null -w "%{http_code}\n"
  # Expected: 200（user 存在）
  ```

- [ ] **Step 3：產生 NPM_TOKEN，先暫存（PR 2 才會加進 secrets）**

  使用者操作：
  1. npmjs.com → Account → Access Tokens → Generate New Token → Granular
  2. Permissions：`Read and write` on packages（scope `@howar31`）
  3. Expiration：1 year
  4. 複製 token 字串，**先存** 1Password 或本機 keyring；**不要** 現在加進 GitHub repo，留到 Task 17 一次加

- [ ] **Step 4：產生 HOMEBREW_TAP_TOKEN，先暫存**

  使用者操作：
  1. GitHub → Settings → Developer settings → Personal access tokens → Fine-grained tokens → Generate new
  2. Resource owner：howar31
  3. Repository access：Only select repositories → 選 `howar31/homebrew-tap`
  4. Permissions：Repository permissions → Contents → Read and write
  5. Expiration：1 year
  6. 複製 token 字串，先暫存；同上不要現在加進 secrets

- [ ] **Step 5：用使用者明確 OK 結束 Task 0**

  Agent 等使用者確認 4 個 sub-step 都完成才能進 Task 1。

---

# PR 1：Release pipeline foundation

## Task 1：更新 `.goreleaser.yaml`

**Files:**
- Modify: `.goreleaser.yaml`

- [ ] **Step 1：確認 baseline**

  Run:
  ```bash
  cat /opt/projects/slk/.goreleaser.yaml
  ```
  Expected：含 `version: 2`、`builds`、`archives`、`brews` 區塊。

- [ ] **Step 2：改寫整份 `.goreleaser.yaml`**

  把檔案內容換成：
  ```yaml
  version: 2
  project_name: slk
  builds:
    - main: ./cmd/slk
      binary: slk
      env: [CGO_ENABLED=0]
      goos: [darwin, linux]
      goarch: [amd64, arm64]
      flags:
        - -trimpath
      ldflags:
        - -s -w -X main.version={{.Version}}
  archives:
    - formats: [tar.gz]
      name_template: 'slk_{{ .Os }}_{{ .Arch }}'
  checksum:
    name_template: 'checksums.txt'
    algorithm: sha256
  # brews block intentionally omitted in PR 1; restored in PR 2 after the
  # howar31/homebrew-tap repo is ready and HOMEBREW_TAP_TOKEN secret exists.
  ```

- [ ] **Step 3：本機 snapshot 驗證**

  Run:
  ```bash
  cd /opt/projects/slk
  goreleaser release --snapshot --clean
  ```
  Expected：成功，`dist/` 出現：
  ```
  slk_darwin_amd64.tar.gz
  slk_darwin_arm64.tar.gz
  slk_linux_amd64.tar.gz
  slk_linux_arm64.tar.gz
  checksums.txt
  ```
  確認檔名格式 `slk_<os>_<arch>.tar.gz`，**無版本號**。

  Run:
  ```bash
  ls /opt/projects/slk/dist/*.tar.gz | wc -l
  # Expected: 4
  ```

- [ ] **Step 4：驗證 binary 可執行**

  Run:
  ```bash
  cd /tmp && rm -rf slk-smoke && mkdir slk-smoke && cd slk-smoke
  tar xzf /opt/projects/slk/dist/slk_darwin_arm64.tar.gz
  ./slk --version
  # Expected: 含 0.0.0-SNAPSHOT 之類字串（snapshot mode）
  ```

  清乾淨：
  ```bash
  rm -rf /opt/projects/slk/dist /tmp/slk-smoke
  ```

## Task 2：新增 `.github/workflows/release.yml`

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1：確認 `.github/workflows/` 目錄不存在**

  Run:
  ```bash
  ls /opt/projects/slk/.github/workflows/ 2>&1
  # Expected: No such file or directory
  ```

- [ ] **Step 2：建立 workflow 檔**

  Create `/opt/projects/slk/.github/workflows/release.yml`：
  ```yaml
  name: Release

  on:
    push:
      tags:
        - 'v[0-9]+.[0-9]+.[0-9]+*'

  permissions:
    contents: write
    id-token: write
    attestations: write

  jobs:
    goreleaser:
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v4
          with:
            fetch-depth: 0
        - uses: actions/setup-go@v5
          with:
            go-version: '1.25'
            check-latest: true
        - uses: goreleaser/goreleaser-action@v6
          with:
            distribution: goreleaser
            version: latest
            args: release --clean
          env:
            GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        - uses: actions/attest-build-provenance@v3
          with:
            subject-path: 'dist/*.tar.gz'
  ```

- [ ] **Step 3：靜態 lint workflow yaml**

  Run:
  ```bash
  python3 -c "import yaml,sys; yaml.safe_load(open('/opt/projects/slk/.github/workflows/release.yml'))" && echo OK
  # Expected: OK
  ```

## Task 3：更新 README.md — Installation 段補 binary 路徑

**Files:**
- Modify: `/opt/projects/slk/README.md`（行號參考 `## Installation` 段下「### Pre-built binaries」與「### Homebrew」子段）

- [ ] **Step 1：定位「Pre-built binaries」子段**

  Run:
  ```bash
  grep -n "### Pre-built binaries" /opt/projects/slk/README.md
  # Expected: 54:### Pre-built binaries
  ```

- [ ] **Step 2：用 Edit 工具把 Pre-built binaries 子段換成真內容**

  將：
  ```markdown
  ### Pre-built binaries

  Planned for the v0.1.0 release. Until then, build from source.
  ```
  改為：
  ```markdown
  ### Pre-built binaries

  Download from [GitHub Releases](https://github.com/howar31/slk/releases).
  Replace `<os>` with `darwin` or `linux`, and `<arch>` with `amd64` or `arm64`.

  ```bash
  VER=0.1.0
  curl -sLO https://github.com/howar31/slk/releases/download/v${VER}/slk_<os>_<arch>.tar.gz
  curl -sLO https://github.com/howar31/slk/releases/download/v${VER}/checksums.txt
  shasum -a 256 -c checksums.txt --ignore-missing
  tar xzf slk_<os>_<arch>.tar.gz
  sudo mv slk /usr/local/bin/
  slk --version
  ```
  ```

  PR 1 階段 Homebrew 子段先**不動**（仍說 Planned）。

- [ ] **Step 3：靜態檢查 — 確認 markdown 沒被改壞**

  Run:
  ```bash
  grep -c "^### Pre-built binaries$" /opt/projects/slk/README.md
  # Expected: 1
  grep -c "^### Homebrew$" /opt/projects/slk/README.md
  # Expected: 1
  ```

## Task 4：Commit PR 1 changes

- [ ] **Step 1：檢查所有變更**

  Run:
  ```bash
  cd /opt/projects/slk
  git status
  git diff --stat
  # Expected: 3 files changed
  #   .github/workflows/release.yml | NEW
  #   .goreleaser.yaml              | MODIFIED
  #   README.md                     | MODIFIED
  ```

- [ ] **Step 2：呈現 diff summary 給使用者，等明確 approve**

  Agent **不可** 自動 commit。先做：
  ```bash
  git diff
  ```
  把摘要貼給使用者，問「approve commit?」

- [ ] **Step 3：approve 後 commit（不能用 git add -A，逐個檔加）**

  Run:
  ```bash
  cd /opt/projects/slk
  git add .github/workflows/release.yml .goreleaser.yaml README.md
  git commit -m "$(cat <<'EOF'
  feat(release): add GHA-on-tag release pipeline

  - .github/workflows/release.yml builds darwin/linux × amd64/arm64
    tarballs via goreleaser and produces build provenance attestation
    on tag push (v*.*.*).
  - .goreleaser.yaml: pin artifact name to slk_<os>_<arch>.tar.gz
    (no version in filename; version lives in release URL path), add
    explicit checksum block. brews: intentionally omitted until PR 2.
  - README: Pre-built binaries section now has a concrete install
    recipe instead of "Planned".

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  git status
  # Expected: nothing to commit, working tree clean
  ```

## Task 5：Push branch + open PR 1（等使用者明確 approve）

- [ ] **Step 1：確認 branch**

  Run:
  ```bash
  git -C /opt/projects/slk branch --show-current
  ```
  若是 `main`：需開 feature branch。
  ```bash
  git -C /opt/projects/slk checkout -b release-pipeline-foundation
  ```

- [ ] **Step 2：問使用者「approve push?」**

  得到明確 OK 才推。

- [ ] **Step 3：Push 並開 PR**

  Run:
  ```bash
  cd /opt/projects/slk
  git push -u origin release-pipeline-foundation
  gh pr create --title "feat(release): GHA-on-tag release pipeline (PR 1/2)" \
    --body "$(cat <<'EOF'
  ## Summary
  - Add `.github/workflows/release.yml`: goreleaser-driven, tag-triggered (`v*.*.*`).
  - Tighten `.goreleaser.yaml`: stable artifact names, explicit checksum block. `brews:` intentionally deferred to PR 2.
  - README: replace "Pre-built binaries: Planned" with a concrete install recipe.

  Part of the distribution refresh; see [spec](docs/superpowers/specs/2026-05-20-slk-distribution-design.md).

  ## Test plan
  - [x] Local `goreleaser release --snapshot --clean` produces 4 tarballs + checksums.txt
  - [ ] Merge → tag `v0.1.0-rc1` → push → GHA green
  - [ ] `curl + tar xzf + ./slk --version` from the published Release works

  🤖 Generated with [Claude Code](https://claude.com/claude-code)
  EOF
  )"
  ```

## Task 6：合併 PR 1 並 tag `v0.1.0-rc1` 驗證 pipeline（**使用者親自操作 + Agent 監看**）

- [ ] **Step 1：使用者合 PR 1**

  使用者操作：在 GitHub UI 上 review + merge PR 1。

- [ ] **Step 2：拉最新 main，tag rc1（等使用者 approve push tag）**

  Run（agent 先準備，等使用者 OK）：
  ```bash
  cd /opt/projects/slk
  git checkout main
  git pull --ff-only origin main
  ```

  問使用者：「approve `git tag v0.1.0-rc1` 與 push tag 到 origin？」

- [ ] **Step 3：建 tag 並 push**

  Run:
  ```bash
  cd /opt/projects/slk
  git tag -a v0.1.0-rc1 -m "Release candidate 1 for v0.1.0"
  git push origin v0.1.0-rc1
  ```

- [ ] **Step 4：監看 GHA**

  Run:
  ```bash
  sleep 5
  gh run list --workflow=release.yml --limit 1
  ```
  記下 run ID。串流 log：
  ```bash
  gh run watch <run-id> --exit-status
  # Expected: 結束時 exit 0
  ```

- [ ] **Step 5：確認 Release 上線**

  Run:
  ```bash
  gh release view v0.1.0-rc1 --json assets --jq '.assets[].name'
  # Expected:
  #   checksums.txt
  #   slk_darwin_amd64.tar.gz
  #   slk_darwin_arm64.tar.gz
  #   slk_linux_amd64.tar.gz
  #   slk_linux_arm64.tar.gz
  ```

- [ ] **Step 6：End-to-end smoke 安裝**

  Run（在當前 Mac arm64 上）：
  ```bash
  cd /tmp && rm -rf slk-e2e && mkdir slk-e2e && cd slk-e2e
  curl -sLO https://github.com/howar31/slk/releases/download/v0.1.0-rc1/slk_darwin_arm64.tar.gz
  curl -sLO https://github.com/howar31/slk/releases/download/v0.1.0-rc1/checksums.txt
  shasum -a 256 -c checksums.txt --ignore-missing
  # Expected: slk_darwin_arm64.tar.gz: OK
  tar xzf slk_darwin_arm64.tar.gz
  ./slk --version
  # Expected: 含 0.1.0-rc1
  cd / && rm -rf /tmp/slk-e2e
  ```

  若上面任一步 fail：**PR 1 不算完成**。回頭診斷後可能要 delete tag、修檔、重 tag `v0.1.0-rc2`。

---

# PR 2：Distribution channels

## Task 7：驗證 OpenClaw `install:` schema

**Files:** 不改 repo，只研究 + 記筆記到 plan 註解。

- [ ] **Step 1：查 OpenClaw skill spec**

  Run:
  ```bash
  # 嘗試的查源優先序
  npm view clawhub 2>&1 | head -20
  gh api repos/anthropic-experimental/openclaw 2>&1 | head -5
  curl -sS https://docs.openclaw.com 2>&1 | head -5
  ```

  若都查不到具體 schema，到 gws repo 看 `gws-shared` 之外有沒有其他 skill frontmatter 帶 `install:` 區塊：
  ```bash
  for skill in gws-drive gws-gmail gws-calendar; do
    echo "=== $skill ==="
    gh api "repos/googleworkspace/cli/contents/skills/$skill/SKILL.md" --jq .content \
      | base64 -d | sed -n '/^---/,/^---/p' | head -20
  done
  ```

- [ ] **Step 2：記決定**

  三種結果，選一條走：
  - **A：找到正式 schema** → Task 13（skill/SKILL.md edit）照 schema 寫。
  - **B：gws 確實有 install 區塊但沒文件** → 抄 gws 格式。
  - **C：都沒有；功能可能未實裝** → Task 13 **不寫** `install:` 區塊，只保留 `requires.bins: [slk]`。README 改寫時用「OpenClaw users: install slk via npm first, then add the skill」措辭，不承諾 autoinstall。

  把選的結果（A/B/C）記在 Task 13 開頭，避免後續 step 走錯。

## Task 8：建立 `npm/package.json`

**Files:**
- Create: `/opt/projects/slk/npm/package.json`

- [ ] **Step 1：確認 `npm/` 不存在**

  Run:
  ```bash
  ls /opt/projects/slk/npm 2>&1
  # Expected: No such file or directory
  mkdir /opt/projects/slk/npm
  ```

- [ ] **Step 2：寫 package.json**

  Create `/opt/projects/slk/npm/package.json`：
  ```json
  {
    "name": "@howar31/slk",
    "description": "Agent-facing Slack CLI for AI agents.",
    "version": "0.0.0",
    "license": "MIT",
    "author": "Howar31",
    "repository": {
      "type": "git",
      "url": "https://github.com/howar31/slk.git"
    },
    "homepage": "https://github.com/howar31/slk",
    "bugs": {
      "url": "https://github.com/howar31/slk/issues"
    },
    "bin": {
      "slk": "run.js"
    },
    "scripts": {
      "postinstall": "node install.js"
    },
    "engines": {
      "node": ">=18"
    },
    "preferUnplugged": true,
    "keywords": [
      "cli", "slack", "agent", "ai-agent", "automation"
    ],
    "publishConfig": {
      "access": "public"
    },
    "supportedPlatforms": {
      "darwin-arm64": { "artifact": "slk_darwin_arm64.tar.gz", "binary": "slk" },
      "darwin-x64":   { "artifact": "slk_darwin_amd64.tar.gz", "binary": "slk" },
      "linux-arm64":  { "artifact": "slk_linux_arm64.tar.gz",  "binary": "slk" },
      "linux-x64":    { "artifact": "slk_linux_amd64.tar.gz",  "binary": "slk" }
    }
  }
  ```

  `version: "0.0.0"` 是 placeholder，release.yml 會在 publish 前 overwrite 成 git tag 的版本。

- [ ] **Step 3：靜態驗證 JSON**

  Run:
  ```bash
  node -e "console.log(require('/opt/projects/slk/npm/package.json').name)"
  # Expected: @howar31/slk
  ```

## Task 9：建立 `npm/platform.js`

**Files:**
- Create: `/opt/projects/slk/npm/platform.js`

- [ ] **Step 1：寫 platform.js**

  Create `/opt/projects/slk/npm/platform.js`：
  ```javascript
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
  ```

- [ ] **Step 2：本機 smoke**

  Run:
  ```bash
  cd /opt/projects/slk/npm && node -e "console.log(require('./platform').getPlatform())"
  # Expected: { artifact: 'slk_darwin_arm64.tar.gz', binary: 'slk' }（在 mac arm64 上）
  ```

## Task 10：建立 `npm/install.js`

**Files:**
- Create: `/opt/projects/slk/npm/install.js`

- [ ] **Step 1：寫 install.js**

  Create `/opt/projects/slk/npm/install.js`：
  ```javascript
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
  ```

- [ ] **Step 2：靜態 syntax check**

  Run:
  ```bash
  node --check /opt/projects/slk/npm/install.js && echo OK
  # Expected: OK
  ```

  **不要** 現在跑 install.js — 它會嘗試從 `releases/download/v0.0.0/` 抓檔（package.json 仍是 placeholder version），會 404。實際驗證留到 Task 19 用 `npm pack` 走真版本。

## Task 11：建立 `npm/run.js`

**Files:**
- Create: `/opt/projects/slk/npm/run.js`

- [ ] **Step 1：寫 run.js**

  Create `/opt/projects/slk/npm/run.js`：
  ```javascript
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
  ```

- [ ] **Step 2：static syntax check**

  Run:
  ```bash
  node --check /opt/projects/slk/npm/run.js && echo OK
  # Expected: OK
  ```

## Task 12：建立 `npm/.gitignore`

**Files:**
- Create: `/opt/projects/slk/npm/.gitignore`

- [ ] **Step 1：寫 gitignore**

  Create `/opt/projects/slk/npm/.gitignore`：
  ```
  bin/
  *.tgz
  node_modules/
  ```

## Task 13：更新 `skill/SKILL.md` frontmatter

**Files:**
- Modify: `/opt/projects/slk/skill/SKILL.md`

**前置：先看 Task 7 的決定（A/B/C）。**

- [ ] **Step 1：定位 frontmatter**

  Run:
  ```bash
  sed -n '1,15p' /opt/projects/slk/skill/SKILL.md
  ```
  Expected：看到 `openclaw: category / requires.bins: [slk]`。

- [ ] **Step 2：依 Task 7 結論修 frontmatter**

  **若 Task 7 結論為 A 或 B**，把 frontmatter 段中 `openclaw:` 區塊改為：
  ```yaml
  metadata:
    version: 0.1.0
    openclaw:
      category: "productivity"
      requires:
        bins:
          - slk
      install:
        npm: "@howar31/slk"
  ```
  （若 Task 7 找到的真 schema 與 `{install: {npm: "<pkg>"}}` 不同，照真 schema 寫，並更新本 step 的 yaml）

  **若 Task 7 結論為 C**，frontmatter 不動，跳到 Step 3。

- [ ] **Step 3：靜態驗 yaml**

  Run:
  ```bash
  python3 -c "
  import yaml
  with open('/opt/projects/slk/skill/SKILL.md') as f:
      data = f.read()
  parts = data.split('---')
  if len(parts) < 3:
      raise SystemExit('No frontmatter')
  fm = yaml.safe_load(parts[1])
  print('name:', fm.get('name'))
  print('openclaw:', fm.get('metadata', {}).get('openclaw'))
  " 2>&1
  # Expected: name: slk; openclaw: dict 含 category/requires (含 install if A/B)
  ```

## Task 14：更新 `.goreleaser.yaml` — 恢復 brews 區塊

**Files:**
- Modify: `/opt/projects/slk/.goreleaser.yaml`

- [ ] **Step 1：在現有 checksum: 區塊後追加 brews:**

  在 `.goreleaser.yaml` 末尾追加：
  ```yaml
  brews:
    - repository:
        owner: howar31
        name: homebrew-tap
        token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
      directory: Formula
      description: Agent-facing Slack CLI for AI agents.
      license: MIT
      homepage: https://github.com/howar31/slk
      test: |
        assert_match version.to_s, shell_output("#{bin}/slk --version")
  ```

- [ ] **Step 2：移除 PR 1 留下的「brews block intentionally omitted」註解**

  把該行（在 `# brews block intentionally omitted in PR 1; ...` 起的 2 行註解）刪掉。

- [ ] **Step 3：本機 snapshot 驗證**

  Run:
  ```bash
  cd /opt/projects/slk
  HOMEBREW_TAP_TOKEN=dummy goreleaser release --snapshot --clean
  ls dist/homebrew/Formula/ 2>/dev/null || ls dist/howar31-homebrew-tap/Formula/ 2>/dev/null
  # Expected: 看到 slk.rb（goreleaser v2 路徑可能是 dist/homebrew/Formula/slk.rb 或類似；任一條 path 出現即可）
  cat dist/*/Formula/slk.rb 2>/dev/null | head -30
  # Expected: ruby formula，含 url "...slk_darwin_arm64.tar.gz" 等
  rm -rf dist
  ```

## Task 15：更新 `.github/workflows/release.yml` — 加 HOMEBREW_TAP_TOKEN + publish-npm job

**Files:**
- Modify: `/opt/projects/slk/.github/workflows/release.yml`

- [ ] **Step 1：在 goreleaser step 的 env 加 HOMEBREW_TAP_TOKEN**

  把：
  ```yaml
          env:
            GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  ```
  改成：
  ```yaml
          env:
            GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
            HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
  ```

- [ ] **Step 2：在檔尾追加 publish-npm job**

  在檔案最末追加：
  ```yaml
    publish-npm:
      needs: goreleaser
      if: ${{ !contains(github.ref_name, '-') }}
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v4
        - uses: actions/setup-node@v4
          with:
            node-version: '20'
            registry-url: 'https://registry.npmjs.org'
        - name: Sync npm version with git tag
          working-directory: npm
          run: |
            VER="${GITHUB_REF_NAME#v}"
            npm version "$VER" --no-git-tag-version
        - name: Publish to npm
          working-directory: npm
          env:
            NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}
          run: npm publish --access public
        - name: Smoke test installed package
          run: |
            sleep 30
            mkdir /tmp/slk-smoke && cd /tmp/slk-smoke
            npm init -y >/dev/null
            npm install --no-save "@howar31/slk@${GITHUB_REF_NAME#v}"
            ./node_modules/.bin/slk --version
  ```

  `if: !contains(github.ref_name, '-')` 跳過 `v0.1.0-rc1` 這類 prerelease — rc 階段不發 npm。

- [ ] **Step 3：靜態 lint**

  Run:
  ```bash
  python3 -c "import yaml; yaml.safe_load(open('/opt/projects/slk/.github/workflows/release.yml'))" && echo OK
  ```

## Task 16：建立 `gemini-extension.json`

**Files:**
- Create: `/opt/projects/slk/gemini-extension.json`

- [ ] **Step 1：寫檔**

  Create `/opt/projects/slk/gemini-extension.json`：
  ```json
  {
    "name": "slk",
    "version": "latest",
    "description": "Agent-facing Slack CLI for AI agents.",
    "contextFileName": "skill/SKILL.md"
  }
  ```

- [ ] **Step 2：驗 JSON**

  Run:
  ```bash
  node -e "console.log(require('/opt/projects/slk/gemini-extension.json').name)"
  # Expected: slk
  ```

## Task 17：更新 `README.md` — 全面改寫 Installation + 新增 Agent setup 段

**Files:**
- Modify: `/opt/projects/slk/README.md`

- [ ] **Step 1：定位 Installation 段**

  Run:
  ```bash
  grep -n "^## Installation" /opt/projects/slk/README.md
  grep -n "^## Quick start" /opt/projects/slk/README.md
  ```
  記住兩個行號 — 整段 Installation 會被換掉。

- [ ] **Step 2：用 Edit 把整個 `## Installation` 段（到下一個 `## Quick start` 之前）換成 5 條路徑**

  新內容：
  ````markdown
  ## Installation

  ### Homebrew (macOS / Linux)

  ```bash
  brew install howar31/tap/slk
  ```

  Adds the `howar31/homebrew-tap` formula automatically on first install.

  ### npm (anywhere Node.js 18+ runs)

  ```bash
  npm install -g @howar31/slk
  ```

  The package is scoped (`@howar31/slk`) because the unscoped name `slk`
  is already taken on npm. `postinstall` downloads the appropriate prebuilt
  binary from GitHub Releases and verifies its SHA256 checksum.

  ### Pre-built binary

  Download from [GitHub Releases](https://github.com/howar31/slk/releases).
  Replace `<os>` with `darwin` or `linux`, and `<arch>` with `amd64` or `arm64`.

  ```bash
  VER=0.1.0
  curl -sLO https://github.com/howar31/slk/releases/download/v${VER}/slk_<os>_<arch>.tar.gz
  curl -sLO https://github.com/howar31/slk/releases/download/v${VER}/checksums.txt
  shasum -a 256 -c checksums.txt --ignore-missing
  tar xzf slk_<os>_<arch>.tar.gz
  sudo mv slk /usr/local/bin/
  slk --version
  ```

  ### `go install` (Go 1.25+)

  ```bash
  go install github.com/howar31/slk/cmd/slk@latest
  ```

  Places `slk` in `$GOBIN` (typically `$HOME/go/bin`).

  ### From source

  ```bash
  git clone https://github.com/howar31/slk
  cd slk
  go build -ldflags "-X main.version=dev" -o slk ./cmd/slk
  ```
  ````

- [ ] **Step 3：定位 `## AI agent skills` 段並換成 Agent setup**

  把現有 `## AI agent skills` 段（從 `## AI agent skills` 到下一個 `## Usage` 之前）整段換成：
  ````markdown
  ## Agent setup

  ### Claude Code

  ```bash
  mkdir -p ~/.claude/skills/slk
  cp ./skill/SKILL.md ~/.claude/skills/slk/SKILL.md
  ```

  Claude Code activates the skill automatically when a task mentions Slack.

  ### Gemini CLI

  ```bash
  gemini extensions install https://github.com/howar31/slk
  ```

  Requires `slk` on your `$PATH` (install via Homebrew or npm first).

  ### OpenClaw

  OpenClaw reads `skill/SKILL.md`'s frontmatter. If the binary is missing,
  OpenClaw will install `@howar31/slk` from npm based on the `install:` hint
  in the skill metadata.

  ### Cursor / aider / others

  Either reference `slk --help` from your agent's instruction file, or paste
  the contents of [`skill/SKILL.md`](skill/SKILL.md) into the agent's
  persistent rules file (e.g., `.cursorrules`, `GEMINI.md`).
  ````

  **若 Task 7 結論為 C**（OpenClaw 沒 install schema 支援），把 OpenClaw 那段第二句改為：「OpenClaw users: install slk via npm or Homebrew first, then OpenClaw will detect the `slk` binary on PATH.」

- [ ] **Step 4：更新 Contents 目錄連結**

  Run:
  ```bash
  grep -n "AI agent skills" /opt/projects/slk/README.md
  ```
  把 Contents 那邊 `- [AI agent skills](#ai-agent-skills)` 改為 `- [Agent setup](#agent-setup)`。

- [ ] **Step 5：lint markdown 結構**

  Run:
  ```bash
  grep -c "^## " /opt/projects/slk/README.md
  # 比較 PR 前後段數應該一致或 +0
  ```

## Task 18：更新 SPEC.md 和 CLAUDE.md

**Files:**
- Modify: `/opt/projects/slk/SPEC.md`
- Modify: `/opt/projects/slk/CLAUDE.md`

- [ ] **Step 1：SPEC.md `## Deploy` 段全面改寫**

  定位：
  ```bash
  grep -n "^## Deploy" /opt/projects/slk/SPEC.md
  grep -n "^## Known Limitations" /opt/projects/slk/SPEC.md
  ```

  把 `## Deploy` 段（到 `## Known Limitations` 之前）換成：
  ```markdown
  ## Deploy

  Releases are tag-driven. Push a semver tag matching
  `v[0-9]+.[0-9]+.[0-9]+*` and `.github/workflows/release.yml`:

  1. Runs `goreleaser release --clean` on ubuntu-latest, producing
     `slk_<os>_<arch>.tar.gz` for `darwin/linux × amd64/arm64` plus
     `checksums.txt`.
  2. Creates the GitHub Release with the artifacts.
  3. Generates SLSA build provenance attestations via
     `actions/attest-build-provenance@v3`.
  4. Pushes a Homebrew formula update to `howar31/homebrew-tap` (uses
     `HOMEBREW_TAP_TOKEN` PAT).
  5. For non-prerelease tags, runs a `publish-npm` job that bumps
     `npm/package.json`'s version to match the tag and publishes
     `@howar31/slk` (uses `NPM_TOKEN`). Prereleases (tags containing
     `-`, e.g. `v0.1.0-rc1`) skip the npm publish.

  The `npm/` directory is a thin postinstall-driven wrapper:

  | File | Responsibility |
  |---|---|
  | `package.json` | Declares `bin.slk = run.js`, `scripts.postinstall = install.js`, and `supportedPlatforms` (4 darwin/linux × arm64/x64 entries). Version is overwritten by CI at publish time. |
  | `install.js` | Downloads the matching tarball + `checksums.txt` from GitHub Releases, verifies SHA256, extracts to `bin/`. |
  | `platform.js` | Maps `os.type()`/`os.arch()` to a `supportedPlatforms` key. |
  | `run.js` | Re-execs `bin/slk`; triggers `install.js` if the binary is missing (e.g. when user ran `npm install --ignore-scripts`). |

  Runtime state:

  - OAuth tokens live in `~/.config/slk/config.toml` (mode `0600`,
    TOML-encoded, multi-profile).
  - The ID-to-name resolver caches in `~/.config/slk/cache/` (mode `0700`).
  - Two env overrides: `SLK_PROFILE` (active profile) and `SLK_TOKEN`
    (raw token, highest precedence).
  ```

- [ ] **Step 2：CLAUDE.md `## Run / build` 段改 release 描述**

  定位：
  ```bash
  grep -n "## Run / build" /opt/projects/slk/CLAUDE.md
  ```

  把該段內 `goreleaser release --clean` 那行改為：
  ```bash
  git tag v0.1.0 && git push origin v0.1.0    # tag-driven release via GHA
  goreleaser release --snapshot --clean       # local dry-run only
  ```

- [ ] **Step 3：CLAUDE.md `## Workflow rules` 段加一條規則**

  在 `## Workflow rules` 列表中追加：
  ```markdown
  - Release is GHA-on-tag only (`.github/workflows/release.yml`). Do NOT run `goreleaser release` (without `--snapshot`) locally against the real `origin` remote — it will create a partial release and push a partial Homebrew formula.
  ```

## Task 19：本機 `npm pack` end-to-end smoke

**Files:** 不改 repo。

- [ ] **Step 1：在 `npm/` pack tarball**

  Run:
  ```bash
  cd /opt/projects/slk/npm
  npm version 0.1.0-rc1 --no-git-tag-version
  npm pack
  # Expected: howar31-slk-0.1.0-rc1.tgz
  ```

  **不要 commit version bump**；下一步會還原。

- [ ] **Step 2：本機 tmpdir install tarball，跑 postinstall，驗 binary 抓對檔**

  Run:
  ```bash
  cd /tmp && rm -rf slk-pack-smoke && mkdir slk-pack-smoke && cd slk-pack-smoke
  npm init -y >/dev/null
  npm install --no-save /opt/projects/slk/npm/howar31-slk-0.1.0-rc1.tgz
  # Expected: postinstall 從 v0.1.0-rc1 release 抓 binary、checksum 通過、bin/slk 出現
  ./node_modules/.bin/slk --version
  # Expected: 含 0.1.0-rc1
  cd / && rm -rf /tmp/slk-pack-smoke
  ```

  注意：這一步**需要** PR 1 的 `v0.1.0-rc1` release 已上線（Task 6 完成）。

- [ ] **Step 3：還原 npm/package.json 的 version**

  Run:
  ```bash
  cd /opt/projects/slk/npm
  npm version 0.0.0 --no-git-tag-version --allow-same-version
  rm howar31-slk-0.1.0-rc1.tgz
  ```

  確認最終 `package.json` `version` 是 `0.0.0`：
  ```bash
  node -e "console.log(require('./package.json').version)"
  # Expected: 0.0.0
  ```

## Task 20：Commit PR 2 changes

- [ ] **Step 1：開新 branch**

  Run:
  ```bash
  cd /opt/projects/slk
  git checkout main
  git pull --ff-only origin main
  git checkout -b distribution-channels
  ```

- [ ] **Step 2：檢查所有變更**

  Run:
  ```bash
  git status
  git diff --stat
  # Expected: ~10 files changed
  #   .github/workflows/release.yml    MODIFIED
  #   .goreleaser.yaml                 MODIFIED
  #   CLAUDE.md                        MODIFIED
  #   README.md                        MODIFIED
  #   SPEC.md                          MODIFIED
  #   gemini-extension.json            NEW
  #   npm/.gitignore                   NEW
  #   npm/install.js                   NEW
  #   npm/package.json                 NEW
  #   npm/platform.js                  NEW
  #   npm/run.js                       NEW
  #   skill/SKILL.md                   MODIFIED (若 Task 7 是 A/B；C 則無)
  ```

- [ ] **Step 3：把摘要呈現給使用者，等 approve**

- [ ] **Step 4：approve 後逐檔 add + commit**

  Run（依照實際變更的檔案列表加；不要 -A）：
  ```bash
  cd /opt/projects/slk
  git add \
    .github/workflows/release.yml \
    .goreleaser.yaml \
    CLAUDE.md \
    README.md \
    SPEC.md \
    gemini-extension.json \
    npm/.gitignore \
    npm/install.js \
    npm/package.json \
    npm/platform.js \
    npm/run.js \
    skill/SKILL.md
  git commit -m "$(cat <<'EOF'
  feat(dist): brew, npm, Gemini extension, OpenClaw autoinstall hint

  - .goreleaser.yaml: restore brews block, push formula updates to
    howar31/homebrew-tap (token via HOMEBREW_TAP_TOKEN).
  - .github/workflows/release.yml: pass HOMEBREW_TAP_TOKEN into the
    goreleaser step; add publish-npm job that syncs npm/package.json
    version with the git tag, publishes @howar31/slk, then smoke-tests
    via the published package. publish-npm skips prereleases (tags with -).
  - npm/: thin wrapper package. postinstall (install.js) downloads the
    matching darwin/linux × arm64/x64 tarball from GitHub Releases,
    verifies SHA256 against checksums.txt, extracts to bin/. run.js
    re-execs the binary, falling back to install.js on missing bin/.
  - gemini-extension.json: registers slk as a Gemini CLI extension
    pointing at skill/SKILL.md.
  - skill/SKILL.md: openclaw.install hint points to @howar31/slk so
    OpenClaw can autoinstall the binary when missing on PATH.
  - README: full Installation rewrite (5 paths in recommended order)
    and Agent setup section for Claude Code, Gemini CLI, OpenClaw,
    Cursor/aider/others.
  - SPEC.md ## Deploy: rewrite to describe the GHA-on-tag pipeline and
    npm/ wrapper file responsibilities.
  - CLAUDE.md: release commands now refer to git tag + push; add a rule
    forbidding local non-snapshot goreleaser runs.

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

## Task 21：Push PR 2 + 開 PR

- [ ] **Step 1：問使用者 approve push**

- [ ] **Step 2：Push branch 並開 PR**

  Run:
  ```bash
  cd /opt/projects/slk
  git push -u origin distribution-channels
  gh pr create --title "feat(dist): brew + npm + Gemini + OpenClaw autoinstall (PR 2/2)" \
    --body "$(cat <<'EOF'
  ## Summary
  - `@howar31/slk` npm scoped wrapper (postinstall pulls verified binaries from GitHub Releases).
  - `howar31/homebrew-tap` formula auto-updated by goreleaser on every release.
  - `gemini-extension.json` enables `gemini extensions install https://github.com/howar31/slk`.
  - `skill/SKILL.md` carries an OpenClaw install hint so missing-binary cases auto-install via npm.
  - README rewritten with 5 install paths and an Agent setup section.
  - SPEC.md and CLAUDE.md updated to reflect the new release/distribution shape.

  Depends on PR 1 (release pipeline foundation).

  ## Test plan
  - [x] Local `goreleaser release --snapshot --clean` produces `dist/*/Formula/slk.rb`
  - [x] Local `npm pack` + install on the `v0.1.0-rc1` release succeeds end-to-end
  - [ ] Merge → tag `v0.1.0` → all three jobs green (`goreleaser`, `publish-npm` smoke included)
  - [ ] `brew install howar31/tap/slk` on macOS → `slk --version` prints `0.1.0`
  - [ ] `npm i -g @howar31/slk` in a clean Linux container → `slk --version` prints `0.1.0`
  - [ ] `gemini extensions install https://github.com/howar31/slk` succeeds

  🤖 Generated with [Claude Code](https://claude.com/claude-code)
  EOF
  )"
  ```

## Task 22：加 NPM_TOKEN + HOMEBREW_TAP_TOKEN secrets（使用者操作）

**前置：在 tag `v0.1.0` 之前** 加 secrets，否則 release job 會 fail。

- [ ] **Step 1：使用者加 secrets**

  使用者執行（從 Task 0 暫存的 token 字串）：
  ```bash
  gh secret set NPM_TOKEN --repo howar31/slk
  # 貼上 npm token
  gh secret set HOMEBREW_TAP_TOKEN --repo howar31/slk
  # 貼上 PAT
  ```

- [ ] **Step 2：驗證 secrets 已加**

  Run:
  ```bash
  gh secret list --repo howar31/slk
  # Expected:
  #   HOMEBREW_TAP_TOKEN  ...
  #   NPM_TOKEN           ...
  ```

## Task 23：合 PR 2 + tag `v0.1.0` 正式發

- [ ] **Step 1：使用者 merge PR 2**

  GitHub UI review + merge。

- [ ] **Step 2：本機 pull main、確認乾淨**

  Run:
  ```bash
  cd /opt/projects/slk
  git checkout main
  git pull --ff-only origin main
  git log --oneline -5
  # Expected: 最新 2 commits 是 PR 1 與 PR 2 的 merge
  ```

- [ ] **Step 3：問使用者 approve tag `v0.1.0` + push**

  **這是 production release，務必再次確認。**

- [ ] **Step 4：tag + push**

  Run:
  ```bash
  cd /opt/projects/slk
  git tag -a v0.1.0 -m "First public release."
  git push origin v0.1.0
  ```

- [ ] **Step 5：監看 GHA**

  Run:
  ```bash
  sleep 5
  gh run list --workflow=release.yml --limit 1
  gh run watch <run-id> --exit-status
  # Expected: 兩個 job 都 0 — goreleaser + publish-npm
  ```

- [ ] **Step 6：確認三條 channel 都上線**

  ```bash
  # Release
  gh release view v0.1.0 --json assets --jq '.assets[].name'
  # Expected: checksums.txt + 4 個 .tar.gz

  # npm
  npm view @howar31/slk version
  # Expected: 0.1.0

  # Homebrew tap formula
  gh api repos/howar31/homebrew-tap/contents/Formula/slk.rb --jq .name
  # Expected: slk.rb
  ```

## Task 24：End-to-end install matrix（人工驗證）

**Files:** 不改 repo。

- [ ] **Step 1：macOS — Homebrew install**

  ```bash
  brew install howar31/tap/slk
  slk --version
  # Expected: 0.1.0
  ```

- [ ] **Step 2：Linux (Docker) — npm install**

  ```bash
  docker run --rm -it node:20 bash -c "npm install -g @howar31/slk && slk --version"
  # Expected: 0.1.0
  ```

- [ ] **Step 3：Linux (Docker) — pre-built binary curl install**

  ```bash
  docker run --rm -it ubuntu:24.04 bash -c "
    apt-get update -qq && apt-get install -qq -y curl &&
    curl -sLO https://github.com/howar31/slk/releases/download/v0.1.0/slk_linux_amd64.tar.gz &&
    curl -sLO https://github.com/howar31/slk/releases/download/v0.1.0/checksums.txt &&
    sha256sum -c checksums.txt --ignore-missing &&
    tar xzf slk_linux_amd64.tar.gz &&
    ./slk --version"
  # Expected: 0.1.0
  ```

- [ ] **Step 4：Gemini CLI extension install（若使用者有裝 gemini CLI）**

  ```bash
  gemini extensions install https://github.com/howar31/slk
  # Expected: install 成功（細節依 gemini CLI 版本）
  ```

- [ ] **Step 5：Claude Code skill smoke**

  - 把本 repo 的 `skill/SKILL.md` cp 到 `~/.claude/skills/slk/`
  - 在 Claude Code session 裡開新對話，問「請用 slk 列我可見的頻道」
  - Expected：Claude Code 觸發 slk skill，呼叫 `slk search channels --dry-run` 或實際 `slk channel list`

- [ ] **Step 6：任一條 fail → 開 follow-up issue（不 hot-fix）**

  v0.1.0 已發出去；不要直接 force-fix。診斷後在 follow-up PR 修，tag `v0.1.1`。

---

## Self-Review

**Spec coverage 對照表：**

| Spec requirement | Plan task(s) |
|---|---|
| GHA on tag → binaries + provenance | Task 2, Task 6 |
| Pre-built binaries（4 platforms + SHA256） | Task 1, Task 2, Task 6 |
| Personal Homebrew tap | Task 0 (Step 1), Task 14, Task 22, Task 23 |
| `@howar31/slk` npm wrapper | Task 8–12, Task 15, Task 19, Task 23 |
| Gemini CLI extension | Task 16, Task 17 (Agent setup 段) |
| OpenClaw `install` hint | Task 7, Task 13 |
| README 5 install paths + Agent setup | Task 3 (PR 1 部分), Task 17 (PR 2 完整) |
| SPEC.md `## Deploy` 更新 | Task 18 (Step 1) |
| CLAUDE.md release rules 更新 | Task 18 (Step 2, 3) |
| `v0.1.0-rc1` 隔離驗證 | Task 6 |
| Required secrets 流程 | Task 0 (Step 3, 4), Task 22 |
| publish-npm 跳過 prereleases | Task 15 (Step 2) |
| 命名統一 `slk_<os>_<arch>.tar.gz` | Task 1 (Step 2), 整個 install.js |

**Placeholder scan：** 無「TBD/TODO」；Task 7 唯一不確定點（OpenClaw schema）已給 A/B/C 三分支處理，下游 Task 13 / Task 17 Step 3 都有「若 C 則 ...」備案。Task 8 package.json `version: "0.0.0"` 是顯式 placeholder，Task 15 / Task 19 都明說 CI / `npm pack` 會 overwrite。

**Type consistency：** 確認跨 task 一致：
- 檔名 `slk_<os>_<arch>.tar.gz` — Task 1 定義，Task 8 supportedPlatforms 引用，Task 10 install.js download 引用，Task 17 README 引用 ✓
- npm package name `@howar31/slk` — Task 8 定義，Task 13 frontmatter 引用，Task 15 smoke-test 引用，Task 17 README 引用 ✓
- secret 名稱 `NPM_TOKEN` / `HOMEBREW_TAP_TOKEN` — Task 0 / 14 / 15 / 22 一致 ✓
- Branch 命名：`release-pipeline-foundation`（PR 1）、`distribution-channels`（PR 2）— Task 5、Task 20 一致 ✓
- Tag pattern `v[0-9]+.[0-9]+.[0-9]+*` — Task 2 workflow trigger 與 Task 6 `v0.1.0-rc1` / Task 23 `v0.1.0` 都吻合 ✓
