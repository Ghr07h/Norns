package cc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/nornsd/internal/config"
)

func TestPublishInjectsNornsRuntimeConfiguration(t *testing.T) {
	subject := t.TempDir()
	if err := os.MkdirAll(filepath.Join(subject, ".nornsd", "staging"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{NornsMaxConcurrentExplorations: 5, NornsExplorationTimeoutSeconds: 1500, NornsAllowHumanIntervention: true}
	path, _, err := Publish(subject, "runtime-1", cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(path, "agents", "coordinator.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "--concurrency 5") || !strings.Contains(text, "1500 秒") || !strings.Contains(text, "允许在遇到") {
		t.Fatalf("generated coordinator did not contain runtime config:\n%s", text)
	}
	if strings.Contains(text, "{{NORNS_RUNTIME_CONFIGURATION}}") {
		t.Fatal("template marker leaked into plugin")
	}
}

func TestPublishIncludesNornsPluginMetadata(t *testing.T) {
	subject := t.TempDir()
	path, _, err := Publish(subject, "runtime-metadata", nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(path, ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Name string `json:"name"`
	}
	if err = json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Name != "norns" {
		t.Fatalf("Plugin name = %q, want norns", metadata.Name)
	}
}

func TestPublishGeneratesDomainAgents(t *testing.T) {
	subject := t.TempDir()
	if err := os.MkdirAll(filepath.Join(subject, ".nornsd", "staging"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		NornsMaxConcurrentExplorations: 3,
		Domain: &config.Domain{
			Name:                "code_review",
			DeciderDescription:  "选择代码审查方向：优先风险。",
			DeciderExtraInfo:    "## 领域决策知识\n\n检查安全边界。\n",
			ExplorerDescription: "执行代码审查并收集证据。",
			ExplorerExtraInfo:   "## 领域探索知识\n\n运行最小测试。\n",
		},
	}
	path, _, err := Publish(subject, "runtime-domain", cfg)
	if err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		file, name, description, extra string
	}{
		{"decider-for-code_review.md", "decider-for-code_review", cfg.Domain.DeciderDescription, cfg.Domain.DeciderExtraInfo},
		{"explorer-for-code_review.md", "explorer-for-code_review", cfg.Domain.ExplorerDescription, cfg.Domain.ExplorerExtraInfo},
	}
	for _, check := range checks {
		b, readErr := os.ReadFile(filepath.Join(path, "agents", check.file))
		if readErr != nil {
			t.Fatal(readErr)
		}
		text := string(b)
		if !strings.Contains(text, "name: "+check.name+"\n") || !strings.Contains(text, "description: "+check.description+"\n") {
			t.Fatalf("generated %s has unexpected front matter:\n%s", check.file, text)
		}
		if !strings.HasSuffix(text, "\n"+check.extra) {
			t.Fatalf("generated %s does not end with domain info:\n%s", check.file, text)
		}
	}

	coordinatorBytes, err := os.ReadFile(filepath.Join(path, "agents", "coordinator.md"))
	if err != nil {
		t.Fatal(err)
	}
	coordinator := string(coordinatorBytes)
	for _, want := range []string{"norns:decider-for-code_review", "norns:explorer-for-code_review"} {
		if !strings.Contains(coordinator, want) {
			t.Fatalf("coordinator is missing %q:\n%s", want, coordinator)
		}
	}
	if strings.Contains(coordinator, "当前领域为 `code_review`") {
		t.Fatalf("domain selection leaked into runtime configuration:\n%s", coordinator)
	}
	front, _, err := parseAgentMarkdown(coordinatorBytes, "coordinator.md")
	if err != nil {
		t.Fatal(err)
	}
	wantAgentTool := "Agent(norns:decider, norns:decider-for-code_review, norns:explorer, norns:explorer-for-code_review)"
	if !containsTool(front.Tools, wantAgentTool) {
		t.Fatalf("Coordinator tools = %#v, want %q", front.Tools, wantAgentTool)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(path, "plugin.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err = json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, check := range checks {
		if manifest.Files["agents/"+check.file] == "" {
			t.Fatalf("manifest is missing %s", check.file)
		}
	}
}

func TestPublishBuildsCoordinatorAllowlistFromAllAgentSources(t *testing.T) {
	subject := t.TempDir()
	if err := os.MkdirAll(filepath.Join(subject, ".nornsd", "staging"), 0700); err != nil {
		t.Fatal(err)
	}
	sources := t.TempDir()
	nornsAgents := filepath.Join(sources, "norns-agents")
	customAgents := filepath.Join(sources, "custom-agents")
	externalPlugin := filepath.Join(sources, "external-plugin")
	for _, dir := range []string{nornsAgents, customAgents, filepath.Join(externalPlugin, ".claude-plugin"), filepath.Join(externalPlugin, "agents")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(nornsAgents, "specialist.md"), "---\nname: specialist\ndescription: domain specialist\n---\n")
	writeTestFile(t, filepath.Join(customAgents, "reviewer.md"), "---\nname: reviewer\ndescription: reviewer\ntools:\n  - Read\n---\n")
	writeTestFile(t, filepath.Join(externalPlugin, ".claude-plugin", "plugin.json"), `{"name":"external-tools"}`)
	writeTestFile(t, filepath.Join(externalPlugin, "agents", "researcher.md"), "---\nname: researcher\ndescription: researcher\n---\n")

	cfg := &config.Config{
		NornsCustomAgentsDir: nornsAgents,
		CustomAgentsDir:      customAgents,
		CustomPluginDirs:     []string{externalPlugin},
	}
	path, _, err := Publish(subject, "runtime-custom-agents", cfg)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := os.ReadFile(filepath.Join(path, "agents", "coordinator.md"))
	if err != nil {
		t.Fatal(err)
	}
	front, _, err := parseAgentMarkdown(coordinator, "coordinator.md")
	if err != nil {
		t.Fatal(err)
	}
	want := "Agent(norns:decider, norns:explorer, norns:specialist, nornsd-custom:reviewer, external-tools:researcher)"
	if !containsTool(front.Tools, want) {
		t.Fatalf("Coordinator tools = %#v, want %q", front.Tools, want)
	}
}

func containsTool(tools toolList, want string) bool {
	for _, tool := range tools {
		if tool == want {
			return true
		}
	}
	return false
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
