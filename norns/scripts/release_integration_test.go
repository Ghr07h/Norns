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
	writeFixture(testingContext, filepath.Join(root, "nornsd/configs/nornsd/agent.example.yaml"), "VERSION: 45\n")
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
  case "$2" in
    *process.platform*) echo linux ;;
    *process.arch*) echo x64 ;;
    *) exit 1 ;;
  esac
  exit 0
fi
project=$(basename "$PWD")
echo "$project:$*" >> "$FAKE_BUILD_LOG"
[[ "${FAKE_FAIL_PRODUCT:-}" != "$project" ]] || exit 1
all=false
output=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output) shift; output=$1 ;;
    --all-platforms) all=true ;;
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
    norns) printf new > "$output/$platform/norns"; printf new > "$output/$platform/norns-websearch" ;;
    nornsd) printf new > "$output/$platform/nornsd"; cp configs/nornsd/agent.example.yaml "$output/$platform/agent.example.yaml" ;;
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
			example := filepath.Join(root, "release/linux-x64/agent.example.yaml")
			if selection := strings.Join(test.arguments, " "); selection == "" || selection == "nornsd" || selection == "all" {
				if contents, err := os.ReadFile(example); err != nil || string(contents) != "VERSION: 45\n" {
					testingContext.Fatalf("missing published Agent example: %q %v", contents, err)
				}
				if !strings.Contains(string(checksums), "agent.example.yaml") {
					testingContext.Fatal("Agent example missing from checksums")
				}
			} else if _, err := os.Stat(example); !os.IsNotExist(err) {
				testingContext.Fatalf("unselected nornsd published an Agent example: %v", err)
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
		for _, project := range []string{"norns", "nornsd", "agent.example.yaml"} {
			if _, err := os.Stat(filepath.Join(root, "release", platform, project)); err != nil {
				testingContext.Fatal(err)
			}
		}
	}
}

func TestCoordinatorPreservesUnselectedArtifacts(testingContext *testing.T) {
	root := coordinatorFixture(testingContext)
	output := filepath.Join(root, "release/linux-x64")
	preserved := []string{filepath.Join(output, "nornsd"), filepath.Join(output, "agent.example.yaml"), filepath.Join(output, "agent.yaml"), filepath.Join(output, "well/bundle/old.pkg"), filepath.Join(root, "release/other/norns")}
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
	staging, err := filepath.Glob(filepath.Join(root, ".release.tmp.*"))
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
	for _, arguments := range [][]string{{"unknown"}, {"norns", "nornsd"}, {"well", "--all-platforms"}, {"all", "--all-platforms"}} {
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
