import { createHash } from "node:crypto";
import { existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, readlinkSync, realpathSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { buildWell, parseOptions, releaseEnvironment } from "./release.mjs";

const directories: string[] = [];
const platform = `${process.platform === "win32" ? "windows" : process.platform}-${process.arch}`;
const executable = process.platform === "win32" ? "well.exe" : "well";

function write(filename: string, contents: string) {
  mkdirSync(dirname(filename), { recursive: true });
  writeFileSync(filename, contents);
}

function fixture() {
  const root = mkdtempSync(join(tmpdir(), "well-release-"));
  directories.push(root);
  const directory = join(root, "well");
  mkdirSync(directory);
  write(join(root, "versions.json"), JSON.stringify({ well: { version: "2.3.4", support_api_schema: 7 } }));
  const output = join(root, "release");
  return { root, directory, output };
}

function fakeBuild(directory: string, environment: NodeJS.ProcessEnv) {
  const profile = resolve(directory, "src-tauri", environment.CARGO_TARGET_DIR || "target", "release");
  expect(existsSync(join(profile, "bundle/stale.pkg"))).toBe(false);
  write(join(profile, executable), "new Well");
  write(join(profile, "bundle/package/Well.pkg"), "new bundle");
  write(join(profile, "bundle/package/bundle_dmg.sh"), "build helper");
  write(join(profile, "bundle/share/create-dmg/template.applescript"), "build helper");
  write(join(profile, "bundle/macos/Well.app/Contents/executable"), "app binary");
  if (process.platform !== "win32") symlinkSync("executable", join(profile, "bundle/macos/Well.app/Contents/current"));
}

afterEach(() => {
  for (const directory of directories.splice(0)) rmSync(directory, { recursive: true, force: true });
});

describe("Well release", () => {
  it("preserves Rust options and safely remaps source paths containing spaces", () => {
    const paths = fixture();
    const environment = {
      HOME: join(paths.root, "user home"),
      CARGO_HOME: join(paths.root, "cargo home"),
      RUSTFLAGS: "-C debuginfo=1",
      EXISTING_SETTING: "unchanged",
    };
    const result = releaseEnvironment(paths.directory, environment);
    const flags = result.CARGO_ENCODED_RUSTFLAGS.split("\x1f");
    expect(flags.slice(0, 2)).toEqual(["-C", "debuginfo=1"]);
    expect(flags).toContain(`--remap-path-prefix=${environment.CARGO_HOME}=/cargo`);
    expect(flags).toContain(`--remap-path-prefix=${realpathSync(paths.root)}=/workspace/norns`);
    expect(result.RUSTFLAGS).toBeUndefined();
    expect(result.EXISTING_SETTING).toBe("unchanged");
    expect(environment.RUSTFLAGS).toBe("-C debuginfo=1");
    const encoded = releaseEnvironment(paths.directory, { ...environment, CARGO_ENCODED_RUSTFLAGS: "--cfg\x1fcustom=\"two words\"" });
    expect(encoded.CARGO_ENCODED_RUSTFLAGS.split("\x1f").slice(0, 2)).toEqual(["--cfg", 'custom="two words"']);
  });

  it.skipIf(process.platform === "win32")("remaps both lexical and real source paths", () => {
    const paths = fixture();
    const alias = join(paths.root, "source alias");
    symlinkSync(paths.root, alias);
    const flags = releaseEnvironment(join(alias, "well"), {}).CARGO_ENCODED_RUSTFLAGS.split("\x1f");
    expect(flags).toContain(`--remap-path-prefix=${alias}=/workspace/norns`);
    expect(flags).toContain(`--remap-path-prefix=${realpathSync(paths.root)}=/workspace/norns`);
  });

  it("refuses to publish an executable that still exposes a source path", () => {
    const paths = fixture();
    const existing = join(paths.output, platform, "well/old.pkg");
    write(existing, "preserved");
    expect(() => buildWell({ ...paths, environment: {}, run: (directory: string, environment: NodeJS.ProcessEnv) => {
      expect(environment.CARGO_ENCODED_RUSTFLAGS).toContain("--remap-path-prefix=");
      fakeBuild(directory, environment);
      write(join(directory, "src-tauri/target/release", executable), `panic at ${paths.root}/well/src-tauri/src/main.rs`);
    } })).toThrow("local build path");
    expect(readFileSync(existing, "utf8")).toBe("preserved");
    expect(readdirSync(paths.output).filter((name) => name.startsWith(".release.tmp."))).toEqual([]);
  });

  it("accepts only local build options", () => {
    expect(parseOptions([])).toEqual({});
    expect(parseOptions(["--output", "custom"])).toEqual({ output: "custom" });
    expect(parseOptions(["--help"])).toEqual({ help: true });
    for (const argumentsList of [["all"], ["--all-platforms"], ["--output"], ["--output", "--help"]]) {
      expect(() => parseOptions(argumentsList)).toThrow();
    }
  });

  it.each(["target", "../cargo-output", "absolute"])("builds independently with Cargo target directory %s", (target) => {
    const paths = fixture();
    const environment = { CARGO_TARGET_DIR: target === "absolute" ? join(paths.root, "absolute-output") : target };
    const profile = resolve(paths.directory, "src-tauri", environment.CARGO_TARGET_DIR, "release");
    write(join(profile, "bundle/stale.pkg"), "old bundle");
    const preserved = [join(paths.output, platform, "norns"), join(paths.output, "other/well/old.pkg")];
    for (const filename of preserved) write(filename, "preserved");
    write(join(paths.output, platform, "well/bundle/stale.pkg"), "old published bundle");
    buildWell({ ...paths, environment, run: fakeBuild });
    for (const filename of preserved) expect(readFileSync(filename, "utf8")).toBe("preserved");
    const artifact = join(paths.output, platform, "well");
    expect(readFileSync(join(artifact, executable), "utf8")).toBe("new Well");
    expect(readFileSync(join(artifact, "bundle/package/Well.pkg"), "utf8")).toBe("new bundle");
    expect(existsSync(join(artifact, "bundle/stale.pkg"))).toBe(false);
    expect(existsSync(join(artifact, "bundle/package/bundle_dmg.sh"))).toBe(false);
    expect(existsSync(join(artifact, "bundle/share"))).toBe(false);
    if (process.platform !== "win32") {
      expect(lstatSync(join(artifact, "bundle/macos/Well.app/Contents/current")).isSymbolicLink()).toBe(true);
      expect(readlinkSync(join(artifact, "bundle/macos/Well.app/Contents/current"))).toBe("executable");
    }
    expect(readFileSync(join(paths.output, "SHA256SUMS"), "utf8")).toContain(createHash("sha256").update("preserved").digest("hex"));
    expect(readdirSync(paths.root).filter((name) => name.startsWith(".release.tmp."))).toEqual([]);
    expect(readdirSync(paths.output).filter((name) => name.startsWith(".release.tmp."))).toEqual([]);
    expect(existsSync(join(paths.root, "norns"))).toBe(false);
    expect(existsSync(join(paths.root, "nornsd"))).toBe(false);
  });

  it("stages inside output and excludes current and stale staging from checksums", () => {
    const paths = fixture();
    const output = join(paths.root, "custom release");
    const stale = join(output, ".release.tmp.stale", "partial");
    write(stale, "unfinished build");
    buildWell({ directory: paths.directory, output, environment: {}, run: (directory: string, environment: NodeJS.ProcessEnv) => {
      expect(readdirSync(output).filter((name) => name.startsWith(".release.tmp."))).toHaveLength(2);
      expect(readdirSync(paths.root).filter((name) => name.startsWith(".release.tmp."))).toEqual([]);
      fakeBuild(directory, environment);
    } });
    expect(readdirSync(output).filter((name) => name.startsWith(".release.tmp."))).toEqual([".release.tmp.stale"]);
    expect(readFileSync(join(output, "SHA256SUMS"), "utf8")).not.toContain(".release.tmp.");
    expect(readFileSync(stale, "utf8")).toBe("unfinished build");
  });

  it("supports a custom output directory without touching default release", () => {
    const paths = fixture();
    const output = join(paths.root, "custom");
    buildWell({ directory: paths.directory, output, environment: {}, run: fakeBuild });
    expect(existsSync(join(output, platform, "well", executable))).toBe(true);
    expect(existsSync(paths.output)).toBe(false);
  });

  it.each(["build", "bundle", "version"])("preserves published artifacts on %s failure", (failure) => {
    const paths = fixture();
    const existing = join(paths.output, platform, "well/old.pkg");
    write(existing, "preserved");
    if (failure === "version") write(join(paths.root, "versions.json"), JSON.stringify({ well: { version: "dev" } }));
    expect(() => buildWell({
      ...paths,
      environment: {},
      run: (directory: string, environment: NodeJS.ProcessEnv) => {
        if (failure === "build") throw new Error("build failed");
        fakeBuild(directory, environment);
        rmSync(join(directory, "src-tauri/target/release/bundle"), { recursive: true });
      },
    })).toThrow();
    expect(readFileSync(existing, "utf8")).toBe("preserved");
    expect(readdirSync(paths.root).filter((name) => name.startsWith(".release.tmp."))).toEqual([]);
    expect(readdirSync(paths.output).filter((name) => name.startsWith(".release.tmp."))).toEqual([]);
  });

  it.skipIf(process.platform === "win32")("rejects symlink output before building", () => {
    for (const platformLink of [false, true]) {
      const paths = fixture();
      const outside = join(paths.root, "outside");
      mkdirSync(outside);
      const link = platformLink ? join(paths.output, platform) : paths.output;
      mkdirSync(dirname(link), { recursive: true });
      symlinkSync(outside, link);
      expect(() => buildWell({ ...paths, run: () => { throw new Error("must not build"); } })).toThrow("symbolic link");
    }
  });
});
