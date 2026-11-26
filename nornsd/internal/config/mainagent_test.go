package config

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainAgentAndReuseOverrides(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		base := resourceTestBase(t)
		base.NornsEnable = enabled
		main := filepath.Join(t.TempDir(), "main.md")
		raw, _ := json.Marshal(map[string]any{"AGENT_MAINAGENT_FILE": main})
		c, err := Effective(base, raw)
		if err != nil || c.MainAgentFile != main {
			t.Fatalf("main file in mode %v: %+v %v", enabled, c, err)
		}
		c, err = Effective(c, json.RawMessage(`{"AGENT_MAINAGENT_FILE":""}`))
		if err != nil || c.MainAgentFile != "" {
			t.Fatalf("cannot clear main: %+v %v", c, err)
		}
		reuse := t.TempDir()
		raw, _ = json.Marshal(map[string]any{"AGENT_REUSE_RUNTIME_DIR": reuse, "AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS": 0, "AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS": -1, "AGENT_LLM_MODEL": "new-model"})
		c, err = Effective(base, raw)
		if err != nil || c.ReuseRuntimeDir != reuse || c.LLMModel != "new-model" {
			t.Fatalf("reuse in mode %v: %+v %v", enabled, c, err)
		}
		c, err = Effective(c, json.RawMessage(`{"AGENT_REUSE_RUNTIME_DIR":"","AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS":2,"AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS":30}`))
		if err != nil || c.ReuseRuntimeDir != "" || c.NornsMaxConcurrentExplorations != 2 {
			t.Fatalf("cannot clear reuse: %+v %v", c, err)
		}
	}
}

func TestReuseRejectsInheritedPluginResources(t *testing.T) {
	fields := []string{"AGENT_MAINAGENT_FILE", "AGENT_NORNS_EXTENDS_AGENTS_DIR", "AGENT_NORNS_CUSTOM_AGENTS_DIR", "AGENT_NORNS_CUSTOM_SKILLS_DIR", "AGENT_CUSTOM_AGENTS_DIR", "AGENT_CUSTOM_SKILLS_DIR", "AGENT_CUSTOM_PLUGIN_DIRS"}
	for _, key := range fields {
		t.Run(key, func(t *testing.T) {
			base := resourceTestBase(t)
			value := any(t.TempDir())
			if key == "AGENT_CUSTOM_PLUGIN_DIRS" {
				value = []string{value.(string)}
			}
			layer, _ := json.Marshal(map[string]any{key: value})
			override, err := ParseOverride(layer)
			if err != nil {
				t.Fatal(err)
			}
			base = Merge(base, override)
			raw, _ := json.Marshal(map[string]any{"AGENT_REUSE_RUNTIME_DIR": t.TempDir()})
			if _, err = Effective(base, raw); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("inherited resource conflict ignored: %v", err)
			}
			values := map[string]any{"AGENT_REUSE_RUNTIME_DIR": t.TempDir(), key: value}
			raw, _ = json.Marshal(values)
			if _, err = ParseOverride(raw); err == nil {
				t.Fatal("explicit conflict accepted")
			}
			values[key] = ""
			if key == "AGENT_CUSTOM_PLUGIN_DIRS" {
				values[key] = []string{}
			}
			raw, _ = json.Marshal(values)
			if _, err = Effective(base, raw); err != nil {
				t.Fatalf("explicit clearing failed: %v", err)
			}
		})
	}
}

func TestMainAgentReuseRejectInvalidPathsAndRemovedNames(t *testing.T) {
	for _, key := range []string{"AGENT_MAINAGENT_FILE", "AGENT_REUSE_RUNTIME_DIR"} {
		raw, _ := json.Marshal(map[string]string{key: "./relative"})
		if _, err := ParseOverride(raw); err == nil {
			t.Fatalf("relative path accepted for %s", key)
		}
	}
	for _, key := range []string{"AGENT_NORNS_CUSTOM_MAINAGENT_FILE", "AGENT_NORNS_REUSE_PLUGINS_DIR"} {
		raw, _ := json.Marshal(map[string]string{key: t.TempDir()})
		if _, err := ParseOverride(raw); err == nil {
			t.Fatalf("removed name accepted: %s", key)
		}
	}
}
