package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

type invocation struct {
	directory   string
	environment []string
	arguments   []string
}

func writeFixture(testingContext *testing.T, filename, contents string) {
	testingContext.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		testingContext.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(contents), 0o755); err != nil {
		testingContext.Fatal(err)
	}
}

func fixture(testingContext *testing.T) string {
	testingContext.Helper()
	root := testingContext.TempDir()
	_, source, _, _ := runtime.Caller(0)
	manifest, err := os.ReadFile(filepath.Join(filepath.Dir(source), "../../versions.json"))
	if err != nil {
		testingContext.Fatal(err)
	}
	writeFixture(testingContext, filepath.Join(root, "versions.json"), string(manifest))
	writeFixture(testingContext, filepath.Join(root, product, "go.mod"), "module example.test/"+product+"\n")
	writeFixture(testingContext, filepath.Join(root, product, "internal/version/versions_generated.go"), "package version\n")
	writeFixture(testingContext, filepath.Join(root, product, "cmd", product, "main.go"), "package main\n")
	if product == "norns" {
		for _, name := range []string{"websearch", "lookup"} {
			writeFixture(testingContext, filepath.Join(root, product, "internal/custom_extensions", name, "cmd", "norns-"+name, "main.go"), "package main\n")
			writeFixture(testingContext, filepath.Join(root, product, "internal/custom_extensions", name, "metadata.json"), fmt.Sprintf(`{"name":%q,"version":"0.1.0","description":"Test extension","requirements":[]}`, name))
		}
		writeFixture(testingContext, filepath.Join(root, product, "cmd/norns-legacy/main.go"), "package main\n")
		writeFixture(testingContext, filepath.Join(root, product, "cmd/unrelated/main.go"), "package main\n")
	}
	return root
}

func fakeRunner(testingContext *testing.T, calls *[]invocation) runCommand {
	testingContext.Helper()
	return func(directory string, environment []string, name string, arguments ...string) error {
		*calls = append(*calls, invocation{directory, environment, arguments})
		if filepath.Base(directory) != product || name != "go" {
			testingContext.Fatalf("unexpected command %s in %s", name, directory)
		}
		if !slices.Contains(environment, "CGO_ENABLED=0") {
			testingContext.Fatal("Go commands must disable CGO")
		}
		switch arguments[0] {
		case "build":
			output := arguments[slices.Index(arguments, "-o")+1]
			writeFixture(testingContext, output, "new "+filepath.Base(output))
		case "generate":
			if product != "nornsd" || strings.Join(arguments, " ") != "generate ./internal/assets" {
				testingContext.Fatalf("unexpected generation: %v", arguments)
			}
		default:
			testingContext.Fatalf("unexpected arguments: %v", arguments)
		}
		return nil
	}
}

func TestLocalBuild(testingContext *testing.T) {
	root := fixture(testingContext)
	var calls []invocation
	if err := build(root, options{}, fakeRunner(testingContext, &calls)); err != nil {
		testingContext.Fatal(err)
	}
	wantCalls := 1
	if len(calls) != wantCalls {
		testingContext.Fatalf("commands = %d, want %d", len(calls), wantCalls)
	}
	checksums, err := os.ReadFile(filepath.Join(root, "release/SHA256SUMS"))
	if err != nil || len(checksums) == 0 {
		testingContext.Fatalf("missing checksums: %v", err)
	}
	if strings.Contains(string(checksums), "norns-websearch") || strings.Contains(string(checksums), "norns-legacy") {
		testingContext.Fatalf("default build included extensions: %s", checksums)
	}
	for _, other := range []string{"norns", "nornsd", "well"} {
		if other == product {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, other)); !os.IsNotExist(err) {
			testingContext.Fatalf("build depends on sibling %s: %v", other, err)
		}
	}
}

func TestOptionsAndRootDiscovery(testingContext *testing.T) {
	for _, arguments := range [][]string{{"unknown"}, {"--unknown"}, {"norns"}, {"nornsd"}, {"--output"}, {"--output", "--all-platforms"}, {"--with-ext"}, {"--with-ext", "--all-platforms"}, {"--with-ext", ""}, {"--with-ext", "../websearch"}, {"--with-ext", "Websearch"}} {
		if _, err := parseOptions(arguments); err == nil {
			testingContext.Fatalf("accepted %v", arguments)
		}
	}
	selected, err := parseOptions([]string{"--all-platforms", "--with-ext", "websearch", "--output", "custom", "--with-ext", "lookup"})
	if err != nil || !selected.allPlatforms || selected.output != "custom" || !slices.Equal(selected.extensions, []string{"websearch", "lookup"}) {
		testingContext.Fatalf("options: %#v %v", selected, err)
	}
	if selected, err := parseOptions([]string{"--help"}); err != nil || !selected.help {
		testingContext.Fatalf("help: %#v %v", selected, err)
	}
	root := fixture(testingContext)
	testingContext.Chdir(filepath.Join(root, product, "scripts/.."))
	actual, err := findRoot()
	if err != nil {
		testingContext.Fatal(err)
	}
	actual, err = filepath.EvalSymlinks(actual)
	if err != nil {
		testingContext.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(root)
	if err != nil || actual != expected {
		testingContext.Fatalf("root = %q, want %q: %v", actual, expected, err)
	}
}

func TestAllGoPlatforms(testingContext *testing.T) {
	root := fixture(testingContext)
	var calls []invocation
	if err := build(root, options{allPlatforms: true}, fakeRunner(testingContext, &calls)); err != nil {
		testingContext.Fatal(err)
	}
	executables := []string{product}
	wantCalls := 6
	if len(calls) != wantCalls {
		testingContext.Fatalf("commands = %d, want %d", len(calls), wantCalls)
	}
	for _, platform := range []target{{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}} {
		for _, executable := range executables {
			if platform.os == "windows" {
				executable += ".exe"
			}
			if _, err := os.Stat(filepath.Join(root, "release", platform.directory(), executable)); err != nil {
				testingContext.Fatal(err)
			}
		}
	}
}

func TestSelectedExtensions(testingContext *testing.T) {
	for _, allPlatforms := range []bool{false, true} {
		for _, names := range [][]string{{"websearch"}, {"lookup", "websearch", "lookup"}} {
			testingContext.Run(fmt.Sprint(allPlatforms)+"/"+strings.Join(names, ","), func(testingContext *testing.T) {
				root := fixture(testingContext)
				var calls []invocation
				if err := build(root, options{allPlatforms: allPlatforms, extensions: names}, fakeRunner(testingContext, &calls)); err != nil {
					testingContext.Fatal(err)
				}
				packages := []string{"./cmd/norns"}
				for _, name := range names {
					pkg := "./internal/custom_extensions/" + name + "/cmd/norns-" + name
					if !slices.Contains(packages, pkg) {
						packages = append(packages, pkg)
					}
				}
				platformCount := 1
				if allPlatforms {
					platformCount = 6
				}
				if len(calls) != platformCount*len(packages) {
					testingContext.Fatalf("unexpected build commands: %#v", calls)
				}
				for index, call := range calls {
					if pkg := call.arguments[len(call.arguments)-1]; pkg != packages[index%len(packages)] {
						testingContext.Fatalf("built %q, want %q", pkg, packages[index%len(packages)])
					}
					output := call.arguments[slices.Index(call.arguments, "-o")+1]
					published := filepath.Join(root, "release", filepath.Base(filepath.Dir(output)), filepath.Base(output))
					if _, err := os.Stat(published); err != nil {
						testingContext.Fatalf("selected artifact not published: %s: %v", published, err)
					}
				}
			})
		}
	}
}

func TestInvalidExtensionPreservesRelease(testingContext *testing.T) {
	for _, name := range []string{"missing", "../websearch", "Websearch", ""} {
		testingContext.Run(name, func(testingContext *testing.T) {
			root := fixture(testingContext)
			filename := filepath.Join(root, "release", target{runtime.GOOS, runtime.GOARCH}.directory(), product)
			writeFixture(testingContext, filename, "preserved")
			if err := build(root, options{extensions: []string{name}}, func(string, []string, string, ...string) error {
				testingContext.Fatal("invalid extension ran a build")
				return nil
			}); err == nil {
				testingContext.Fatal("invalid extension accepted")
			}
			if contents, err := os.ReadFile(filename); err != nil || string(contents) != "preserved" {
				testingContext.Fatalf("release changed: %q %v", contents, err)
			}
		})
	}
}

func updateManifest(testingContext *testing.T, root string, change func(map[string]any)) {
	testingContext.Helper()
	filename := filepath.Join(root, "versions.json")
	source, err := os.ReadFile(filename)
	if err != nil {
		testingContext.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(source, &manifest); err != nil {
		testingContext.Fatal(err)
	}
	change(manifest)
	source, err = json.Marshal(manifest)
	if err != nil {
		testingContext.Fatal(err)
	}
	writeFixture(testingContext, filename, string(source))
}

func TestGeneratedVersionsFollowManifestWithoutRewriting(testingContext *testing.T) {
	root := fixture(testingContext)
	manifest, err := readVersions(root)
	if err != nil {
		testingContext.Fatal(err)
	}
	if err := generateVersions(root, manifest); err != nil {
		testingContext.Fatal(err)
	}
	filename := filepath.Join(root, product, "internal/version/versions_generated.go")
	oldTime := time.Unix(1, 0)
	if err := os.Chtimes(filename, oldTime, oldTime); err != nil {
		testingContext.Fatal(err)
	}
	if err := generateVersions(root, manifest); err != nil {
		testingContext.Fatal(err)
	}
	info, err := os.Stat(filename)
	if err != nil || !info.ModTime().Equal(oldTime) {
		testingContext.Fatalf("unchanged file was rewritten: %v", err)
	}
	updateManifest(testingContext, root, func(manifest map[string]any) {
		own := manifest[product].(map[string]any)
		own["version"] = "9.8.7"
		own["support_protocol"] = 42
		own["database_schema"] = 43
		if product == "nornsd" {
			own["api_schema"] = 44
			own["agent_config_schema"] = 45
			own["plugins"].(map[string]any)["cc"] = 46
		}
	})
	manifest, err = readVersions(root)
	if err != nil {
		testingContext.Fatal(err)
	}
	if err := generateVersions(root, manifest); err != nil {
		testingContext.Fatal(err)
	}
	contents, err := os.ReadFile(filename)
	if err != nil {
		testingContext.Fatal(err)
	}
	normalized := strings.Join(strings.Fields(string(contents)), " ")
	expected := []string{`ProductVersion = "9.8.7"`, "ProtocolVersion = 42", "SchemaVersion = 43"}
	if product == "nornsd" {
		expected = []string{`ProductVersion = "9.8.7"`, "NornsProtocol = 42", "DatabaseSchema = 43", "APISchema = 44", "AgentConfigVersion = 45", "CCPluginVersion = 46"}
	}
	for _, declaration := range expected {
		if !strings.Contains(normalized, declaration) {
			testingContext.Fatalf("missing %s in %s", declaration, normalized)
		}
	}
}

func TestValidateOnlyOwnVersions(testingContext *testing.T) {
	root := fixture(testingContext)
	updateManifest(testingContext, root, func(manifest map[string]any) {
		for _, other := range []string{"norns", "nornsd", "well"} {
			if other != product {
				manifest[other] = "unrelated"
			}
		}
	})
	if _, err := readVersions(root); err != nil {
		testingContext.Fatal(err)
	}
	cases := []struct {
		path  string
		value any
	}{
		{"protocol", "2"}, {"protocol", 0}, {product + ".version", "dev"}, {product + ".version", "1.2"}, {product + ".version", "01.2.3"}, {product + ".version", "1x2x3"},
		{product + ".support_protocol", nil}, {product + ".database_schema", -1}, {product + ".database_schema", 1.5},
	}
	if product == "nornsd" {
		cases = append(cases, struct {
			path  string
			value any
		}{"nornsd.api_schema", 0}, struct {
			path  string
			value any
		}{"nornsd.agent_config_schema", 0}, struct {
			path  string
			value any
		}{"nornsd.plugins.cc", nil}, struct {
			path  string
			value any
		}{"nornsd.plugins.extra", -1})
	}
	for _, test := range cases {
		testingContext.Run(test.path+fmt.Sprint(test.value), func(testingContext *testing.T) {
			root := fixture(testingContext)
			updateManifest(testingContext, root, func(manifest map[string]any) {
				parts := strings.Split(test.path, ".")
				node := manifest
				for _, part := range parts[:len(parts)-1] {
					node = node[part].(map[string]any)
				}
				node[parts[len(parts)-1]] = test.value
			})
			if _, err := readVersions(root); err == nil {
				testingContext.Fatal("invalid version was accepted")
			}
		})
	}
}

func TestCustomOutputPreservesOtherArtifacts(testingContext *testing.T) {
	root := fixture(testingContext)
	release := filepath.Join(testingContext.TempDir(), "artifacts")
	platform := target{runtime.GOOS, runtime.GOARCH}.directory()
	output := filepath.Join(release, platform)
	other := "nornsd"
	if product == "nornsd" {
		other = "norns"
	}
	preserved := []string{filepath.Join(output, other), filepath.Join(output, "well/bundle/old.pkg"), filepath.Join(release, "other-platform", product)}
	for _, filename := range preserved {
		writeFixture(testingContext, filename, "preserved")
	}
	stale := filepath.Join(output, "norns-obsolete")
	if product == "norns" {
		writeFixture(testingContext, stale, "stale")
	}
	var calls []invocation
	if err := build(root, options{output: release}, fakeRunner(testingContext, &calls)); err != nil {
		testingContext.Fatal(err)
	}
	for _, filename := range preserved {
		contents, err := os.ReadFile(filename)
		if err != nil || string(contents) != "preserved" {
			testingContext.Fatalf("unselected artifact changed: %s %v", filename, err)
		}
	}
	if product == "norns" {
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			testingContext.Fatal("obsolete plugin was not removed")
		}
	}
	contents, err := os.ReadFile(filepath.Join(release, "SHA256SUMS"))
	if err != nil || !strings.Contains(string(contents), fmt.Sprintf("%x", sha256.Sum256([]byte("preserved")))) {
		testingContext.Fatalf("preserved artifacts missing from checksums: %s %v", contents, err)
	}
	if _, err := os.Stat(filepath.Join(root, "release")); !os.IsNotExist(err) {
		testingContext.Fatal("custom output wrote default release")
	}
}

func TestFailedBuildDoesNotReplaceRelease(testingContext *testing.T) {
	root := fixture(testingContext)
	filename := filepath.Join(root, "release", target{runtime.GOOS, runtime.GOARCH}.directory(), product)
	writeFixture(testingContext, filename, "preserved")
	failure := errors.New("build failed")
	if err := build(root, options{}, func(string, []string, string, ...string) error { return failure }); !errors.Is(err, failure) {
		testingContext.Fatalf("build error = %v", err)
	}
	contents, err := os.ReadFile(filename)
	if err != nil || string(contents) != "preserved" {
		testingContext.Fatalf("release replaced on failure: %s %v", contents, err)
	}
	staging, err := filepath.Glob(filepath.Join(root, "release", ".release.tmp.*"))
	if err != nil || len(staging) != 0 {
		testingContext.Fatalf("staging leaked: %v %v", staging, err)
	}
}

func TestRejectSymlinkRelease(testingContext *testing.T) {
	if runtime.GOOS == "windows" {
		testingContext.Skip("symlink creation requires Windows privileges")
	}
	for _, platformLink := range []bool{false, true} {
		root := fixture(testingContext)
		filename := filepath.Join(root, "release")
		if platformLink {
			filename = filepath.Join(filename, target{runtime.GOOS, runtime.GOARCH}.directory())
			if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
				testingContext.Fatal(err)
			}
		}
		if err := os.Symlink(testingContext.TempDir(), filename); err != nil {
			testingContext.Fatal(err)
		}
		if err := build(root, options{}, func(string, []string, string, ...string) error {
			testingContext.Fatal("must reject symlink before commands")
			return nil
		}); err == nil {
			testingContext.Fatal("accepted symlink release directory")
		}
	}
}

func TestStagingInsideOutputExcludedFromChecksums(t *testing.T) {
	root := fixture(t)
	output := filepath.Join(t.TempDir(), "custom release")
	stale := filepath.Join(output, ".release.tmp.stale", "partial")
	writeFixture(t, stale, "unfinished build")
	var calls []invocation
	run := fakeRunner(t, &calls)
	err := build(root, options{output: output}, func(directory string, environment []string, name string, arguments ...string) error {
		stages, err := filepath.Glob(filepath.Join(output, ".release.tmp.*"))
		if err != nil || len(stages) != 2 {
			t.Fatalf("expected current and stale staging in output: %v %v", stages, err)
		}
		roots, err := filepath.Glob(filepath.Join(root, ".release.tmp.*"))
		if err != nil || len(roots) != 0 {
			t.Fatalf("staging created at repository root: %v %v", roots, err)
		}
		return run(directory, environment, name, arguments...)
	})
	if err != nil {
		t.Fatal(err)
	}
	stages, err := filepath.Glob(filepath.Join(output, ".release.tmp.*"))
	if err != nil || len(stages) != 1 || stages[0] != filepath.Dir(stale) {
		t.Fatalf("current staging not cleaned: %v %v", stages, err)
	}
	contents, err := os.ReadFile(filepath.Join(output, "SHA256SUMS"))
	if err != nil || len(contents) == 0 || strings.Contains(string(contents), ".release.tmp.") {
		t.Fatalf("invalid checksums: %s %v", contents, err)
	}
	if contents, err := os.ReadFile(stale); err != nil || string(contents) != "unfinished build" {
		t.Fatalf("unrelated stale staging changed: %s %v", contents, err)
	}
}

func TestSelectedTargets(t *testing.T) {
	root := fixture(t)
	selected, err := parseOptions([]string{"--target", "darwin/arm64", "--target", "windows/amd64", "--target", "linux/amd64", "--target", "darwin/arm64"})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.targets) != 3 {
		t.Fatalf("targets = %v", selected.targets)
	}
	var calls []invocation
	if err := build(root, selected, fakeRunner(t, &calls)); err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"darwin-arm64", "windows-x64", "linux-x64"} {
		executable := product
		if strings.HasPrefix(platform, "windows-") {
			executable += ".exe"
		}
		if _, err := os.Stat(filepath.Join(root, "release", platform, executable)); err != nil {
			t.Fatal(err)
		}
	}
	for _, platform := range []string{"darwin-x64", "windows-arm64", "linux-arm64"} {
		if _, err := os.Stat(filepath.Join(root, "release", platform)); !os.IsNotExist(err) {
			t.Fatalf("unexpected target %s: %v", platform, err)
		}
	}
	builds := 0
	for _, call := range calls {
		if call.arguments[0] != "build" {
			continue
		}
		builds++
		if !slices.Contains(call.arguments, "-trimpath") || !strings.Contains(strings.Join(call.arguments, " "), "-s -w") {
			t.Fatalf("missing release privacy flags: %v", call.arguments)
		}
	}
	if builds != 3 {
		t.Fatalf("build count = %d", builds)
	}
	for _, arguments := range [][]string{{"--target"}, {"--target", "linux/386"}, {"--target", "freebsd/amd64"}, {"--target", "../arm64"}, {"--target", "linux/amd64", "--all-platforms"}} {
		if _, err := parseOptions(arguments); err == nil {
			t.Fatalf("accepted %v", arguments)
		}
	}
}
