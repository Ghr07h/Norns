package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeOverridePresenceAndIsolation(t *testing.T) {
	template := Config{AgentType: "cc", LLMURL: "https://example.test", LLMKey: "secret", LLMModel: "base", AgentExec: []string{"/bin/sh"}, NornsEnable: false, NornsAllowHumanIntervention: true, CustomPluginDirs: []string{"/plugins/a", "/plugins/b"}}
	for _, raw := range []string{``, `null`, `{}`} {
		cfg, err := Effective(template, json.RawMessage(raw))
		if err != nil || cfg.LLMModel != "base" || cfg.NornsEnable {
			t.Fatalf("%s: %+v %v", raw, cfg, err)
		}
	}
	cfg, err := Effective(template, json.RawMessage(`{"AGENT_NORNS_ENABLE":false,"AGENT_NORNS_ALLOW_HUMAN_INTERVENTION":false,"AGENT_CUSTOM_PLUGIN_DIRS":[],"AGENT_LLM_MODEL":"new"}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NornsEnable || cfg.NornsAllowHumanIntervention || len(cfg.CustomPluginDirs) != 0 || cfg.LLMModel != "new" {
		t.Fatalf("lost explicit overrides: %+v", cfg)
	}
	if template.NornsEnable || len(template.CustomPluginDirs) != 2 || template.LLMModel != "base" || template.Has("AGENT_LLM_MODEL") {
		t.Fatalf("template mutated: %+v", template)
	}
	cfg, err = Effective(template, json.RawMessage(`{"AGENT_CUSTOM_PLUGIN_DIRS":["/plugins/c"]}`))
	if err != nil || len(cfg.CustomPluginDirs) != 1 || cfg.CustomPluginDirs[0] != "/plugins/c" {
		t.Fatalf("array not replaced: %+v %v", cfg, err)
	}
}

func TestRuntimeOverrideValidationBeforeQueue(t *testing.T) {
	for _, raw := range []string{
		`[]`, `"x"`, `{"VERSION":3}`, `{"SERVICE_PORT":1234}`, `{"AGENT_BAD":true}`,
		`{"AGENT_NORNS_ENABLE":null}`, `{"AGENT_NORNS_ENABLE":"false"}`, `{"AGENT_LLM_MODEL":false}`,
		`{"AGENT_EXEC":["claude"]}`, `{"AGENT_EXEC":[""]}`, `{"AGENT_EXEC":null}`, `{"AGENT_TYPE":""}`, `{"AGENT_LLM_KEY":""}`,
		`{"AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS":0}`, `{"AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS":-1}`,
		`{"AGENT_CUSTOM_PLUGIN_DIRS":["relative"]}`, `{"AGENT_CUSTOM_AGENTS_DIR":"./agents"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseOverride(json.RawMessage(raw)); err == nil {
				t.Fatal("invalid override accepted")
			}
		})
	}
}

func TestFrozenTemplatesAndExternalCopies(t *testing.T) {
	control := t.TempDir()
	templates := []Config{{LLMModel: "a", LLMKey: "secret-a"}, {LLMModel: "b"}, {LLMModel: "c"}}
	if err := SaveTemplates(control, templates); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"nornsd.yaml", "nornsd2.yaml", "nornsd3.yaml"} {
		cfg, err := LoadPartial(filepath.Join(control, "configs", name))
		if err != nil || cfg.LLMModel != templates[i].LLMModel {
			t.Fatalf("template %s: %+v %v", name, cfg, err)
		}
	}
	if err := SaveTemplates(control, templates[:1]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(control, "configs", "nornsd2.yaml")); !os.IsNotExist(err) {
		t.Fatalf("stale template: %v", err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "resource.txt"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(t.TempDir(), "subject_stamp")
	paths, err := PublishExternal(workspace, "runtime", &Config{CustomPluginDirs: []string{source}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "resource.txt"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(paths[0], "resource.txt"))
	if err != nil || string(b) != "old" {
		t.Fatalf("not an independent copy: %s %v", b, err)
	}
	var manifest CustomPluginManifest
	b, err = os.ReadFile(filepath.Join(paths[0], "plugin.manifest.json"))
	if err != nil || json.Unmarshal(b, &manifest) != nil || manifest.Files["resource.txt"] == "" {
		t.Fatalf("missing integrity manifest: %s %v", b, err)
	}
}
