import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { buildGitRelease, parseGitReleaseOptions } from "../../scripts/gitrelease.mjs";

const directories: string[] = [];
const host = { platform: "darwin", arch: "arm64" };
const platforms = ["darwin-arm64", "windows-x64", "linux-x64"];

function write(filename: string, contents: string) {
  mkdirSync(dirname(filename), { recursive: true });
  writeFileSync(filename, contents);
}

function fixture() {
  const root = mkdtempSync(join(tmpdir(), "github release "));
  directories.push(root);
  write(join(root, "versions.json"), JSON.stringify({ norns: { version: "1.2.3" }, nornsd: { version: "2.3.4" }, well: { version: "3.4.5" } }));
  write(join(root, "LICENSE"), "license");
  write(join(root, "norns/THIRD_PARTY_NOTICES.md"), "notices");
  for (const name of ["websearch", "deepseeksearch"]) {
    write(join(root, "norns/internal/custom_extensions", name, "cmd", `norns-${name}`, "main.go"), "package main");
  }
  return { root, host, environment: {}, run: fakeRun };
}

function fakeRun(command: string, args: string[], options: { cwd: string }) {
  if (command === "zip") {
    execFileSync(command, args, { cwd: options.cwd });
    return;
  }
  const output = args[args.indexOf("--output") + 1];
  const product = basename(options.cwd);
  if (command === "go") {
    expect(args.filter((arg) => arg === "--target")).toHaveLength(3);
    const extensions = args.flatMap((argument, index) => argument === "--with-ext" ? [args[index + 1]] : []);
    if (product !== "norns") expect(extensions).toEqual([]);
    for (const platform of platforms) {
      write(join(output, platform, product + (platform.startsWith("windows-") ? ".exe" : "")), `${product} binary`);
      write(join(output, platform, "nornsd.example.yaml"), "safe example");
      write(join(output, platform, "nornsd.yaml"), "must not publish");
      write(join(output, platform, "norns-private-extension"), "must not publish");
      if (product === "norns") {
        write(join(output, platform, "norns.exts.meta.json"), JSON.stringify(extensions.map((name) => ({
          name, version: "0.1.0", description: name, requirements: [],
          release_files: name === "deepseeksearch" ? [
            { origin_file: "config.example.yaml", target_file: "" },
            { origin_file: "docs/usage.txt", target_file: "guide.txt" },
          ] : [],
        }))));
        for (const name of extensions) write(join(output, platform, `norns-${name}${platform.startsWith("windows-") ? ".exe" : ""}`), `${name} binary`);
        write(join(output, platform, "norns.exts.deepseeksearch.config.example.yaml"), "public template");
        write(join(output, platform, "norns.exts.deepseeksearch.guide.txt"), "public guide");
        write(join(output, platform, "norns.exts.deepseeksearch.private.yaml"), "must not publish");
      }
    }
  } else {
    expect(command).toBe("npm");
    expect(product).toBe("well");
    write(join(output, "darwin-arm64/well/bundle/dmg/Well_3.4.5_aarch64.dmg"), "fresh DMG");
    write(join(options.cwd, "src-tauri/target/release/bundle/macos/Well.app/file"), "duplicate bundle");
  }
}

afterEach(() => {
  for (const directory of directories.splice(0)) rmSync(directory, { recursive: true, force: true });
});

describe("GitHub release attachments", () => {
  it("parses repeated extension selections and rejects missing names and unknown options", () => {
    expect(parseGitReleaseOptions([])).toEqual({ extensions: [] });
    expect(parseGitReleaseOptions(["--with-ext", "websearch", "--with-ext", "deepseeksearch", "--with-ext", "websearch"])).toEqual({ extensions: ["websearch", "deepseeksearch"] });
    for (const args of [["--with-ext"], ["--with-ext", ""], ["--with-ext", "../secret"], ["--with-ext", "--output"], ["--with-exts", "websearch"]]) {
      expect(() => parseGitReleaseOptions(args)).toThrow();
    }
  });

  it("packages selected extension binaries, metadata and declared files in all Norns ZIPs", () => {
    const options = fixture();
    const files = buildGitRelease({ ...options, extensions: ["websearch", "deepseeksearch", "websearch"] });
    for (const filename of files.filter((name: string) => name.endsWith(".zip"))) {
      const archive = join(options.root, "release/gitrelease", filename);
      const entries = execFileSync("unzip", ["-Z1", archive], { encoding: "utf8" }).trim().split("\n").sort();
      const windows = filename.includes("windows-");
      const suffix = windows ? ".exe" : "";
      if (filename.startsWith("norns-")) {
        expect(entries).toEqual([
          "LICENSE", "THIRD_PARTY_NOTICES.md", `norns${suffix}`, `norns-websearch${suffix}`, `norns-deepseeksearch${suffix}`,
          "norns.exts.meta.json", "norns.exts.deepseeksearch.config.example.yaml", "norns.exts.deepseeksearch.guide.txt",
        ].sort());
        expect(execFileSync("unzip", ["-p", archive, "norns.exts.deepseeksearch.config.example.yaml"], { encoding: "utf8" })).toBe("public template");
      } else {
        expect(entries).toEqual(["LICENSE", `nornsd${suffix}`, "nornsd.example.yaml"].sort());
      }
    }
    buildGitRelease(options);
    const archive = join(options.root, "release/gitrelease/norns-v1.2.3-linux-amd64.zip");
    expect(execFileSync("unzip", ["-Z1", archive], { encoding: "utf8" })).not.toContain("norns.exts.");
  });

  it.each(["../private", "missing"])("rejects extension %s before starting builds", (name) => {
    const options = fixture();
    expect(() => buildGitRelease({ ...options, extensions: [name], run: () => { throw new Error("must not build"); } })).toThrow(/invalid extension|unknown extension/);
    expect(existsSync(join(options.root, "release"))).toBe(false);
  });

  it.each(["missing", "symlink", "metadata", "unsafe-target"])("preserves old release when extension attachment has %s failure", (failure) => {
    const options = fixture();
    write(join(options.root, "release/gitrelease/old.zip"), "preserved");
    expect(() => buildGitRelease({ ...options, extensions: ["deepseeksearch"], run: (command: string, args: string[], runOptions: { cwd: string }) => {
      fakeRun(command, args, runOptions);
      if (command !== "go" || basename(runOptions.cwd) !== "norns") return;
      const output = args[args.indexOf("--output") + 1];
      const directory = join(output, "darwin-arm64");
      const attachment = join(directory, "norns.exts.deepseeksearch.config.example.yaml");
      if (failure === "missing" || failure === "symlink") rmSync(attachment);
      if (failure === "symlink") symlinkSync(join(directory, "nornsd.yaml"), attachment);
      if (failure === "metadata") write(join(directory, "norns.exts.meta.json"), "[]");
      if (failure === "unsafe-target") write(join(directory, "norns.exts.meta.json"), JSON.stringify([{ name: "deepseeksearch", release_files: [{ origin_file: "config.example.yaml", target_file: "../private" }] }]));
    } })).toThrow();
    expect(readFileSync(join(options.root, "release/gitrelease/old.zip"), "utf8")).toBe("preserved");
    expect(readdirSync(join(options.root, "release")).filter((name) => name.startsWith(".release.tmp."))).toEqual([]);
  });

  it("publishes exactly six independent product ZIPs and a raw DMG, replacing stale attachments", () => {
    const options = fixture();
    const output = join(options.root, "release");
    write(join(output, "gitrelease/stale.zip"), "old");
    write(join(output, "darwin-arm64/norns"), "preserved normal release");
    const files = buildGitRelease(options);
    expect(files).toEqual([
      "norns-v1.2.3-darwin-arm64.zip", "norns-v1.2.3-linux-amd64.zip", "norns-v1.2.3-windows-amd64.zip",
      "nornsd-v2.3.4-darwin-arm64.zip", "nornsd-v2.3.4-linux-amd64.zip", "nornsd-v2.3.4-windows-amd64.zip",
      "well-v3.4.5-darwin-arm64.dmg",
    ]);
    for (const filename of files.filter((name: string) => name.endsWith(".zip"))) {
      const entries = execFileSync("unzip", ["-Z1", join(output, "gitrelease", filename)], { encoding: "utf8" }).trim().split("\n");
      const product = filename.startsWith("nornsd-") ? "nornsd" : "norns";
      expect(entries.sort()).toEqual(["LICENSE", product + (filename.includes("windows-") ? ".exe" : ""), product === "norns" ? "THIRD_PARTY_NOTICES.md" : "nornsd.example.yaml"].sort());
    }
    expect(readFileSync(join(output, "gitrelease/well-v3.4.5-darwin-arm64.dmg"), "utf8")).toBe("fresh DMG");
    expect(readFileSync(join(output, "darwin-arm64/norns"), "utf8")).toBe("preserved normal release");
    expect(existsSync(join(options.root, "well/src-tauri/target/release/bundle"))).toBe(false);
    expect(readdirSync(output).filter((name) => name.startsWith(".release.tmp."))).toEqual([]);
  });

  it.each(["nornsd", "well", "zip", "dmg"])("preserves the old release and cleans staging on %s failure", (failure) => {
    const options = fixture();
    const output = join(options.root, "release");
    write(join(output, "gitrelease/old.zip"), "preserved");
    expect(() => buildGitRelease({ ...options, run: (command: string, args: string[], runOptions: { cwd: string }) => {
      if (basename(runOptions.cwd) === failure || command === failure) throw new Error("build failure");
      fakeRun(command, args, runOptions);
      if (failure === "dmg" && command === "npm") rmSync(join(args[args.indexOf("--output") + 1], "darwin-arm64/well/bundle/dmg"), { recursive: true });
    } })).toThrow();
    expect(readdirSync(join(output, "gitrelease"))).toEqual(["old.zip"]);
    expect(readFileSync(join(output, "gitrelease/old.zip"), "utf8")).toBe("preserved");
    expect(readdirSync(output).filter((name) => name.startsWith(".release.tmp."))).toEqual([]);
  });

  it("rejects unsupported hosts and invalid versions before building", () => {
    const options = fixture();
    for (const host of [{ platform: "linux", arch: "arm64" }, { platform: "darwin", arch: "x64" }]) {
      expect(() => buildGitRelease({ ...options, host })).toThrow("macOS ARM64");
    }
    write(join(options.root, "versions.json"), '{"norns":{"version":"../unsafe"}}');
    expect(() => buildGitRelease(options)).toThrow("three-part version");
    expect(existsSync(join(options.root, "release"))).toBe(false);
  });

  it.skipIf(process.platform === "win32")("refuses a symlink release destination", () => {
    const options = fixture();
    const outside = join(options.root, "outside");
    mkdirSync(outside);
    mkdirSync(join(options.root, "release"));
    symlinkSync(outside, join(options.root, "release/gitrelease"));
    expect(() => buildGitRelease(options)).toThrow("symbolic link");
    expect(readdirSync(outside)).toEqual([]);
  });
});
