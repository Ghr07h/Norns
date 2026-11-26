package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/extension/metadata"
)

func TestPublishSelectedExtensionMetadata(t *testing.T) {
	root := fixture(t)
	var calls []invocation
	if err := build(root, options{allPlatforms: true, extensions: []string{"websearch", "lookup", "websearch"}}, fakeRunner(t, &calls)); err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"darwin-x64", "darwin-arm64", "linux-x64", "linux-arm64", "windows-x64", "windows-arm64"} {
		filename := filepath.Join(root, "release", platform, metadata.Filename)
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		items, err := metadata.ParseCatalog(data)
		if err != nil || len(items) != 2 || items[0].Name != "lookup" || items[1].Name != "websearch" {
			t.Fatalf("catalog=%s err=%v", data, err)
		}
	}
	checksums, _ := os.ReadFile(filepath.Join(root, "release/SHA256SUMS"))
	if strings.Count(string(checksums), metadata.Filename) != 6 {
		t.Fatal("metadata missing from platform checksums")
	}
	if err := build(root, options{}, fakeRunner(t, &calls)); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "release", target{runtime.GOOS, runtime.GOARCH}.directory(), metadata.Filename)
	data, _ := os.ReadFile(filename)
	if strings.TrimSpace(string(data)) != "[]" {
		t.Fatalf("default release retained stale metadata: %s", data)
	}
}

func TestPublishExtensionFilesOnEveryPlatformAndPrune(t *testing.T) {
	root := fixture(t)
	files := []metadata.ReleaseFile{{OriginFile: "configs/default.yaml", TargetFile: "lookup.example.yaml"}, {OriginFile: "usage.txt", TargetFile: ""}}
	item := metadata.Metadata{Name: "lookup", Version: "0.1.0", Description: "Lookup", Requirements: []string{}, ReleaseFiles: files}
	data, _ := json.Marshal(item)
	writeFixture(t, filepath.Join(root, product, "internal/custom_extensions/lookup/metadata.json"), string(data))
	writeFixture(t, filepath.Join(root, product, "internal/custom_extensions/lookup/configs/default.yaml"), "public config")
	writeFixture(t, filepath.Join(root, product, "internal/custom_extensions/lookup/usage.txt"), "public usage")
	var calls []invocation
	if err := build(root, options{allPlatforms: true, extensions: []string{"lookup"}}, fakeRunner(t, &calls)); err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"darwin-x64", "darwin-arm64", "linux-x64", "linux-arm64", "windows-x64", "windows-arm64"} {
		for name, want := range map[string]string{"norns.exts.lookup.lookup.example.yaml": "public config", "norns.exts.lookup.usage.txt": "public usage"} {
			if data, err := os.ReadFile(filepath.Join(root, "release", platform, name)); err != nil || string(data) != want {
				t.Fatalf("%s %s: %s %v", platform, name, data, err)
			}
		}
	}
	checksums, _ := os.ReadFile(filepath.Join(root, "release/SHA256SUMS"))
	if strings.Count(string(checksums), "norns.exts.lookup.lookup.example.yaml") != 6 || strings.Count(string(checksums), "norns.exts.lookup.usage.txt") != 6 {
		t.Fatal("attachments missing from checksums")
	}
	if err := build(root, options{}, fakeRunner(t, &calls)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"norns.exts.lookup.lookup.example.yaml", "norns.exts.lookup.usage.txt"} {
		if _, err := os.Stat(filepath.Join(root, "release", target{runtime.GOOS, runtime.GOARCH}.directory(), name)); !os.IsNotExist(err) {
			t.Fatal("stale attachment retained")
		}
	}
}

func TestMissingExtensionFileFailsBeforeBuild(t *testing.T) {
	root := fixture(t)
	item := metadata.Metadata{Name: "lookup", Version: "0.1.0", Description: "Lookup", Requirements: []string{}, ReleaseFiles: []metadata.ReleaseFile{{OriginFile: "missing.yaml", TargetFile: "lookup.example.yaml"}}}
	data, _ := json.Marshal(item)
	writeFixture(t, filepath.Join(root, product, "internal/custom_extensions/lookup/metadata.json"), string(data))
	published := filepath.Join(root, "release", target{runtime.GOOS, runtime.GOARCH}.directory(), "norns")
	writeFixture(t, published, "preserved")
	if err := build(root, options{extensions: []string{"lookup"}}, func(string, []string, string, ...string) error {
		t.Fatal("missing file should fail before compiling")
		return nil
	}); err == nil {
		t.Fatal("missing source accepted")
	}
	if data, _ := os.ReadFile(published); string(data) != "preserved" {
		t.Fatal("release changed on validation failure")
	}
}

func TestInvalidExtensionMetadataPreservesRelease(t *testing.T) {
	for _, source := range []string{"", "{}", `{"name":"other","version":"1","description":"Wrong name","requirements":[]}`} {
		root := fixture(t)
		metadataFile := filepath.Join(root, product, "internal/custom_extensions/websearch/metadata.json")
		if source == "" {
			if err := os.Remove(metadataFile); err != nil {
				t.Fatal(err)
			}
		} else {
			writeFixture(t, metadataFile, source)
		}
		published := filepath.Join(root, "release", target{runtime.GOOS, runtime.GOARCH}.directory(), metadata.Filename)
		writeFixture(t, published, "preserved")
		if err := build(root, options{extensions: []string{"websearch"}}, func(string, []string, string, ...string) error {
			t.Fatal("invalid metadata must be rejected before compilation")
			return nil
		}); err == nil {
			t.Fatal("invalid metadata accepted")
		}
		if data, _ := os.ReadFile(published); string(data) != "preserved" {
			t.Fatal("invalid metadata changed published artifacts")
		}
	}
}

func TestCoordinatorReplacesStaleMetadataDirectory(t *testing.T) {
	root := coordinatorFixture(t)
	stale := filepath.Join(root, "release/linux-x64", metadata.Filename, "stale")
	writeFixture(t, stale, "old")
	if output, err := runCoordinator(root, "norns"); err != nil {
		t.Fatalf("coordinator failed: %v\n%s", err, output)
	}
	data, err := os.ReadFile(filepath.Dir(stale))
	if err != nil || strings.TrimSpace(string(data)) != "[]" {
		t.Fatalf("stale metadata not replaced: %s %v", data, err)
	}
}
