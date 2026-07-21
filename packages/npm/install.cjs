#!/usr/bin/env node
"use strict";

const crypto = require("node:crypto");
const fs = require("node:fs");
const https = require("node:https");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");

const MAX_ARCHIVE_BYTES = 128 * 1024 * 1024;
const MAX_CHECKSUM_BYTES = 1024 * 1024;

function releaseTarget(platform = process.platform, architecture = process.arch) {
  const operatingSystems = { darwin: "darwin", linux: "linux" };
  const architectures = { x64: "x86_64", arm64: "arm64" };
  const targetOS = operatingSystems[platform];
  const targetArch = architectures[architecture];
  if (!targetOS || !targetArch) throw new Error(`unsupported platform ${platform}/${architecture}`);
  return { os: targetOS, arch: targetArch };
}

function releaseURLs(version, target, base = "https://github.com/TheSnakeFang/rlviz/releases/download") {
  const normalized = version.replace(/^v/, "");
  if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/.test(normalized)) {
    throw new Error(`invalid rlviz package version ${version}`);
  }
  const archive = `rlviz_${normalized}_${target.os}_${target.arch}.tar.gz`;
  const release = `${base.replace(/\/$/, "")}/v${normalized}`;
  return { archive, archiveURL: `${release}/${archive}`, checksumsURL: `${release}/checksums.txt` };
}

function expectedChecksum(contents, archive) {
  for (const line of contents.split(/\r?\n/)) {
    const match = line.trim().match(/^([a-f0-9]{64})\s+\*?(.+)$/);
    if (match && match[2] === archive) return match[1];
  }
  throw new Error(`checksum not found for ${archive}`);
}

function download(url, maxBytes, redirects = 5) {
  return new Promise((resolve, reject) => {
    const parsed = new URL(url);
    if (parsed.protocol !== "https:") {
      reject(new Error(`refusing non-HTTPS download ${url}`));
      return;
    }
    const request = https.get(parsed, { headers: { "user-agent": "rlviz-npm-installer" } }, (response) => {
      if (response.statusCode >= 300 && response.statusCode < 400 && response.headers.location) {
        response.resume();
        if (redirects <= 0) reject(new Error(`too many redirects downloading ${url}`));
        else resolve(download(new URL(response.headers.location, url).toString(), maxBytes, redirects - 1));
        return;
      }
      if (response.statusCode !== 200) {
        response.resume();
        reject(new Error(`download ${url} returned HTTP ${response.statusCode}`));
        return;
      }
      const declared = Number(response.headers["content-length"]);
      if (Number.isFinite(declared) && declared > maxBytes) {
        response.resume();
        reject(new Error(`download ${url} exceeds ${maxBytes} bytes`));
        return;
      }
      const chunks = [];
      let received = 0;
      response.on("data", (chunk) => {
        received += chunk.length;
        if (received > maxBytes) response.destroy(new Error(`download ${url} exceeds ${maxBytes} bytes`));
        else chunks.push(chunk);
      });
      response.on("end", () => resolve(Buffer.concat(chunks)));
      response.on("error", reject);
    }).on("error", reject);
    request.setTimeout(30_000, () => request.destroy(new Error(`download ${url} timed out`)));
  });
}

async function install(options = {}) {
  if (process.env.RLVIZ_SKIP_DOWNLOAD === "1") return;
  const packageDirectory = options.packageDirectory || __dirname;
  const packageJSON = options.packageVersion ? { version: options.packageVersion } : require("./package.json");
  const target = releaseTarget(options.platform, options.architecture);
  const urls = releaseURLs(packageJSON.version, target, options.releaseBaseURL || process.env.RLVIZ_RELEASE_BASE_URL);
  const downloadFile = options.download || download;
  const [archiveData, checksumsData] = await Promise.all([
    downloadFile(urls.archiveURL, MAX_ARCHIVE_BYTES),
    downloadFile(urls.checksumsURL, MAX_CHECKSUM_BYTES),
  ]);
  if (!Buffer.isBuffer(archiveData) || archiveData.length > MAX_ARCHIVE_BYTES) throw new Error("invalid or oversized release archive");
  if (!Buffer.isBuffer(checksumsData) || checksumsData.length > MAX_CHECKSUM_BYTES) throw new Error("invalid or oversized checksum file");
  const expected = expectedChecksum(checksumsData.toString("utf8"), urls.archive);
  const actual = crypto.createHash("sha256").update(archiveData).digest("hex");
  if (actual !== expected) throw new Error(`checksum verification failed for ${urls.archive}`);

  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), "rlviz-npm-"));
  try {
    const archivePath = path.join(temporary, urls.archive);
    fs.writeFileSync(archivePath, archiveData, { mode: 0o600 });
    // Extract only the expected top-level binary. Other archive entries never
    // reach disk, preventing path traversal through a compromised archive.
    const extracted = spawnSync("tar", ["-xzf", archivePath, "-C", temporary, "--", "rlviz"], { encoding: "utf8" });
    if (extracted.error) throw new Error(`extract release archive: ${extracted.error.message}`);
    if (extracted.status !== 0) throw new Error(`extract release archive: ${(extracted.stderr || "tar failed").trim()}`);
    const source = path.join(temporary, "rlviz");
    const sourceInfo = fs.lstatSync(source);
    if (!sourceInfo.isFile() || sourceInfo.size <= 0 || sourceInfo.size > MAX_ARCHIVE_BYTES) throw new Error("release archive does not contain a safe rlviz binary");
    const binDirectory = path.join(packageDirectory, "bin");
    const destination = path.join(binDirectory, "rlviz");
    const staged = path.join(binDirectory, `.rlviz-${process.pid}-${crypto.randomBytes(6).toString("hex")}`);
    fs.mkdirSync(binDirectory, { recursive: true });
    try {
      fs.copyFileSync(source, staged, fs.constants.COPYFILE_EXCL);
      fs.chmodSync(staged, 0o755);
      fs.renameSync(staged, destination);
    } finally {
      fs.rmSync(staged, { force: true });
    }
  } finally {
    fs.rmSync(temporary, { recursive: true, force: true });
  }
}

module.exports = { download, expectedChecksum, install, releaseTarget, releaseURLs, MAX_ARCHIVE_BYTES, MAX_CHECKSUM_BYTES };

if (require.main === module) {
  install().catch((error) => {
    console.error(`rlviz: ${error.message}`);
    process.exit(1);
  });
}
