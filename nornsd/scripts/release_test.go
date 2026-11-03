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
	writeFixture(testingContext, filepath.Join(root, product, "configs/nornsd/agent.example.yaml"), "VERSION: 1\nLLM_MODEL: example\n")
	writeFixture(testingContext, filepath.Join(root, product, "cmd", product, "main.go"), "package main\n")
	if product == "norns" {
		writeFixture(testingContext, filepath.Join(root, product, "cmd/norns-websearch/main.go"), "package main\n")
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
	updateManifest(testingContext, root, func(manifest map[string]any) {
		manifest[product].(map[string]any)["agent_config_schema"] = 45
	})
	var calls []invocation
	runner := fakeRunner(testingContext, &calls)
	if err := build(root, options{}, func(directory string, environment []string, name string, arguments ...string) error {
		contents, err := os.ReadFile(filepath.Join(root, product, "configs/nornsd/agent.example.yaml"))
		if err != nil || string(contents) != "VERSION: 45\nLLM_MODEL: example\n" {
			testingContext.Fatalf("Agent example was not synchronized before Go commands: %q %v", contents, err)
		}
		return runner(directory, environment, name, arguments...)
	}); err != nil {
		testingContext.Fatal(err)
	}
	wantCalls := 2
	if len(calls) != wantCalls {
		testingContext.Fatalf("commands = %d, want %d", len(calls), wantCalls)
	}
	checksums, err := os.ReadFile(filepath.Join(root, "release/SHA256SUMS"))
	if err != nil || len(checksums) == 0 {
		testingContext.Fatalf("missing checksums: %v", err)
	}
	example := "VERSION: 45\nLLM_MODEL: example\n"
	filename := filepath.Join(root, "release", target{runtime.GOOS, runtime.GOARCH}.directory(), "agent.example.yaml")
	if contents, err := os.ReadFile(filename); err != nil || string(contents) != example {
		testingContext.Fatalf("published Agent example was not synchronized: %q %v", contents, err)
	}
	if !strings.Contains(string(checksums), fmt.Sprintf("%x", sha256.Sum256([]byte(example)))) {
		testingContext.Fatal("published Agent example missing from checksums")
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

func TestGenerateAgentExample(testingContext *testing.T) {
	root := fixture(testingContext)
	manifest, err := readVersions(root)
	if err != nil {
		testingContext.Fatal(err)
	}
	manifest.Nornsd.AgentConfigSchema = 45
	filename := filepath.Join(root, product, "configs/nornsd/agent.example.yaml")
	source := "# schema\r\nVERSION: 1\r\nLLM_MODEL: example\r\n"
	writeFixture(testingContext, filename, source)
	userConfig := filepath.Join(root, "bin/agent.yaml")
	writeFixture(testingContext, userConfig, "VERSION: 1\nLLM_KEY: private\n")
	if err := generateAgentExample(root, manifest); err != nil {
		testingContext.Fatal(err)
	}
	contents, err := os.ReadFile(filename)
	if err != nil || string(contents) != strings.Replace(source, "VERSION: 1", "VERSION: 45", 1) {
		testingContext.Fatalf("example = %q: %v", contents, err)
	}
	oldTime := time.Unix(1_000_000, 0)
	if err := os.Chtimes(filename, oldTime, oldTime); err != nil {
		testingContext.Fatal(err)
	}
	if err := generateAgentExample(root, manifest); err != nil {
		testingContext.Fatal(err)
	}
	info, err := os.Stat(filename)
	if err != nil || !info.ModTime().Equal(oldTime) {
		testingContext.Fatalf("unchanged example was rewritten: %v", err)
	}
	contents, err = os.ReadFile(userConfig)
	if err != nil || string(contents) != "VERSION: 1\nLLM_KEY: private\n" {
		testingContext.Fatalf("user config was modified: %q %v", contents, err)
	}
}

func TestGenerateAgentExampleRejectsMissingOrDuplicateVersion(testingContext *testing.T) {
	for _, source := range []string{"LLM_MODEL: example\n", "VERSION: 1\nVERSION: 2\n"} {
		root := fixture(testingContext)
		manifest, err := readVersions(root)
		if err != nil {
			testingContext.Fatal(err)
		}
		filename := filepath.Join(root, product, "configs/nornsd/agent.example.yaml")
		writeFixture(testingContext, filename, source)
		if err := generateAgentExample(root, manifest); err == nil {
			testingContext.Fatalf("accepted malformed example: %q", source)
		}
		contents, err := os.ReadFile(filename)
		if err != nil || string(contents) != source {
			testingContext.Fatalf("malformed example was modified: %q %v", contents, err)
		}
	}
}

func TestOptionsAndRootDiscovery(testingContext *testing.T) {
	for _, arguments := range [][]string{{"unknown"}, {"--unknown"}, {"norns"}, {"nornsd"}, {"--output"}, {"--output", "--all-platforms"}} {
		if _, err := parseOptions(arguments); err == nil {
			testingContext.Fatalf("accepted %v", arguments)
		}
	}
	selected, err := parseOptions([]string{"--all-platforms", "--output", "custom"})
	if err != nil || !selected.allPlatforms || selected.output != "custom" {
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
	wantCalls := 7
	if product == "norns" {
		executables = append(executables, "norns-websearch")
		wantCalls = 12
	}
	if len(calls) != wantCalls {
		testingContext.Fatalf("commands = %d, want %d", len(calls), wantCalls)
	}
	for _, platform := range []target{{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}} {
		expected, err := os.ReadFile(filepath.Join(root, product, "configs/nornsd/agent.example.yaml"))
		if err != nil {
			testingContext.Fatal(err)
		}
		if contents, err := os.ReadFile(filepath.Join(root, "release", platform.directory(), "agent.example.yaml")); err != nil || !slices.Equal(contents, expected) {
			testingContext.Fatalf("missing or mismatched %s Agent example: %q %v", platform.directory(), contents, err)
		}
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
	preserved := []string{filepath.Join(output, other), filepath.Join(output, "well/bundle/old.pkg"), filepath.Join(output, "agent.yaml"), filepath.Join(release, "other-platform", product)}
	for _, filename := range preserved {
		writeFixture(testingContext, filename, "preserved")
	}
	writeFixture(testingContext, filepath.Join(output, "agent.example.yaml"), "old example")
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
	expected, err := os.ReadFile(filepath.Join(root, product, "configs/nornsd/agent.example.yaml"))
	if err != nil {
		testingContext.Fatal(err)
	}
	if contents, err := os.ReadFile(filepath.Join(output, "agent.example.yaml")); err != nil || !slices.Equal(contents, expected) {
		testingContext.Fatalf("old Agent example was not replaced: %q %v", contents, err)
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
	example := filepath.Join(filepath.Dir(filename), "agent.example.yaml")
	writeFixture(testingContext, example, "old example")
	failure := errors.New("build failed")
	if err := build(root, options{}, func(string, []string, string, ...string) error { return failure }); !errors.Is(err, failure) {
		testingContext.Fatalf("build error = %v", err)
	}
	contents, err := os.ReadFile(filename)
	if err != nil || string(contents) != "preserved" {
		testingContext.Fatalf("release replaced on failure: %s %v", contents, err)
	}
	if contents, err := os.ReadFile(example); err != nil || string(contents) != "old example" {
		testingContext.Fatalf("Agent example replaced on failure: %q %v", contents, err)
	}
	staging, err := filepath.Glob(filepath.Join(root, ".release.tmp.*"))
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
