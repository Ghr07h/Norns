package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentExampleIsValid(t *testing.T) {
	cfg, err := Load("agent.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 || cfg.LLMURL == "" || cfg.LLMKey == "" || cfg.LLMModel == "" || !cfg.NornsEnable || cfg.NornsServerHost != "0.0.0.0" || cfg.NornsServerPort != 28081 || cfg.NornsServerToken == "" || cfg.NornsExplorationTimeoutSeconds != 1200 {
		t.Fatalf("agent example is incomplete: %#v", cfg)
	}
}

func TestSettingsUsesEmbeddedTemplateAndInjectsRuntimeValues(t *testing.T) {
	settings, err := Settings("cc", Config{
		LLMURL:       "https://example.test/v1",
		LLMKey:       `secret-\"value`,
		LLMModel:     "primary-model",
		LLMFastModel: "fast-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatal("settings env is not an object")
	}
	for key, want := range map[string]any{
		"ANTHROPIC_BASE_URL":             "https://example.test/v1",
		"ANTHROPIC_AUTH_TOKEN":           `secret-\"value`,
		"ANTHROPIC_MODEL":                "primary-model",
		"ANTHROPIC_SMALL_FAST_MODEL":     "fast-model",
		"CLAUDE_CODE_DISABLE_1M_CONTEXT": "1",
	} {
		if got := env[key]; got != want {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}
	if _, ok = settings["skillOverrides"].(map[string]any); !ok {
		t.Fatal("settings template did not provide skillOverrides")
	}
	if _, err = json.Marshal(settings); err != nil {
		t.Fatalf("generated settings are not valid JSON values: %v", err)
	}
}

func TestSettingsRejectsUnsupportedAgent(t *testing.T) {
	if _, err := Settings("unknown", Config{}); err == nil {
		t.Fatal("expected unsupported Agent settings generation to fail")
	}
}

func TestPartialMergePreservesExplicitEmptyAndFalse(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	override := filepath.Join(dir, "override.yaml")
	mustWrite(t, base, "VERSION: 1\nLLM_URL: https://example.test\nLLM_KEY: secret\nLLM_MODEL: model\nLLM_FAST_MODEL: fast\nNORNS_ENABLE: true\nNORNS_SERVER_HOST: 127.0.0.1\nNORNS_SERVER_PORT: 28081\nNORNS_SERVER_TOKEN: base-token\nNORNS_MAX_CONCURRENT_EXPLORATIONS: 7\nNORNS_EXPLORATION_TIMEOUT: 1800\nNORNS_ALLOW_HUMAN_INTERVENTION: true\nAGENT_EXEC: [claude]\n")
	mustWrite(t, override, "LLM_FAST_MODEL: \"\"\nNORNS_ENABLE: false\nNORNS_SERVER_HOST: 0.0.0.0\nNORNS_SERVER_PORT: 29000\nNORNS_SERVER_TOKEN: override-token\nNORNS_MAX_CONCURRENT_EXPLORATIONS: 2\nNORNS_EXPLORATION_TIMEOUT: 900\nNORNS_ALLOW_HUMAN_INTERVENTION: false\n")
	a, err := LoadPartial(base)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadPartial(override)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := Normalize(Merge(a, b))
	if err != nil {
		t.Fatal(err)
	}
	if merged.LLMFastModel != "model" {
		t.Fatalf("fast model = %q", merged.LLMFastModel)
	}
	if merged.NornsMaxConcurrentExplorations != 2 || merged.NornsExplorationTimeoutSeconds != 900 || merged.NornsAllowHumanIntervention {
		t.Fatalf("unexpected Norns config: %#v", merged)
	}
	if merged.NornsEnable || merged.NornsServerHost != "0.0.0.0" || merged.NornsServerPort != 29000 || merged.NornsServerToken != "override-token" {
		t.Fatalf("unexpected Norns server config: %#v", merged)
	}
	for _, key := range []string{"NORNS_ENABLE", "NORNS_SERVER_HOST", "NORNS_SERVER_PORT", "NORNS_SERVER_TOKEN"} {
		if !merged.Has(key) {
			t.Fatalf("merged config does not mark %s present", key)
		}
	}
}

func TestLoadRejectsInvalidNornsServerConfiguration(t *testing.T) {
	for name, field := range map[string]string{
		"empty host":       "NORNS_SERVER_HOST: \"\"",
		"invalid port":     "NORNS_SERVER_PORT: 65536",
		"empty token":      "NORNS_SERVER_TOKEN: \"\"",
		"token whitespace": "NORNS_SERVER_TOKEN: \"two words\"",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "agent.yaml")
			mustWrite(t, path, "VERSION: 1\nLLM_URL: x\nLLM_KEY: y\nLLM_MODEL: z\n"+field+"\n")
			if _, err := Load(path); err == nil {
				t.Fatalf("expected %s to fail", field)
			}
		})
	}
}

func TestPersistedConfigPreservesNewFieldPresence(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.yaml")
	mustWrite(t, oldPath, "VERSION: 1\nLLM_URL: x\nLLM_KEY: y\nLLM_MODEL: z\n")
	oldConfig, err := Load(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := marshalPersisted(&oldConfig)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, key := range []string{"NORNS_ENABLE", "NORNS_SERVER_HOST", "NORNS_SERVER_PORT", "NORNS_SERVER_TOKEN"} {
		if strings.Contains(text, key+":") {
			t.Fatalf("persisted old config unexpectedly contains %s:\n%s", key, text)
		}
	}

	newPath := filepath.Join(dir, "new.yaml")
	mustWrite(t, newPath, "VERSION: 1\nLLM_URL: x\nLLM_KEY: y\nLLM_MODEL: z\nNORNS_ENABLE: false\nNORNS_SERVER_HOST: 127.0.0.1\nNORNS_SERVER_PORT: 28081\nNORNS_SERVER_TOKEN: token\n")
	newConfig, err := Load(newPath)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = marshalPersisted(&newConfig)
	if err != nil {
		t.Fatal(err)
	}
	text = string(encoded)
	if !strings.Contains(text, "NORNS_ENABLE: false") {
		t.Fatalf("persisted new config does not retain explicit NORNS_ENABLE: false:\n%s", text)
	}
	for _, want := range []string{"NORNS_SERVER_HOST", "NORNS_SERVER_PORT", "NORNS_SERVER_TOKEN"} {
		if strings.Contains(text, want+":") {
			t.Fatalf("persisted Runtime config must not contain server field %s:\n%s", want, text)
		}
	}
}

func TestLoadAndMergeCustomPluginDirs(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first-plugin")
	second := filepath.Join(dir, "second-plugin")
	for _, path := range []string{first, second} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	basePath := filepath.Join(dir, "base.yaml")
	mustWrite(t, basePath, "VERSION: 1\nLLM_URL: x\nLLM_KEY: y\nLLM_MODEL: z\nCUSTOM_PLUGIN_DIRS:\n  - first-plugin\n  - second-plugin\n")
	base, err := Load(basePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(base.CustomPluginDirs) != 2 || base.CustomPluginDirs[0] != first || base.CustomPluginDirs[1] != second {
		t.Fatalf("unexpected custom Plugin directories: %#v", base.CustomPluginDirs)
	}

	overridePath := filepath.Join(dir, "override.yaml")
	mustWrite(t, overridePath, "CUSTOM_PLUGIN_DIRS: []\n")
	override, err := LoadPartial(overridePath)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := Normalize(Merge(base, override))
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.CustomPluginDirs) != 0 {
		t.Fatalf("explicit empty CUSTOM_PLUGIN_DIRS did not clear the inherited list: %#v", merged.CustomPluginDirs)
	}
}

func TestLoadRejectsInvalidCustomPluginDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	mustWrite(t, path, "VERSION: 1\nLLM_URL: x\nLLM_KEY: y\nLLM_MODEL: z\nCUSTOM_PLUGIN_DIRS:\n  - missing\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected missing CUSTOM_PLUGIN_DIRS entry to fail")
	}
}

func TestLoadRequiresCustomPluginDirsList(t *testing.T) {
	for _, value := range []string{"null", `"plugin"`} {
		path := filepath.Join(t.TempDir(), "agent.yaml")
		mustWrite(t, path, "VERSION: 1\nLLM_URL: x\nLLM_KEY: y\nLLM_MODEL: z\nCUSTOM_PLUGIN_DIRS: "+value+"\n")
		if _, err := Load(path); err == nil {
			t.Fatalf("expected CUSTOM_PLUGIN_DIRS value %s to fail", value)
		}
	}
}

func TestNornsConcurrencyExplicitZeroRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	mustWrite(t, path, "VERSION: 1\nLLM_URL: x\nLLM_KEY: y\nLLM_MODEL: z\nNORNS_MAX_CONCURRENT_EXPLORATIONS: 0\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected explicit zero to fail")
	}
}

func TestNornsExplorationTimeoutDefaultsAndRejectsExplicitZero(t *testing.T) {
	dir := t.TempDir()
	defaultPath := filepath.Join(dir, "default.yaml")
	mustWrite(t, defaultPath, "VERSION: 1\nLLM_URL: x\nLLM_KEY: y\nLLM_MODEL: z\n")
	cfg, err := Load(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NornsExplorationTimeoutSeconds != 1200 {
		t.Fatalf("exploration timeout = %d, want 1200", cfg.NornsExplorationTimeoutSeconds)
	}

	zeroPath := filepath.Join(dir, "zero.yaml")
	mustWrite(t, zeroPath, "VERSION: 1\nLLM_URL: x\nLLM_KEY: y\nLLM_MODEL: z\nNORNS_EXPLORATION_TIMEOUT: 0\n")
	if _, err = Load(zeroPath); err == nil {
		t.Fatal("expected explicit zero NORNS_EXPLORATION_TIMEOUT to fail")
	}
}

func TestLoadDomainConfiguration(t *testing.T) {
	dir := t.TempDir()
	domainDir := filepath.Join(dir, "domain")
	if err := os.MkdirAll(filepath.Join(domainDir, "info"), 0700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(domainDir, "info", "decider.md"), "decider knowledge\n")
	mustWrite(t, filepath.Join(domainDir, "info", "explorer.md"), "explorer knowledge\n")
	mustWrite(t, filepath.Join(domainDir, "meta.json"), `{
  "domain_name": "code_review",
  "domain_decider_description": "Decide code review directions",
  "domain_decider_extra_info_file": "info/decider.md",
  "domain_explorer_description": "Explore code review evidence",
  "domain_explorer_extra_info_file": "info/explorer.md"
}`)
	setting := filepath.Join(dir, "agent.yaml")
	mustWrite(t, setting, "VERSION: 1\nLLM_URL: x\nLLM_KEY: y\nLLM_MODEL: z\nNORNS_CUSTOM_DOMAIN_DIR: domain\n")

	cfg, err := Load(setting)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Domain == nil || cfg.Domain.Name != "code_review" {
		t.Fatalf("unexpected domain: %#v", cfg.Domain)
	}
	if cfg.Domain.DeciderDescription != "Decide code review directions" || cfg.Domain.DeciderExtraInfo != "decider knowledge\n" {
		t.Fatalf("unexpected decider domain data: %#v", cfg.Domain)
	}
	if cfg.Domain.ExplorerDescription != "Explore code review evidence" || cfg.Domain.ExplorerExtraInfo != "explorer knowledge\n" {
		t.Fatalf("unexpected explorer domain data: %#v", cfg.Domain)
	}
	if cfg.NornsCustomDomainDir != domainDir {
		t.Fatalf("domain path = %q, want %q", cfg.NornsCustomDomainDir, domainDir)
	}
}

func TestLoadDomainRejectsExplorationFieldTypo(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "meta.json"), `{
  "domain_name": "review",
  "domain_decider_description": "decider",
  "domain_decider_extra_info_file": "decider.md",
  "domain_exploration_description": "explorer",
  "domain_exploration_extra_info_file": "explorer.md"
}`)

	_, err := LoadDomain(dir)
	if err == nil || !strings.Contains(err.Error(), `unknown field "domain_exploration_description"`) {
		t.Fatalf("expected old explorer field spelling to be rejected, got %v", err)
	}
}

func TestLoadDomainRejectsUnsafeName(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "decider.md"), "decider")
	mustWrite(t, filepath.Join(dir, "explorer.md"), "explorer")
	mustWrite(t, filepath.Join(dir, "meta.json"), `{
  "domain_name": "../escape",
  "domain_decider_description": "decider",
  "domain_decider_extra_info_file": "decider.md",
  "domain_explorer_description": "explorer",
  "domain_explorer_extra_info_file": "explorer.md"
}`)
	if _, err := LoadDomain(dir); err == nil {
		t.Fatal("expected unsafe domain_name to fail")
	}
}

func TestLoadDomainRejectsExtraInfoOutsideDomain(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "domain")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(parent, "outside.md"), "outside")
	mustWrite(t, filepath.Join(dir, "inside.md"), "inside")
	mustWrite(t, filepath.Join(dir, "meta.json"), `{
  "domain_name": "review",
  "domain_decider_description": "decider",
  "domain_decider_extra_info_file": "../outside.md",
  "domain_explorer_description": "explorer",
  "domain_explorer_extra_info_file": "inside.md"
}`)
	if _, err := LoadDomain(dir); err == nil {
		t.Fatal("expected extra info path outside domain to fail")
	}
}

func TestPublishCustomBuildsRuntimePluginWithoutWritingSubjectClaudeDirectory(t *testing.T) {
	subject := t.TempDir()
	sourceRoot := t.TempDir()
	agents := filepath.Join(sourceRoot, "agents")
	skills := filepath.Join(sourceRoot, "skills")
	if err := os.MkdirAll(agents, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skills, "review"), 0700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(agents, "custom.md"), "custom agent")
	mustWrite(t, filepath.Join(skills, "review", "SKILL.md"), "custom skill")
	pluginPath, hash, err := PublishCustom(subject, "runtime-1", &Config{CustomAgentsDir: agents, CustomSkillsDir: skills})
	if err != nil {
		t.Fatal(err)
	}
	if pluginPath != filepath.Join(subject, ".nornsd", "runtimes", "runtime-1", "artifacts", "custom-plugin") || hash == "" {
		t.Fatalf("unexpected publication: path=%q hash=%q", pluginPath, hash)
	}
	for _, path := range []string{
		filepath.Join(pluginPath, ".claude-plugin", "plugin.json"),
		filepath.Join(pluginPath, "agents", "custom.md"),
		filepath.Join(pluginPath, "skills", "review", "SKILL.md"),
		filepath.Join(pluginPath, "plugin.manifest.json"),
	} {
		if _, err = os.Stat(path); err != nil {
			t.Fatalf("missing generated plugin file %s: %v", path, err)
		}
	}
	if _, err = os.Stat(filepath.Join(subject, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("PublishCustom wrote Subject .claude directory: %v", err)
	}
	var manifest CustomPluginManifest
	manifestBytes, err := os.ReadFile(filepath.Join(pluginPath, "plugin.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".claude-plugin/plugin.json", "agents/custom.md", "skills/review/SKILL.md"} {
		if manifest.Files[name] == "" {
			t.Fatalf("manifest missing %s", name)
		}
	}
}

func TestPublishCustomWithoutSourcesDoesNotBuildPlugin(t *testing.T) {
	path, hash, err := PublishCustom(t.TempDir(), "runtime-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if path != "" || hash != "" {
		t.Fatalf("unexpected empty custom plugin publication: path=%q hash=%q", path, hash)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
