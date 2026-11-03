import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const wellDirectory = resolve(dirname(fileURLToPath(import.meta.url)), "..");

export function readWellVersions(versionsPath = join(wellDirectory, "../versions.json")) {
  const { well } = JSON.parse(readFileSync(versionsPath, "utf8"));
  if (typeof well?.version !== "string" || !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(well.version)) {
    throw new Error("versions.json: well.version must be a three-part version string");
  }
  if (!Number.isSafeInteger(well.support_api_schema) || well.support_api_schema <= 0) {
    throw new Error("versions.json: well.support_api_schema must be a positive safe integer");
  }
  return well;
}

export function prepareVersions({ versionsPath, cargoDirectory = join(wellDirectory, "src-tauri") } = {}) {
  const well = readWellVersions(versionsPath);
  const targets = [
    ["Cargo.toml", /(\[package\][\s\S]*?^version\s*=\s*")[^"]*(")/m],
    ["Cargo.lock", /(\[\[package\]\]\r?\nname = "well"\r?\nversion = ")[^"]*(")/],
  ];
  const updates = targets.map(([name, pattern]) => {
    const filename = join(cargoDirectory, name);
    const source = readFileSync(filename, "utf8");
    if (!pattern.test(source)) throw new Error(`Cannot find Well package version in ${filename}`);
    const output = source.replace(pattern, (_, prefix, suffix) => `${prefix}${well.version}${suffix}`);
    return { filename, source, output };
  });
  for (const { filename, source, output } of updates) {
    if (source !== output) writeFileSync(filename, output);
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  prepareVersions();
}
