import { createHash } from "node:crypto";
import { cpSync, lstatSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";
import { readWellVersions } from "./prepare-versions.mjs";

const wellDirectory = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const usage = "Usage: npm run release -- [--output directory]\nBuilds Well for the current platform and writes to ../release by default.";

export function parseOptions(argumentsList) {
  const options = {};
  for (let index = 0; index < argumentsList.length; index++) {
    switch (argumentsList[index]) {
      case "--help":
      case "-h":
        options.help = true;
        break;
      case "--output":
        options.output = argumentsList[++index];
        if (!options.output || options.output.startsWith("--")) throw new Error("--output requires a directory");
        break;
      default:
        throw new Error(`unknown argument: ${argumentsList[index]}`);
    }
  }
  return options;
}

function ensureDirectory(directory) {
  let info;
  try {
    info = lstatSync(directory);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  if (info && !info.isDirectory()) throw new Error(`refusing to replace non-directory or symbolic link ${directory}`);
  mkdirSync(directory, { recursive: true });
}

function execute(directory, environment) {
  const command = process.platform === "win32" ? "npm.cmd" : "npm";
  const result = spawnSync(command, ["run", "tauri", "--", "build"], {
    cwd: directory,
    env: environment,
    stdio: "inherit",
    shell: process.platform === "win32",
  });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`Well build failed (${result.signal ?? result.status})`);
}

function collectBundles(source, destination) {
  let found = false;
  for (const format of readdirSync(source, { withFileTypes: true })) {
    if (!format.isDirectory()) continue;
    for (const artifact of readdirSync(join(source, format.name), { withFileTypes: true })) {
      const matches = artifact.isDirectory() && /\.app$/i.test(artifact.name) ||
        artifact.isFile() && /\.(dmg|appimage|deb|rpm|msi|exe|pkg|zip|tar\.gz|sig)$/i.test(artifact.name);
      if (!matches) continue;
      mkdirSync(join(destination, format.name), { recursive: true });
      cpSync(join(source, format.name, artifact.name), join(destination, format.name, artifact.name), { recursive: true, verbatimSymlinks: true });
      found = true;
    }
  }
  if (!found) throw new Error(`no Well application bundles or installers found in ${source}`);
}

function checksums(directory, prefix = ".") {
  const lines = [];
  for (const entry of readdirSync(directory, { withFileTypes: true }).sort((left, right) => left.name < right.name ? -1 : left.name > right.name ? 1 : 0)) {
    const relative = `${prefix}/${entry.name}`;
    if (entry.isDirectory()) lines.push(...checksums(join(directory, entry.name), relative));
    else if (entry.isFile() && entry.name !== "SHA256SUMS" && entry.name !== ".gitkeep") {
      lines.push(`${createHash("sha256").update(readFileSync(join(directory, entry.name))).digest("hex")}  ${relative}\n`);
    }
  }
  return lines;
}

export function buildWell({ directory = wellDirectory, output = resolve(directory, "../release"), environment = process.env, run = execute } = {}) {
  readWellVersions(resolve(directory, "../versions.json"));
  output = resolve(output);
  const platform = `${process.platform === "win32" ? "windows" : process.platform}-${process.arch}`;
  ensureDirectory(output);
  ensureDirectory(join(output, platform));
  const staging = mkdtempSync(resolve(directory, "../.release.tmp."));
  try {
    const profile = resolve(directory, "src-tauri", environment.CARGO_TARGET_DIR || "target", "release");
    rmSync(join(profile, "bundle"), { recursive: true, force: true });
    run(directory, environment);
    const artifact = join(staging, "well");
    collectBundles(join(profile, "bundle"), join(artifact, "bundle"));
    const executable = process.platform === "win32" ? "well.exe" : "well";
    cpSync(join(profile, executable), join(artifact, executable));
    ensureDirectory(output);
    ensureDirectory(join(output, platform));
    rmSync(join(output, platform, "well"), { recursive: true, force: true });
    renameSync(artifact, join(output, platform, "well"));
    writeFileSync(join(staging, "SHA256SUMS"), checksums(output).join(""));
    renameSync(join(staging, "SHA256SUMS"), join(output, "SHA256SUMS"));
    console.log(`release: wrote well to ${output} and updated SHA256SUMS`);
  } finally {
    rmSync(staging, { recursive: true, force: true });
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    const options = parseOptions(process.argv.slice(2));
    if (options.help) console.log(usage);
    else buildWell(options);
  } catch (error) {
    console.error(`release: ${error.message}`);
    process.exitCode = 1;
  }
}
