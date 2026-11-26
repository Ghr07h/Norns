package cc

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/nornsd/internal/assets"
	"github.com/Ghr07h/norns/nornsd/internal/config"
	"gopkg.in/yaml.v3"
)

func TestCustomMainAgentUsesNameAndReceivesOnlyIntendedChanges(t *testing.T) {
	source := filepath.Join(t.TempDir(), "coordinator-for-math.md")
	original := "---\nname: math-main\ndescription: math coordinator\ntools: [Read, 'Agent(old:role)', Bash]\nmaxTurns: 100 # preserve custom setting\npermissionMode: plan\ncustom:\n  flags: [true, false]\n  threshold: 0.5\n---\nMath instructions.\n{{NORNS_RUNTIME_CONFIGURATION}}\n"
	writeTestFile(t, source, original)
	agents := t.TempDir()
	writeTestFile(t, filepath.Join(agents, "helper.md"), "---\nname: math-helper\n---\nhelper\n")
	cfg := &config.Config{NornsEnable: true, MainAgentFile: source, NornsCustomAgentsDir: agents, NornsMaxConcurrentExplorations: 7, NornsExplorationTimeoutSeconds: 99, Domain: &config.Domain{Name: "math", DeciderDescription: "math decision", ExplorerDescription: "math exploration"}}
	path, _, mainID, err := Publish(t.TempDir(), "runtime-main", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if mainID != "norns:math-main" {
		t.Fatalf("main=%s", mainID)
	}
	data, err := os.ReadFile(filepath.Join(path, "agents", "coordinator-for-math.md"))
	if err != nil {
		t.Fatal(err)
	}
	front, body, err := parseAgentMarkdown(data, "custom main")
	if err != nil {
		t.Fatal(err)
	}
	var fields struct {
		Tools      toolList `yaml:"tools"`
		MaxTurns   int      `yaml:"maxTurns"`
		Permission string   `yaml:"permissionMode"`
	}
	if err = front.Decode(&fields); err != nil {
		t.Fatal(err)
	}
	want := "Agent(norns:decider, norns:decider-for-math, norns:explorer, norns:explorer-for-math, norns:math-helper)"
	if !containsTool(fields.Tools, want) || fields.MaxTurns != 100 || fields.Permission != "plan" {
		t.Fatalf("frontmatter: %s", data)
	}
	for _, unwanted := range []string{"Agent(norns:coordinator", "norns:math-main", "{{NORNS_RUNTIME_CONFIGURATION}}"} {
		if strings.Contains(string(data), unwanted) {
			t.Fatalf("unexpected %s in %s", unwanted, data)
		}
	}
	if !strings.Contains(body, "--concurrency 7") || !strings.Contains(body, "99 秒") || !strings.Contains(string(data), "# preserve custom setting") {
		t.Fatalf("missing injected configuration or comment: %s", data)
	}
	before, _, _ := parseAgentMarkdown([]byte(original), source)
	assertFrontmatterPreserved(t, before, front, "tools")
	unchanged, _ := os.ReadFile(source)
	if string(unchanged) != original {
		t.Fatal("source main Agent was modified")
	}
	builtin, _ := assets.Generated.ReadFile("generated/plugins/cc/agents/coordinator.md")
	actual, _ := os.ReadFile(filepath.Join(path, "agents", "coordinator.md"))
	if string(actual) != string(builtin) {
		t.Fatal("unused generic Coordinator was rewritten")
	}
}

func TestDomainAgentPreservesUnknownFrontmatter(t *testing.T) {
	stage := t.TempDir()
	if err := os.MkdirAll(filepath.Join(stage, "agents"), 0700); err != nil {
		t.Fatal(err)
	}
	original := "---\nname: decider\ndescription: generic\ntools: Read, Bash\npermissionMode: plan\nmaxTurns: 200\nmemory: project\ncustom: &settings\n  nested: [one, two]\ncopy: *settings\n---\nOriginal role body.\n"
	writeTestFile(t, filepath.Join(stage, "agents", "decider.md"), original)
	if err := generateDomainAgent(stage, "decider.md", "decider-for-math.md", "decider-for-math", "domain description", "Additional domain knowledge."); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(stage, "agents", "decider-for-math.md"))
	if err != nil {
		t.Fatal(err)
	}
	before, _, err := parseAgentMarkdown([]byte(original), "source")
	if err != nil {
		t.Fatal(err)
	}
	after, body, err := parseAgentMarkdown(data, "generated")
	if err != nil {
		t.Fatal(err)
	}
	assertFrontmatterPreserved(t, before, after, "name", "description")
	if !strings.Contains(body, "Original role body.") || !strings.HasSuffix(body, "Additional domain knowledge.\n") {
		t.Fatalf("body=%s", body)
	}
}

func assertFrontmatterPreserved(t *testing.T, before, after *yaml.Node, changed ...string) {
	t.Helper()
	var a, b map[string]any
	if err := before.Decode(&a); err != nil {
		t.Fatal(err)
	}
	if err := after.Decode(&b); err != nil {
		t.Fatal(err)
	}
	for _, key := range changed {
		delete(a, key)
		delete(b, key)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("unrelated YAML fields changed:\nbefore=%#v\nafter=%#v", a, b)
	}
}

func TestCustomMainAgentRejectsMissingAgentToolAndRoleConflicts(t *testing.T) {
	for _, tc := range []struct{ name, tools string }{{"math-main", "Read, Bash"}, {"coordinator", "Read, Agent"}, {"decider", "Read, Agent"}} {
		t.Run(tc.name+tc.tools, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "main.md")
			writeTestFile(t, source, "---\nname: "+tc.name+"\ntools: "+tc.tools+"\n---\n{{NORNS_RUNTIME_CONFIGURATION}}\n")
			if _, _, _, err := Publish(t.TempDir(), "runtime", &config.Config{MainAgentFile: source}); err == nil {
				t.Fatal("invalid custom main Agent accepted")
			}
		})
	}
}

func TestMainAgentRewritingAcceptsCRLFAndRetainsBody(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.md")
	writeTestFile(t, path, "---\r\nname: main\r\ntools: Read, Agent\r\nmaxTurns: 42\r\n---\r\nBody\r\n")
	if err := rewriteMainAgent(path, []string{"norns:explorer"}, "configuration"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasSuffix(string(data), "Body\r\n") || !strings.Contains(string(data), "maxTurns: 42") {
		t.Fatalf("lost content: %s", data)
	}
}
