package config

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func resourceTestBase(t *testing.T) Config {
	t.Helper()
	return Config{AgentType: "cc", LLMURL: "https://example.test", LLMKey: "secret", LLMModel: "model", AgentExec: []string{filepath.Join(t.TempDir(), "agent")}}
}

func TestResourceModesRejectWrongFamily(t *testing.T) {
	cases := []struct {
		key   string
		apply func(*Config)
	}{
		{"AGENT_NORNS_EXTENDS_AGENTS_DIR", func(c *Config) { c.NornsExtendsAgentsDir = t.TempDir() }},
		{"AGENT_NORNS_CUSTOM_AGENTS_DIR", func(c *Config) { c.NornsCustomAgentsDir = t.TempDir() }},
		{"AGENT_NORNS_CUSTOM_SKILLS_DIR", func(c *Config) { c.NornsCustomSkillsDir = t.TempDir() }},
		{"AGENT_CUSTOM_AGENTS_DIR", func(c *Config) { c.NornsEnable = true; c.CustomAgentsDir = t.TempDir() }},
		{"AGENT_CUSTOM_SKILLS_DIR", func(c *Config) { c.NornsEnable = true; c.CustomSkillsDir = t.TempDir() }},
		{"AGENT_CUSTOM_PLUGIN_DIRS", func(c *Config) { c.NornsEnable = true; c.CustomPluginDirs = []string{t.TempDir()} }},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			c := resourceTestBase(t)
			tc.apply(&c)
			if _, err := Normalize(c); err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("wrong-mode field accepted: %v", err)
			}
		})
	}
	for _, enabled := range []bool{false, true} {
		c := resourceTestBase(t)
		c.NornsEnable = enabled
		if _, err := Normalize(c); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRuntimeModeOverridesRequireClearingInheritedResources(t *testing.T) {
	template := resourceTestBase(t)
	template.NornsEnable = true
	template.NornsCustomAgentsDir = t.TempDir()
	template.NornsCustomSkillsDir = t.TempDir()
	if _, err := Effective(template, json.RawMessage(`{"AGENT_NORNS_ENABLE":false}`)); err == nil {
		t.Fatal("switch silently ignored inherited Norns resources")
	}
	c, err := Effective(template, json.RawMessage(`{"AGENT_NORNS_ENABLE":false,"AGENT_NORNS_CUSTOM_AGENTS_DIR":"","AGENT_NORNS_CUSTOM_SKILLS_DIR":""}`))
	if err != nil || c.NornsEnable || c.NornsCustomAgentsDir != "" || c.NornsCustomSkillsDir != "" {
		t.Fatalf("cannot clear resources: %+v %v", c, err)
	}
	ordinary := resourceTestBase(t)
	ordinary.CustomPluginDirs = []string{t.TempDir()}
	ordinary.CustomAgentsDir = t.TempDir()
	if _, err := Effective(ordinary, json.RawMessage(`{"AGENT_NORNS_ENABLE":true}`)); err == nil {
		t.Fatal("Norns mode accepted inherited extra Plugin")
	}
	c, err = Effective(ordinary, json.RawMessage(`{"AGENT_NORNS_ENABLE":true,"AGENT_CUSTOM_AGENTS_DIR":"","AGENT_CUSTOM_PLUGIN_DIRS":[]}`))
	if err != nil || !c.NornsEnable || c.CustomAgentsDir != "" || len(c.CustomPluginDirs) != 0 {
		t.Fatalf("cannot clear ordinary resources: %+v %v", c, err)
	}
	if !template.NornsEnable || template.NornsCustomAgentsDir == "" || len(ordinary.CustomPluginDirs) != 1 {
		t.Fatal("mode override mutated template")
	}
}

func TestResourceOverrideChecksExplicitModeAndDefersUnknownTemplate(t *testing.T) {
	dir := t.TempDir()
	for _, values := range []map[string]any{
		{"AGENT_NORNS_ENABLE": false, "AGENT_NORNS_CUSTOM_AGENTS_DIR": dir},
		{"AGENT_NORNS_ENABLE": true, "AGENT_CUSTOM_PLUGIN_DIRS": []string{dir}},
		{"AGENT_NORNS_CUSTOM_AGENTS_DIR": dir, "AGENT_CUSTOM_SKILLS_DIR": dir},
	} {
		raw, _ := json.Marshal(values)
		if _, err := ParseOverride(raw); err == nil {
			t.Fatalf("invalid partial mode accepted: %s", raw)
		}
	}
	raw, _ := json.Marshal(map[string]any{"AGENT_NORNS_CUSTOM_AGENTS_DIR": dir})
	if _, err := ParseOverride(raw); err != nil {
		t.Fatalf("override prematurely assumed ordinary template: %v", err)
	}
	template := resourceTestBase(t)
	template.NornsEnable = true
	if _, err := Effective(template, raw); err != nil {
		t.Fatal(err)
	}
	template.NornsEnable = false
	if _, err := Effective(template, raw); err == nil {
		t.Fatal("claim accepted wrong-mode override")
	}
	for _, key := range []string{"AGENT_NORNS_CUSTOM_DOMAIN_DIR", "AGENT_NORNS_CUSTOM_PLUGIN_DIR", "AGENT_NORNS_CUSTOM_PLUGIN_DIRS"} {
		raw, _ := json.Marshal(map[string]string{key: dir})
		if _, err := ParseOverride(raw); err == nil {
			t.Fatalf("removed/unknown field accepted: %s", key)
		}
	}
}
