package cc

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestPublishedUgrepGuard(t *testing.T) {
	subject := t.TempDir()
	pluginPath, _, err := Publish(subject, "runtime-ugrep-guard", nil)
	if err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(pluginPath, "hooks", "pretooluse_ugrep_guard.sh")
	if !supportsUgrepGuard(runtime.GOOS) {
		if _, statErr := os.Stat(hook); !os.IsNotExist(statErr) {
			t.Fatalf("ugrep guard must not be published on %s", runtime.GOOS)
		}
		return
	}
	if _, err = os.Stat(hook); err != nil {
		t.Fatalf("published ugrep guard: %v", err)
	}
	if _, err = os.Stat(filepath.Join(pluginPath, "hooks", "hooks.json")); err != nil {
		t.Fatalf("published hook registration: %v", err)
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is required to exercise the Claude Code hook")
	}

	tests := []struct {
		name    string
		payload any
		deny    bool
	}{
		{
			name: "c-03 bounded repeats",
			payload: map[string]any{
				"tool_name":  "Bash",
				"tool_input": map[string]any{"command": `grep -oE '"[^"]{0,40}(init|password|setup|validate)[^"]{0,40}"' /tmp/initpage.js`},
			},
			deny: true,
		},
		{
			name: "alternation remains inside quoted pattern",
			payload: map[string]any{
				"tool_name":  "Bash",
				"tool_input": map[string]any{"command": `grep -E 'a.{0,12}(x|y).{0,11}b' input.txt | head`},
			},
			deny: true,
		},
		{
			name: "bare ugrep",
			payload: map[string]any{
				"tool_name":  "Bash",
				"tool_input": map[string]any{"command": `ugrep -E 'a.{0,12}b.{0,11}c' input.txt`},
			},
			deny: true,
		},
		{
			name: "backslash does not bypass a function",
			payload: map[string]any{
				"tool_name":  "Bash",
				"tool_input": map[string]any{"command": `\grep -E 'a.{0,12}b.{0,11}c' input.txt`},
			},
			deny: true,
		},
		{
			name: "unparseable command uses fallback",
			payload: map[string]any{
				"tool_name":  "Bash",
				"tool_input": map[string]any{"command": `grep -E 'a.{0,12}b.{0,11}c input.txt`},
			},
			deny: true,
		},
		{
			name: "one range",
			payload: map[string]any{
				"tool_name":  "Bash",
				"tool_input": map[string]any{"command": `grep -E 'a.{0,100}b' input.txt`},
			},
		},
		{
			name: "exact counts",
			payload: map[string]any{
				"tool_name":  "Bash",
				"tool_input": map[string]any{"command": `grep -E 'a.{20}b.{30}c' input.txt`},
			},
		},
		{
			name: "independent grep segments",
			payload: map[string]any{
				"tool_name":  "Bash",
				"tool_input": map[string]any{"command": `grep -E 'a.{0,40}' a.txt | grep -E 'b.{0,40}' b.txt`},
			},
		},
		{
			name: "explicit GNU grep",
			payload: map[string]any{
				"tool_name":  "Bash",
				"tool_input": map[string]any{"command": `/usr/bin/grep -E 'a.{0,100}b.{0,100}c' input.txt`},
			},
		},
		{
			name: "command bypasses shell function",
			payload: map[string]any{
				"tool_name":  "Bash",
				"tool_input": map[string]any{"command": `command grep -E 'a.{0,100}b.{0,100}c' input.txt`},
			},
		},
		{
			name: "other tool",
			payload: map[string]any{
				"tool_name":  "Grep",
				"tool_input": map[string]any{"pattern": `a.{0,100}b.{0,100}c`},
			},
		},
		{
			name: "non-string command",
			payload: map[string]any{
				"tool_name":  "Bash",
				"tool_input": map[string]any{"command": 42},
			},
		},
		{name: "malformed payload", payload: "not-an-object"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input, marshalErr := json.Marshal(test.payload)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			cmd := exec.Command(shell, hook)
			cmd.Stdin = bytes.NewReader(input)
			output, runErr := cmd.Output()
			if runErr != nil {
				t.Fatalf("run guard: %v", runErr)
			}
			if !test.deny {
				if len(output) != 0 {
					t.Fatalf("guard denied safe command: %s", output)
				}
				return
			}
			var response struct {
				HookSpecificOutput struct {
					HookEventName            string `json:"hookEventName"`
					PermissionDecision       string `json:"permissionDecision"`
					PermissionDecisionReason string `json:"permissionDecisionReason"`
				} `json:"hookSpecificOutput"`
			}
			if unmarshalErr := json.Unmarshal(output, &response); unmarshalErr != nil {
				t.Fatalf("decode deny response: %v; output=%s", unmarshalErr, output)
			}
			decision := response.HookSpecificOutput
			if decision.HookEventName != "PreToolUse" || decision.PermissionDecision != "deny" {
				t.Fatalf("unexpected deny response: %s", output)
			}
			if !strings.Contains(decision.PermissionDecisionReason, "/usr/bin/grep") {
				t.Fatalf("deny response lacks remediation: %s", output)
			}
		})
	}
}

func TestSupportsUgrepGuard(t *testing.T) {
	for _, test := range []struct {
		goos string
		want bool
	}{
		{"linux", true},
		{"darwin", true},
		{"windows", false},
		{"freebsd", false},
	} {
		if got := supportsUgrepGuard(test.goos); got != test.want {
			t.Errorf("supportsUgrepGuard(%q) = %t, want %t", test.goos, got, test.want)
		}
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
