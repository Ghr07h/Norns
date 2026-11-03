import { mkdtempSync, readFileSync, rmSync, statSync, utimesSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { prepareVersions, readWellVersions } from "./prepare-versions.mjs";

const directories: string[] = [];

function fixture(well: unknown = { version: "2.3.4", support_api_schema: 7 }) {
  const cargoDirectory = mkdtempSync(join(tmpdir(), "well-versions-"));
  directories.push(cargoDirectory);
  const versionsPath = join(cargoDirectory, "versions.json");
  writeFileSync(versionsPath, JSON.stringify({ well }));
  writeFileSync(join(cargoDirectory, "Cargo.toml"), '[package]\nname = "well"\nversion = "0.1.0"\n\n[dependencies]\nexample = "0.1.0"\n');
  writeFileSync(join(cargoDirectory, "Cargo.lock"), 'version = 4\n\n[[package]]\nname = "example"\nversion = "0.1.0"\n\n[[package]]\nname = "well"\nversion = "0.1.0"\n');
  return { cargoDirectory, versionsPath };
}

afterEach(() => {
  for (const directory of directories.splice(0)) rmSync(directory, { recursive: true, force: true });
});

describe("Well version preparation", () => {
  it("reads the root manifest and updates only Well Cargo metadata", () => {
    const paths = fixture();
    expect(readWellVersions(paths.versionsPath)).toEqual({ version: "2.3.4", support_api_schema: 7 });
    prepareVersions(paths);
    expect(readFileSync(join(paths.cargoDirectory, "Cargo.toml"), "utf8")).toBe('[package]\nname = "well"\nversion = "2.3.4"\n\n[dependencies]\nexample = "0.1.0"\n');
    expect(readFileSync(join(paths.cargoDirectory, "Cargo.lock"), "utf8")).toBe('version = 4\n\n[[package]]\nname = "example"\nversion = "0.1.0"\n\n[[package]]\nname = "well"\nversion = "2.3.4"\n');
  });

  it("does not rewrite unchanged metadata and follows subsequent manifest changes", () => {
    const paths = fixture();
    prepareVersions(paths);
    const cargoManifest = join(paths.cargoDirectory, "Cargo.toml");
    utimesSync(cargoManifest, 1, 1);
    const modified = statSync(cargoManifest).mtimeMs;
    prepareVersions(paths);
    expect(statSync(cargoManifest).mtimeMs).toBe(modified);
    writeFileSync(paths.versionsPath, JSON.stringify({ well: { version: "2.3.5", support_api_schema: 8 } }));
    prepareVersions(paths);
    expect(readFileSync(cargoManifest, "utf8")).toContain('version = "2.3.5"');
    expect(readFileSync(join(paths.cargoDirectory, "Cargo.lock"), "utf8")).toContain('name = "well"\nversion = "2.3.5"');
  });

  it.each([undefined, { version: "dev", support_api_schema: 7 }, { version: "1.2", support_api_schema: 7 }, { version: "01.2.3", support_api_schema: 7 }, { version: "1.2.3", support_api_schema: "7" }, { version: "1.2.3", support_api_schema: 0 }, { version: "1.2.3", support_api_schema: -1 }, { version: "1.2.3", support_api_schema: 1.5 }, { version: "1.2.3", support_api_schema: Number.MAX_SAFE_INTEGER + 1 }])("rejects invalid manifest values without changing metadata: %j", (well) => {
    const paths = fixture(well ?? null);
    const cargoManifest = join(paths.cargoDirectory, "Cargo.toml");
    const source = readFileSync(cargoManifest, "utf8");
    expect(() => prepareVersions(paths)).toThrow("versions.json:");
    expect(readFileSync(cargoManifest, "utf8")).toBe(source);
  });

  it("does not partially update metadata when the lockfile is invalid", () => {
    const paths = fixture();
    writeFileSync(join(paths.cargoDirectory, "Cargo.lock"), "version = 4\n");
    const cargoManifest = join(paths.cargoDirectory, "Cargo.toml");
    const source = readFileSync(cargoManifest, "utf8");
    expect(() => prepareVersions(paths)).toThrow("Cannot find Well package version");
    expect(readFileSync(cargoManifest, "utf8")).toBe(source);
  });
});
