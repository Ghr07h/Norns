package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func coordinatorFixture(testingContext *testing.T) string {
	testingContext.Helper()
	if runtime.GOOS == "windows" {
		testingContext.Skip("root entry requires Bash")
	}
	root := testingContext.TempDir()
	_, source, _, _ := runtime.Caller(0)
	entry, err := os.ReadFile(filepath.Join(filepath.Dir(source), "../../release.sh"))
	if err != nil {
		testingContext.Fatal(err)
	}
	writeFixture(testingContext, filepath.Join(root, "release.sh"), string(entry))
	for _, project := range []string{"norns", "nornsd", "well"} {
		writeFixture(testingContext, filepath.Join(root, project, ".fixture"), "")
	}
	if err := os.MkdirAll(filepath.Join(root, "well/src-tauri"), 0o755); err != nil {
		testingContext.Fatal(err)
	}
	writeFixture(testingContext, filepath.Join(root, "nornsd/configs/nornsd/nornsd.example.yaml"), "AGENT_NORNS_ENABLE: true\n")
	for _, name := range []string{"websearch", "lookup"} {
		writeFixture(testingContext, filepath.Join(root, "norns/internal/custom_extensions", name, "cmd", "norns-"+name, "main.go"), "package main\n")
	}
	tool := `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == env ]]; then
  case "$2" in
    GOVERSION) echo go1.25.0 ;;
    GOHOSTOS) echo linux ;;
    GOHOSTARCH) echo amd64 ;;
    *) exit 1 ;;
  esac
  exit 0
fi
if [[ "$(basename "$0")" == node ]]; then
  if [[ "$1" == */scripts/gitrelease.mjs ]]; then
    echo "gitrelease:$*" >> "$FAKE_BUILD_LOG"
    exit 0
  fi
  case "$2" in
    *process.platform*) echo linux ;;
    *process.arch*) echo x64 ;;
    *) exit 1 ;;
  esac
  exit 0
fi
project=$(basename "$PWD")
if [[ "$*" == *--publish-from* ]]; then
  source=""
  output=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --publish-from) shift; source=$1 ;;
      --output) shift; output=$1 ;;
    esac
    shift
  done
  for platform_dir in "$source"/*; do
    [[ -d "$platform_dir" ]] || continue
    destination="$output/$(basename "$platform_dir")"
    mkdir -p "$destination"
    rm -rf -- "$destination/norns" "$destination/norns.exe" "$destination/norns.exts.meta.json" "$destination"/norns-*
    for artifact in "$platform_dir/norns" "$platform_dir/norns.exe" "$platform_dir/norns.exts.meta.json" "$platform_dir"/norns-*; do
      [[ -e "$artifact" ]] || continue
      mv -- "$artifact" "$destination/"
    done
  done
  exit 0
fi
echo "$project:$*" >> "$FAKE_BUILD_LOG"
[[ "${FAKE_FAIL_PRODUCT:-}" != "$project" ]] || exit 1
all=false
extensions=()
output=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output) shift; output=$1 ;;
    --all-platforms) all=true ;;
    --with-ext) shift; extensions+=("$1") ;;
  esac
  shift
done
[[ -n "$output" ]]
platforms=(linux-x64)
if [[ "$all" == true ]]; then
  platforms=(darwin-x64 darwin-arm64 linux-x64 linux-arm64 windows-x64 windows-arm64)
fi
for platform in "${platforms[@]}"; do
  mkdir -p "$output/$platform"
  case "$project" in
    norns)
      printf new > "$output/$platform/norns"
      printf '[]\n' > "$output/$platform/norns.exts.meta.json"
      for extension in ${extensions[@]+"${extensions[@]}"}; do printf new > "$output/$platform/norns-$extension"; done
      ;;
    nornsd) printf new > "$output/$platform/nornsd"; cp configs/nornsd/nornsd.example.yaml "$output/$platform/nornsd.example.yaml" ;;
    well) mkdir -p "$output/$platform/well/bundle"; printf new > "$output/$platform/well/bundle/Well.pkg" ;;
  esac
done
`
	for _, name := range []string{"go", "node", "npm"} {
		writeFixture(testingContext, filepath.Join(root, "tools", name), tool)
	}
	testingContext.Setenv("PATH", filepath.Join(root, "tools")+string(os.PathListSeparator)+os.Getenv("PATH"))
	testingContext.Setenv("FAKE_BUILD_LOG", filepath.Join(root, "build.log"))
	testingContext.Setenv("FAKE_FAIL_PRODUCT", "")
	testingContext.Setenv("CARGO_TARGET_DIR", "")
	return root
}

func runCoordinator(root string, arguments ...string) ([]byte, error) {
	command := exec.Command("bash", append([]string{filepath.Join(root, "release.sh")}, arguments...)...)
	return command.CombinedOutput()
}

func TestCoordinatorProductSelection(testingContext *testing.T) {
	for _, test := range []struct {
		arguments []string
		products  []string
	}{
		{nil, []string{"norns", "nornsd"}},
		{[]string{"norns"}, []string{"norns"}},
		{[]string{"nornsd"}, []string{"nornsd"}},
		{[]string{"well"}, []string{"well"}},
		{[]string{"all"}, []string{"norns", "nornsd", "well"}},
	} {
		testingContext.Run(strings.Join(test.arguments, " "), func(testingContext *testing.T) {
			root := coordinatorFixture(testingContext)
			output, err := runCoordinator(root, test.arguments...)
			if err != nil {
				testingContext.Fatalf("entry failed: %v\n%s", err, output)
			}
			log, err := os.ReadFile(filepath.Join(root, "build.log"))
			if err != nil {
				testingContext.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(string(log)), "\n")
			if len(lines) != len(test.products) {
				testingContext.Fatalf("unexpected dispatch: %s", log)
			}
			for index, project := range test.products {
				prefix := project + ":run ./scripts/release.go "
				if project == "well" {
					prefix = "well:run release -- "
				}
				if !strings.HasPrefix(lines[index], prefix) {
					testingContext.Fatalf("unexpected dispatch: %s", lines[index])
				}
			}
			checksums, err := os.ReadFile(filepath.Join(root, "release/SHA256SUMS"))
			if err != nil || len(checksums) == 0 {
				testingContext.Fatalf("missing checksums: %v", err)
			}
			if strings.Contains(string(checksums), "norns-websearch") {
				testingContext.Fatal("default build included an extension")
			}
			example := filepath.Join(root, "release/linux-x64/nornsd.example.yaml")
			if selection := strings.Join(test.arguments, " "); selection == "" || selection == "nornsd" || selection == "all" {
				if contents, err := os.ReadFile(example); err != nil || string(contents) != "AGENT_NORNS_ENABLE: true\n" {
					testingContext.Fatalf("missing published nornsd example: %q %v", contents, err)
				}
				if !strings.Contains(string(checksums), "nornsd.example.yaml") {
					testingContext.Fatal("nornsd example missing from checksums")
				}
			} else if _, err := os.Stat(example); !os.IsNotExist(err) {
				testingContext.Fatalf("unselected nornsd published a nornsd example: %v", err)
			}
		})
	}
}

func TestCoordinatorAllPlatforms(testingContext *testing.T) {
	root := coordinatorFixture(testingContext)
	if output, err := runCoordinator(root, "--all-platforms"); err != nil {
		testingContext.Fatalf("entry failed: %v\n%s", err, output)
	}
	for _, platform := range []string{"darwin-x64", "darwin-arm64", "linux-x64", "linux-arm64", "windows-x64", "windows-arm64"} {
		for _, project := range []string{"norns", "nornsd", "nornsd.example.yaml"} {
			if _, err := os.Stat(filepath.Join(root, "release", platform, project)); err != nil {
				testingContext.Fatal(err)
			}
		}
	}
}

func TestCoordinatorSelectedExtensions(testingContext *testing.T) {
	for _, arguments := range [][]string{
		{"--with-ext", "websearch"},
		{"norns", "--with-ext", "websearch", "--with-ext", "lookup"},
		{"--with-ext", "websearch", "all"},
		{"--all-platforms", "--with-ext", "websearch", "--with-ext", "lookup"},
	} {
		testingContext.Run(strings.Join(arguments, " "), func(testingContext *testing.T) {
			root := coordinatorFixture(testingContext)
			if output, err := runCoordinator(root, arguments...); err != nil {
				testingContext.Fatalf("release failed: %v\n%s", err, output)
			}
			log, err := os.ReadFile(filepath.Join(root, "build.log"))
			if err != nil {
				testingContext.Fatal(err)
			}
			for _, line := range strings.Split(strings.TrimSpace(string(log)), "\n") {
				if !strings.HasPrefix(line, "norns:") && strings.Contains(line, " --with-ext ") {
					testingContext.Fatalf("extension flags forwarded to another product: %s", line)
				}
			}
			checksums, err := os.ReadFile(filepath.Join(root, "release/SHA256SUMS"))
			if err != nil {
				testingContext.Fatal(err)
			}
			expected := 1
			if strings.Contains(strings.Join(arguments, " "), "--all-platforms") {
				expected = 6
			}
			if strings.Count(string(checksums), "/norns-websearch\n") != expected {
				testingContext.Fatalf("selected extension missing from platforms/checksums: %s", checksums)
			}
			if strings.Contains(strings.Join(arguments, " "), "--with-ext lookup") {
				if strings.Count(string(checksums), "/norns-lookup\n") != expected {
					testingContext.Fatalf("second extension missing: %s", checksums)
				}
			} else if strings.Contains(string(checksums), "norns-lookup") {
				testingContext.Fatal("unselected extension was built")
			}
		})
	}
}

func TestCoordinatorPreservesUnselectedArtifacts(testingContext *testing.T) {
	root := coordinatorFixture(testingContext)
	output := filepath.Join(root, "release/linux-x64")
	preserved := []string{filepath.Join(output, "nornsd"), filepath.Join(output, "nornsd.example.yaml"), filepath.Join(output, "nornsd.yaml"), filepath.Join(output, "agent.example.yaml"), filepath.Join(output, "agent.yaml"), filepath.Join(output, "well/bundle/old.pkg"), filepath.Join(root, "release/other/norns")}
	for _, filename := range preserved {
		writeFixture(testingContext, filename, "preserved")
	}
	writeFixture(testingContext, filepath.Join(output, "norns-stale"), "stale")
	if output, err := runCoordinator(root, "norns"); err != nil {
		testingContext.Fatalf("entry failed: %v\n%s", err, output)
	}
	for _, filename := range preserved {
		contents, err := os.ReadFile(filename)
		if err != nil || string(contents) != "preserved" {
			testingContext.Fatalf("unselected artifact changed: %s %v", filename, err)
		}
	}
	if _, err := os.Stat(filepath.Join(output, "norns-stale")); !os.IsNotExist(err) {
		testingContext.Fatal("stale plugin was not removed")
	}
}

func TestCoordinatorRefreshesNornsdExample(testingContext *testing.T) {
	root := coordinatorFixture(testingContext)
	destination := filepath.Join(root, "release/linux-x64")
	example := filepath.Join(destination, "nornsd.example.yaml")
	legacy := filepath.Join(destination, "agent.example.yaml")
	config := filepath.Join(destination, "nornsd.yaml")
	writeFixture(testingContext, filepath.Join(example, "stale.yaml"), "stale")
	writeFixture(testingContext, legacy, "old example")
	writeFixture(testingContext, config, "user configuration")
	if output, err := runCoordinator(root, "nornsd"); err != nil {
		testingContext.Fatalf("release failed: %v\n%s", err, output)
	}
	if contents, err := os.ReadFile(example); err != nil || string(contents) != "AGENT_NORNS_ENABLE: true\n" {
		testingContext.Fatalf("nornsd example was not replaced: %q %v", contents, err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		testingContext.Fatalf("legacy example was not removed: %v", err)
	}
	if contents, err := os.ReadFile(config); err != nil || string(contents) != "user configuration" {
		testingContext.Fatalf("user configuration changed: %q %v", contents, err)
	}
}

func TestCoordinatorFailurePreservesRelease(testingContext *testing.T) {
	root := coordinatorFixture(testingContext)
	filename := filepath.Join(root, "release/linux-x64/norns")
	writeFixture(testingContext, filename, "preserved")
	testingContext.Setenv("FAKE_FAIL_PRODUCT", "nornsd")
	if output, err := runCoordinator(root); err == nil {
		testingContext.Fatalf("accepted failed build: %s", output)
	}
	contents, err := os.ReadFile(filename)
	if err != nil || string(contents) != "preserved" {
		testingContext.Fatalf("published incomplete build: %s %v", contents, err)
	}
	staging, err := filepath.Glob(filepath.Join(root, "release", ".release.tmp.*"))
	if err != nil || len(staging) != 0 {
		testingContext.Fatalf("staging leaked: %v %v", staging, err)
	}
}

func TestCoordinatorWellBundleCleanup(testingContext *testing.T) {
	for _, selection := range []string{"", "norns", "nornsd", "well", "all"} {
		for _, targetDirectory := range []string{"", "custom target", "absolute"} {
			testingContext.Run(selection+"/"+targetDirectory, func(testingContext *testing.T) {
				root := coordinatorFixture(testingContext)
				if targetDirectory == "absolute" {
					targetDirectory = testingContext.TempDir()
				}
				testingContext.Setenv("CARGO_TARGET_DIR", targetDirectory)
				resolvedTarget := targetDirectory
				if resolvedTarget == "" {
					resolvedTarget = "target"
				}
				if !filepath.IsAbs(resolvedTarget) {
					resolvedTarget = filepath.Join(root, "well/src-tauri", resolvedTarget)
				}
				bundle := filepath.Join(resolvedTarget, "release/bundle")
				writeFixture(testingContext, filepath.Join(bundle, "macos/Well.app/Contents/executable"), "source app")
				preserved := []string{
					filepath.Join(resolvedTarget, "release/well"),
					filepath.Join(resolvedTarget, "release/deps/cache"),
					filepath.Join(resolvedTarget, "debug/bundle/macos/Well.app/Contents/executable"),
				}
				for _, filename := range preserved {
					writeFixture(testingContext, filename, "preserved")
				}
				var arguments []string
				if selection != "" {
					arguments = append(arguments, selection)
				}
				if output, err := runCoordinator(root, arguments...); err != nil {
					testingContext.Fatalf("release failed: %v\n%s", err, output)
				}
				_, err := os.Stat(bundle)
				if selection == "well" || selection == "all" {
					if !os.IsNotExist(err) {
						testingContext.Fatalf("source bundle was not removed: %v", err)
					}
					published := filepath.Join(root, "release/linux-x64/well/bundle/Well.pkg")
					if contents, err := os.ReadFile(published); err != nil || string(contents) != "new" {
						testingContext.Fatalf("published bundle was not preserved: %q %v", contents, err)
					}
				} else if err != nil {
					testingContext.Fatalf("unselected Well bundle was removed: %v", err)
				}
				for _, filename := range preserved {
					if contents, err := os.ReadFile(filename); err != nil || string(contents) != "preserved" {
						testingContext.Fatalf("non-bundle artifact changed: %s %v", filename, err)
					}
				}
			})
		}
	}
}

func TestCoordinatorFailedWellReleasePreservesSourceBundle(testingContext *testing.T) {
	for _, failure := range []string{"build", "checksum"} {
		testingContext.Run(failure, func(testingContext *testing.T) {
			root := coordinatorFixture(testingContext)
			filename := filepath.Join(root, "well/src-tauri/target/release/bundle/macos/Well.app/Contents/executable")
			writeFixture(testingContext, filename, "source app")
			if failure == "build" {
				testingContext.Setenv("FAKE_FAIL_PRODUCT", "well")
			} else {
				writeFixture(testingContext, filepath.Join(root, "tools/sha256sum"), "#!/usr/bin/env bash\nexit 1\n")
			}
			if output, err := runCoordinator(root, "well"); err == nil {
				testingContext.Fatalf("accepted failed release: %s", output)
			}
			if contents, err := os.ReadFile(filename); err != nil || string(contents) != "source app" {
				testingContext.Fatalf("source bundle removed after failed release: %q %v", contents, err)
			}
		})
	}
}

func TestCoordinatorRejectsInvalidRequests(testingContext *testing.T) {
	for _, arguments := range [][]string{{"unknown"}, {"norns", "nornsd"}, {"well", "--all-platforms"}, {"all", "--all-platforms"}, {"--with-ext"}, {"--with-ext", ""}, {"--with-ext", "--all-platforms"}, {"--with-ext", "../websearch"}, {"--with-ext", "missing"}, {"nornsd", "--with-ext", "websearch"}, {"well", "--with-ext", "websearch"}} {
		root := coordinatorFixture(testingContext)
		if output, err := runCoordinator(root, arguments...); err == nil {
			testingContext.Fatalf("accepted %v: %s", arguments, output)
		}
		if _, err := os.Stat(filepath.Join(root, "build.log")); !os.IsNotExist(err) {
			testingContext.Fatal("invalid request ran build commands")
		}
	}
	for _, platformLink := range []bool{false, true} {
		root := coordinatorFixture(testingContext)
		filename := filepath.Join(root, "release")
		if platformLink {
			filename = filepath.Join(filename, "linux-x64")
			if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
				testingContext.Fatal(err)
			}
		}
		if err := os.Symlink(testingContext.TempDir(), filename); err != nil {
			testingContext.Fatal(err)
		}
		if output, err := runCoordinator(root); err == nil {
			testingContext.Fatalf("accepted symlink output: %s", output)
		}
	}
}

func TestCoordinatorStagingInsideReleaseExcludedFromChecksums(t *testing.T) {
	root := coordinatorFixture(t)
	stale := filepath.Join(root, "release", ".release.tmp.stale", "partial")
	writeFixture(t, stale, "unfinished build")
	if output, err := runCoordinator(root); err != nil {
		t.Fatalf("release failed: %v\n%s", err, output)
	}
	log, err := os.ReadFile(filepath.Join(root, "build.log"))
	if err != nil || !strings.Contains(string(log), "--output "+filepath.Join(root, "release", ".release.tmp.")) {
		t.Fatalf("staging not inside release: %s %v", log, err)
	}
	stages, err := filepath.Glob(filepath.Join(root, "release", ".release.tmp.*"))
	if err != nil || len(stages) != 1 || stages[0] != filepath.Dir(stale) {
		t.Fatalf("current staging not cleaned: %v %v", stages, err)
	}
	roots, err := filepath.Glob(filepath.Join(root, ".release.tmp.*"))
	if err != nil || len(roots) != 0 {
		t.Fatalf("staging created at repository root: %v %v", roots, err)
	}
	contents, err := os.ReadFile(filepath.Join(root, "release", "SHA256SUMS"))
	if err != nil || len(contents) == 0 || strings.Contains(string(contents), ".release.tmp.") {
		t.Fatalf("invalid checksums: %s %v", contents, err)
	}
}

func TestCoordinatorGitReleaseFixedSelection(t *testing.T) {
	root := coordinatorFixture(t)
	output, err := runCoordinator(root, "well", "--all-platforms", "--gitrelease")
	if err != nil {
		t.Fatalf("gitrelease dispatch: %v\n%s", err, output)
	}
	log, err := os.ReadFile(filepath.Join(root, "build.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(log) != "gitrelease:"+filepath.Join(root, "scripts/gitrelease.mjs")+"\n" {
		t.Fatalf("unexpected dispatch: %s", log)
	}
	if _, err := os.Stat(filepath.Join(root, "release")); !os.IsNotExist(err) {
		t.Fatalf("normal release was touched: %v", err)
	}
}

func TestCoordinatorGitReleaseForwardsExtensions(t *testing.T) {
	root := coordinatorFixture(t)
	output, err := runCoordinator(root, "--gitrelease", "--with-ext", "websearch", "--with-ext", "lookup")
	if err != nil {
		t.Fatalf("gitrelease dispatch: %v\n%s", err, output)
	}
	log, err := os.ReadFile(filepath.Join(root, "build.log"))
	if err != nil {
		t.Fatal(err)
	}
	want := "gitrelease:" + filepath.Join(root, "scripts/gitrelease.mjs") + " --with-ext websearch --with-ext lookup\n"
	if string(log) != want {
		t.Fatalf("unexpected dispatch: %s", log)
	}
}
