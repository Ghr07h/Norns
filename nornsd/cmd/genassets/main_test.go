package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGenerateVersionlessPluginTemplates(testingContext *testing.T) {
	root := testingContext.TempDir()
	plugins := filepath.Join(root, "plugins")
	if err := copyTree("../../../plugin", plugins); err != nil {
		testingContext.Fatal(err)
	}
	readObject := func(filename string) map[string]any {
		testingContext.Helper()
		contents, err := os.ReadFile(filename)
		if err != nil {
			testingContext.Fatal(err)
		}
		var object map[string]any
		if err := json.Unmarshal(contents, &object); err != nil {
			testingContext.Fatal(err)
		}
		return object
	}
	metadataPath := filepath.Join("cc", ".claude-plugin", "plugin.json")
	settingsPath := filepath.Join("cc", "settings.json")
	metadata := readObject(filepath.Join(plugins, metadataPath))
	if _, exists := metadata["version"]; exists {
		testingContext.Fatal("Plugin source template declares its own version")
	}
	settings := readObject(filepath.Join(plugins, settingsPath))
	if _, exists := settings["env"].(map[string]any)["NORNS_PROTOCOL_VERSION"]; exists {
		testingContext.Fatal("Plugin source template declares its own protocol version")
	}
	for path, contents := range map[string]string{
		"versions.json":        `{"protocol": 42, "nornsd": {"support_protocol": 41, "plugins": {"cc": 46}}}`,
		"protocol/protocol.md": "# Protocol fixture\n",
	} {
		filename := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			testingContext.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(contents), 0o644); err != nil {
			testingContext.Fatal(err)
		}
	}
	output := filepath.Join(root, "generated")
	command := exec.Command("go", "run", ".", "--versions", filepath.Join(root, "versions.json"),
		"--protocol", filepath.Join(root, "protocol"), "--plugins", plugins, "--output", output)
	if logs, err := command.CombinedOutput(); err != nil {
		testingContext.Fatalf("generate: %v\n%s", err, logs)
	}
	generatedPlugins := filepath.Join(output, "plugins")
	generatedMetadata := readObject(filepath.Join(generatedPlugins, metadataPath))
	if generatedMetadata["version"] != "46.0.0" || generatedMetadata["name"] != metadata["name"] {
		testingContext.Fatalf("unexpected generated Plugin metadata: %#v", generatedMetadata)
	}
	generatedSettings := readObject(filepath.Join(generatedPlugins, settingsPath))
	if generatedSettings["env"].(map[string]any)["NORNS_PROTOCOL_VERSION"] != "41" {
		testingContext.Fatalf("unexpected generated Plugin settings: %#v", generatedSettings)
	}
	generatedManifest := readObject(filepath.Join(output, "manifest.generated.json"))
	if generatedManifest["protocol_version"] != float64(42) {
		testingContext.Fatalf("unexpected generated protocol version: %#v", generatedManifest)
	}
	protocol, err := os.ReadFile(filepath.Join(generatedPlugins, "cc/skills/norns-loop/references/protocol.md"))
	if err != nil || string(protocol) != "# Protocol fixture\n" {
		testingContext.Fatalf("unexpected embedded protocol: %q %v", protocol, err)
	}
	if source := readObject(filepath.Join(plugins, metadataPath)); source["version"] != nil {
		testingContext.Fatal("generation modified the Plugin source version")
	}
}

func TestCopyTreeSkipsPluginBackupDirectories(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	for path, content := range map[string]string{
		filepath.Join("cc", "agents", "coordinator.md"):           "current",
		filepath.Join("cc", "backup", "agents", "coordinator.md"): "backup",
	} {
		full := filepath.Join(source, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := copyTree(source, target); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(filepath.Join(target, "cc", "agents", "coordinator.md")); err != nil || string(content) != "current" {
		t.Fatalf("active Agent was not copied: content=%q err=%v", content, err)
	}
	if _, err := os.Stat(filepath.Join(target, "cc", "backup")); !os.IsNotExist(err) {
		t.Fatalf("backup directory was copied: %v", err)
	}
}

func TestReadVersionsWithoutRootGoModule(testingContext *testing.T) {
	manifest, err := readVersions("../../../versions.json")
	if err != nil {
		testingContext.Fatal(err)
	}
	if manifest.Protocol < 1 || manifest.Nornsd.SupportProtocol < 1 || manifest.Nornsd.Plugins["cc"] < 1 {
		testingContext.Fatalf("invalid root manifest: %#v", manifest)
	}
}

func TestReadVersionsRejectsInvalidManifest(testingContext *testing.T) {
	for _, source := range []string{
		`{`,
		`{"protocol": 0, "nornsd": {"support_protocol": 2, "plugins": {"cc": 1}}}`,
		`{"protocol": 2, "nornsd": {"support_protocol": "2", "plugins": {"cc": 1}}}`,
		`{"protocol": 2, "nornsd": {"support_protocol": 2, "plugins": {}}}`,
		`{"protocol": 2, "nornsd": {"support_protocol": 2, "plugins": {"cc": 1, "other": -1}}}`,
	} {
		filename := filepath.Join(testingContext.TempDir(), "versions.json")
		if err := os.WriteFile(filename, []byte(source), 0o600); err != nil {
			testingContext.Fatal(err)
		}
		if _, err := readVersions(filename); err == nil {
			testingContext.Fatalf("invalid manifest was accepted: %s", source)
		}
	}
}
