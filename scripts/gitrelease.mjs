import { cpSync, existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, renameSync, rmSync } from "node:fs";
import { dirname, join, posix, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";

const repository = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const targets = [
  { target: "darwin/arm64", directory: "darwin-arm64", label: "darwin-arm64" },
  { target: "windows/amd64", directory: "windows-x64", label: "windows-amd64" },
  { target: "linux/amd64", directory: "linux-x64", label: "linux-amd64" },
];

function execute(command, argumentsList, options) {
  const result = spawnSync(command, argumentsList, { ...options, stdio: "inherit" });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${command} failed (${result.signal ?? result.status})`);
}

function ensureDirectory(directory) {
  try {
    if (!lstatSync(directory).isDirectory()) throw new Error(`refusing to replace non-directory or symbolic link ${directory}`);
  } catch (error) { if (error.code !== "ENOENT") throw error; }
  mkdirSync(directory, { recursive: true });
}

function copyFile(source, destination) {
  if (!lstatSync(source).isFile()) throw new Error(`expected regular release file: ${source}`);
  cpSync(source, destination);
}

export function parseGitReleaseOptions(argumentsList) {
  const extensions = [];
  for (let index = 0; index < argumentsList.length; index++) {
    if (argumentsList[index] !== "--with-ext" || !/^[a-z][a-z0-9-]*$/.test(argumentsList[index + 1] ?? "")) {
      throw new Error("expected --with-ext NAME; repeat it to select multiple extensions");
    }
    extensions.push(argumentsList[++index]);
  }
  return { extensions: [...new Set(extensions)] };
}

function extensionFiles(directory, extensions, windows) {
  const metadataName = "norns.exts.meta.json";
  const metadataFile = join(directory, metadataName);
  if (!lstatSync(metadataFile).isFile()) throw new Error("expected regular extension metadata file");
  // The Go builder validates the catalog and stages only declared attachments.
  // Select those exact files rather than sweeping the build directory into ZIPs.
  const catalog = JSON.parse(readFileSync(metadataFile, "utf8"));
  if (!Array.isArray(catalog) || catalog.length !== extensions.length) throw new Error("unexpected extension catalog");
  const remaining = new Set(extensions);
  const names = [metadataName, ...extensions.map((name) => `norns-${name}${windows ? ".exe" : ""}`)];
  const seen = new Set(names.map((name) => name.toLowerCase()));
  for (const item of catalog) {
    if (!remaining.delete(item.name) || !Array.isArray(item.release_files)) throw new Error("unexpected extension catalog entry");
    for (const file of item.release_files) {
      if (typeof file.origin_file !== "string" || !file.origin_file || typeof file.target_file !== "string") throw new Error("invalid extension release file");
      const suffix = file.target_file || posix.basename(file.origin_file);
      const name = `norns.exts.${item.name}.${suffix}`;
      if (!suffix || /[\\/:*?"<>|\x00-\x1f\x7f]/.test(suffix) || /[. ]$/.test(suffix) || Buffer.byteLength(name) > 255 || seen.has(name.toLowerCase())) {
        throw new Error("invalid extension release filename");
      }
      seen.add(name.toLowerCase());
      names.push(name);
    }
  }
  return names;
}

export function buildGitRelease({ root = repository, environment = process.env, host = process, run = execute, extensions = [] } = {}) {
  if (host.platform !== "darwin" || host.arch !== "arm64") {
    throw new Error("--gitrelease requires a macOS ARM64 host to build the Well DMG; Well Windows is not included");
  }
  extensions = [...new Set(extensions)];
  for (const name of extensions) {
    if (typeof name !== "string" || name.length > 128 || !/^[a-z][a-z0-9-]*$/.test(name)) throw new Error("invalid extension name");
    const entry = join(root, "norns/internal/custom_extensions", name, "cmd", `norns-${name}`, "main.go");
    if (!existsSync(entry) || !lstatSync(entry).isFile()) throw new Error(`unknown extension: ${name}`);
  }
  const versions = JSON.parse(readFileSync(join(root, "versions.json"), "utf8"));
  for (const product of ["norns", "nornsd", "well"]) {
    if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(versions[product]?.version ?? "")) {
      throw new Error(`versions.json: ${product}.version must be a three-part version string`);
    }
  }
  const output = join(root, "release");
  const destination = join(output, "gitrelease");
  ensureDirectory(output);
  // Check an existing destination without creating an empty public release.
  try {
    if (!lstatSync(destination).isDirectory()) throw new Error(`refusing to replace non-directory or symbolic link ${destination}`);
  } catch (error) { if (error.code !== "ENOENT") throw error; }
  const staging = mkdtempSync(join(output, ".release.tmp."));
  try {
    const artifacts = join(staging, "artifacts");
    const attachments = join(staging, "attachments");
    mkdirSync(attachments);
    const buildEnvironment = {
      ...environment,
      BUILD_TIME: environment.BUILD_TIME || new Date().toISOString().replace(/\.\d{3}Z$/, "Z"),
    };
    const options = { env: buildEnvironment };
    const targetArguments = targets.flatMap(({ target }) => ["--target", target]);
    for (const product of ["norns", "nornsd"]) {
      const extensionArguments = product === "norns" ? extensions.flatMap((name) => ["--with-ext", name]) : [];
      run("go", ["run", "./scripts/release.go", ...targetArguments, ...extensionArguments, "--output", artifacts], { ...options, cwd: join(root, product) });
    }
    run("npm", ["run", "release", "--", "--output", artifacts], { ...options, cwd: join(root, "well") });

    for (const product of ["norns", "nornsd"]) {
      for (const platform of targets) {
        const packageDirectory = join(staging, `${product}-${platform.label}`);
        mkdirSync(packageDirectory);
        const executable = product + (platform.target.startsWith("windows/") ? ".exe" : "");
        const files = [executable, "LICENSE"];
        copyFile(join(artifacts, platform.directory, executable), join(packageDirectory, executable));
        copyFile(join(root, "LICENSE"), join(packageDirectory, "LICENSE"));
        if (product === "norns") {
          files.push("THIRD_PARTY_NOTICES.md");
          copyFile(join(root, "norns/THIRD_PARTY_NOTICES.md"), join(packageDirectory, "THIRD_PARTY_NOTICES.md"));
          if (extensions.length) {
            for (const name of extensionFiles(join(artifacts, platform.directory), extensions, platform.target.startsWith("windows/"))) {
              copyFile(join(artifacts, platform.directory, name), join(packageDirectory, name));
              files.push(name);
            }
          }
        } else {
          files.push("nornsd.example.yaml");
          copyFile(join(artifacts, platform.directory, "nornsd.example.yaml"), join(packageDirectory, "nornsd.example.yaml"));
        }
        const archive = join(attachments, `${product}-v${versions[product].version}-${platform.label}.zip`);
        run("zip", ["-X", "-q", archive, ...files], { ...options, cwd: packageDirectory });
      }
    }
    const dmgDirectory = join(artifacts, "darwin-arm64/well/bundle/dmg");
    const images = readdirSync(dmgDirectory).filter((name) => name.toLowerCase().endsWith(".dmg"));
    if (images.length !== 1) throw new Error("expected exactly one newly built Well DMG");
    copyFile(join(dmgDirectory, images[0]), join(attachments, `well-v${versions.well.version}-darwin-arm64.dmg`));
    if (readdirSync(attachments).length !== 7) throw new Error("expected exactly seven GitHub release attachments");

    const previous = join(staging, "previous");
    if (existsSync(destination)) {
      if (!lstatSync(destination).isDirectory()) throw new Error("GitHub release destination changed during build");
      renameSync(destination, previous);
    }
    try { renameSync(attachments, destination); } catch (error) {
      if (existsSync(previous)) renameSync(previous, destination);
      throw error;
    }
    // Keep Cargo caches; discard the duplicate bundle only after successful publication.
    const profile = resolve(root, "well/src-tauri", environment.CARGO_TARGET_DIR || "target", "release");
    rmSync(join(profile, "bundle"), { recursive: true, force: true });
    console.log(`release: wrote seven GitHub release attachments to ${destination}`);
    return readdirSync(destination).sort();
  } finally {
    rmSync(staging, { recursive: true, force: true });
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try { buildGitRelease(parseGitReleaseOptions(process.argv.slice(2))); } catch (error) {
    console.error(`release: ${error.message}`);
    process.exitCode = 1;
  }
}
