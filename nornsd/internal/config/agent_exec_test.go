package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func defaultAgentOnPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	name := "claude"
	if runtime.GOOS == "windows" {
		name += ".exe"
		t.Setenv("PATHEXT", ".EXE")
	}
	path := filepath.Join(dir, name)
	// Only executable discovery is exercised; this file is never launched.
	if err := os.WriteFile(path, []byte("test executable"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return path
}

func TestAgentExecEmptyListUsesDefault(t *testing.T) {
	want := defaultAgentOnPath(t)
	for _, field := range []string{"", "AGENT_EXEC: []\n"} {
		t.Run(field, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "nornsd.yaml")
			mustWrite(t, path, "AGENT_LLM_URL: x\nAGENT_LLM_KEY: y\nAGENT_LLM_MODEL: z\n"+field)
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cfg.AgentExec, []string{want}) || cfg.AgentExecExplicit() {
				t.Fatalf("expected default command %q, got %v (explicit=%v)", want, cfg.AgentExec, cfg.AgentExecExplicit())
			}
		})
	}
}

func TestRuntimeAgentExecEmptyListResetsDefault(t *testing.T) {
	want := defaultAgentOnPath(t)
	path := filepath.Join(t.TempDir(), "nornsd.yaml")
	wrapper := filepath.Join(t.TempDir(), "wrapper")
	argv, err := json.Marshal([]string{wrapper, "--custom"})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, "AGENT_LLM_URL: x\nAGENT_LLM_KEY: y\nAGENT_LLM_MODEL: z\nAGENT_EXEC: "+string(argv)+"\n")
	template, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{}`, `{"AGENT_EXEC":[]}`} {
		cfg, err := Effective(template, json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
		expected, explicit := template.AgentExec, true
		if raw != `{}` {
			expected, explicit = []string{want}, false
		}
		if !reflect.DeepEqual(cfg.AgentExec, expected) || cfg.AgentExecExplicit() != explicit {
			t.Fatalf("%s: command=%v explicit=%v", raw, cfg.AgentExec, cfg.AgentExecExplicit())
		}
	}
	if !reflect.DeepEqual(template.AgentExec, []string{wrapper, "--custom"}) || !template.AgentExecExplicit() {
		t.Fatalf("template mutated: %+v", template)
	}
}

func TestAgentExecDefaultRequiresClaudeOnPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, field := range []string{"", "AGENT_EXEC: []\n"} {
		path := filepath.Join(t.TempDir(), "nornsd.yaml")
		mustWrite(t, path, "AGENT_LLM_URL: x\nAGENT_LLM_KEY: y\nAGENT_LLM_MODEL: z\n"+field)
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "claude is not on PATH") {
			t.Fatalf("expected missing claude error, got %v", err)
		}
	}
}

func TestAgentExecRejectsNullAndEmptyArguments(t *testing.T) {
	for _, value := range []string{"null", `""`, `[""]`, `["/absolute/wrapper", ""]`} {
		path := filepath.Join(t.TempDir(), "nornsd.yaml")
		mustWrite(t, path, "AGENT_LLM_URL: x\nAGENT_LLM_KEY: y\nAGENT_LLM_MODEL: z\nAGENT_EXEC: "+value+"\n")
		if _, err := Load(path); err == nil {
			t.Fatalf("invalid AGENT_EXEC %s accepted", value)
		}
	}
}
