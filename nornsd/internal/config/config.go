package config

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Ghr07h/norns/nornsd/internal/version"
)

//go:embed templates
var agentSettingsTemplates embed.FS

type Config struct {
	Version                        int      `yaml:"VERSION" json:"VERSION"`
	LLMURL                         string   `yaml:"LLM_URL" json:"LLM_URL"`
	LLMKey                         string   `yaml:"LLM_KEY" json:"-"`
	LLMModel                       string   `yaml:"LLM_MODEL" json:"LLM_MODEL"`
	LLMFastModel                   string   `yaml:"LLM_FAST_MODEL" json:"LLM_FAST_MODEL"`
	NornsEnable                    bool     `yaml:"NORNS_ENABLE" json:"NORNS_ENABLE"`
	NornsServerHost                string   `yaml:"NORNS_SERVER_HOST" json:"NORNS_SERVER_HOST,omitempty"`
	NornsServerPort                int      `yaml:"NORNS_SERVER_PORT" json:"NORNS_SERVER_PORT,omitempty"`
	NornsServerToken               string   `yaml:"NORNS_SERVER_TOKEN" json:"-"`
	NornsMaxConcurrentExplorations int      `yaml:"NORNS_MAX_CONCURRENT_EXPLORATIONS" json:"NORNS_MAX_CONCURRENT_EXPLORATIONS"`
	NornsExplorationTimeoutSeconds int      `yaml:"NORNS_EXPLORATION_TIMEOUT" json:"NORNS_EXPLORATION_TIMEOUT"`
	NornsAllowHumanIntervention    bool     `yaml:"NORNS_ALLOW_HUMAN_INTERVENTION" json:"NORNS_ALLOW_HUMAN_INTERVENTION"`
	NornsCustomDomainDir           string   `yaml:"NORNS_CUSTOM_DOMAIN_DIR" json:"NORNS_CUSTOM_DOMAIN_DIR,omitempty"`
	NornsCustomSkillsDir           string   `yaml:"NORNS_CUSTOM_SKILLS_DIR" json:"NORNS_CUSTOM_SKILLS_DIR,omitempty"`
	NornsCustomAgentsDir           string   `yaml:"NORNS_CUSTOM_AGENTS_DIR" json:"NORNS_CUSTOM_AGENTS_DIR,omitempty"`
	CustomAgentsDir                string   `yaml:"CUSTOM_AGENTS_DIR" json:"CUSTOM_AGENTS_DIR,omitempty"`
	CustomSkillsDir                string   `yaml:"CUSTOM_SKILLS_DIR" json:"CUSTOM_SKILLS_DIR,omitempty"`
	CustomPluginDirs               []string `yaml:"CUSTOM_PLUGIN_DIRS" json:"CUSTOM_PLUGIN_DIRS,omitempty"`
	AgentExec                      []string `yaml:"AGENT_EXEC" json:"AGENT_EXEC"`
	Source                         string   `yaml:"-" json:"-"`
	Domain                         *Domain  `yaml:"-" json:"-"`
	present                        map[string]bool
	agentExecExplicit              bool
}

type persistedConfig struct {
	Version                        int      `yaml:"VERSION"`
	LLMURL                         string   `yaml:"LLM_URL"`
	LLMKey                         string   `yaml:"LLM_KEY"`
	LLMModel                       string   `yaml:"LLM_MODEL"`
	LLMFastModel                   string   `yaml:"LLM_FAST_MODEL"`
	NornsEnable                    *bool    `yaml:"NORNS_ENABLE,omitempty"`
	NornsMaxConcurrentExplorations int      `yaml:"NORNS_MAX_CONCURRENT_EXPLORATIONS"`
	NornsExplorationTimeoutSeconds int      `yaml:"NORNS_EXPLORATION_TIMEOUT"`
	NornsAllowHumanIntervention    bool     `yaml:"NORNS_ALLOW_HUMAN_INTERVENTION"`
	NornsCustomDomainDir           string   `yaml:"NORNS_CUSTOM_DOMAIN_DIR"`
	NornsCustomSkillsDir           string   `yaml:"NORNS_CUSTOM_SKILLS_DIR"`
	NornsCustomAgentsDir           string   `yaml:"NORNS_CUSTOM_AGENTS_DIR"`
	CustomAgentsDir                string   `yaml:"CUSTOM_AGENTS_DIR"`
	CustomSkillsDir                string   `yaml:"CUSTOM_SKILLS_DIR"`
	CustomPluginDirs               []string `yaml:"CUSTOM_PLUGIN_DIRS"`
	AgentExec                      []string `yaml:"AGENT_EXEC"`
}

func Load(path string) (Config, error) {
	out, err := load(path)
	if err != nil {
		return out, err
	}
	return normalize(out)
}

// LoadPartial parses one merge layer. Required fields are checked only after
// all layers have been merged, while path-valued fields are still resolved
// relative to the file that declared them.
func LoadPartial(path string) (Config, error) { return load(path) }

func load(path string) (Config, error) {
	var out Config
	b, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	var node yaml.Node
	if err = yaml.Unmarshal(b, &node); err != nil {
		return out, fmt.Errorf("invalid setting: %w", err)
	}
	out.present = map[string]bool{}
	if len(node.Content) > 0 && len(node.Content[0].Content) > 0 {
		mapping := node.Content[0]
		if mapping.Kind != yaml.MappingNode {
			return out, fmt.Errorf("invalid setting: expected a YAML object")
		}
		for i := 0; i < len(mapping.Content); i += 2 {
			key := mapping.Content[i].Value
			out.present[key] = true
			if key == "CUSTOM_PLUGIN_DIRS" && mapping.Content[i+1].Kind != yaml.SequenceNode {
				return out, fmt.Errorf("invalid setting: CUSTOM_PLUGIN_DIRS must be a YAML list")
			}
		}
	}
	decoder := yaml.NewDecoder(bytes.NewReader(b))
	decoder.KnownFields(true)
	if err = decoder.Decode(&out); err != nil {
		return out, fmt.Errorf("invalid setting: %w", err)
	}
	var extra any
	if err = decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return out, fmt.Errorf("multiple YAML documents are not allowed")
		}
		return out, err
	}
	out.Source = path
	out.agentExecExplicit = out.present["AGENT_EXEC"] && len(out.AgentExec) > 0
	base := filepath.Dir(path)
	for _, pair := range []struct {
		name  string
		value *string
	}{{"NORNS_CUSTOM_DOMAIN_DIR", &out.NornsCustomDomainDir}, {"NORNS_CUSTOM_SKILLS_DIR", &out.NornsCustomSkillsDir}, {"NORNS_CUSTOM_AGENTS_DIR", &out.NornsCustomAgentsDir}, {"CUSTOM_AGENTS_DIR", &out.CustomAgentsDir}, {"CUSTOM_SKILLS_DIR", &out.CustomSkillsDir}} {
		if *pair.value == "" {
			continue
		}
		resolved := *pair.value
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(base, resolved)
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return out, err
		}
		info, statErr := os.Stat(resolved)
		if statErr != nil || !info.IsDir() {
			return out, fmt.Errorf("%s must be a readable directory", pair.name)
		}
		*pair.value = resolved
	}
	for i, value := range out.CustomPluginDirs {
		if strings.TrimSpace(value) == "" {
			return out, fmt.Errorf("CUSTOM_PLUGIN_DIRS cannot contain an empty path")
		}
		resolved := value
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(base, resolved)
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return out, err
		}
		info, statErr := os.Stat(resolved)
		if statErr != nil || !info.IsDir() {
			return out, fmt.Errorf("CUSTOM_PLUGIN_DIRS[%d] must be a readable directory", i)
		}
		out.CustomPluginDirs[i] = resolved
	}
	return out, nil
}

func Merge(base, override Config) Config {
	has := func(key string, fallback bool) bool {
		if override.present != nil {
			return override.present[key]
		}
		return fallback
	}
	if has("VERSION", override.Version != 0) {
		base.Version = override.Version
	}
	if has("LLM_URL", override.LLMURL != "") {
		base.LLMURL = override.LLMURL
	}
	if has("LLM_KEY", override.LLMKey != "") {
		base.LLMKey = override.LLMKey
	}
	if has("LLM_MODEL", override.LLMModel != "") {
		base.LLMModel = override.LLMModel
	}
	if has("LLM_FAST_MODEL", override.LLMFastModel != "") {
		base.LLMFastModel = override.LLMFastModel
	}
	if has("NORNS_ENABLE", override.NornsEnable) {
		base.NornsEnable = override.NornsEnable
	}
	if has("NORNS_SERVER_HOST", override.NornsServerHost != "") {
		base.NornsServerHost = override.NornsServerHost
	}
	if has("NORNS_SERVER_PORT", override.NornsServerPort != 0) {
		base.NornsServerPort = override.NornsServerPort
	}
	if has("NORNS_SERVER_TOKEN", override.NornsServerToken != "") {
		base.NornsServerToken = override.NornsServerToken
	}
	if has("NORNS_CUSTOM_DOMAIN_DIR", override.NornsCustomDomainDir != "") {
		base.NornsCustomDomainDir = override.NornsCustomDomainDir
		base.Domain = nil
	}
	if has("NORNS_CUSTOM_SKILLS_DIR", override.NornsCustomSkillsDir != "") {
		base.NornsCustomSkillsDir = override.NornsCustomSkillsDir
	}
	if has("NORNS_CUSTOM_AGENTS_DIR", override.NornsCustomAgentsDir != "") {
		base.NornsCustomAgentsDir = override.NornsCustomAgentsDir
	}
	if has("CUSTOM_AGENTS_DIR", override.CustomAgentsDir != "") {
		base.CustomAgentsDir = override.CustomAgentsDir
	}
	if has("CUSTOM_SKILLS_DIR", override.CustomSkillsDir != "") {
		base.CustomSkillsDir = override.CustomSkillsDir
	}
	if has("CUSTOM_PLUGIN_DIRS", len(override.CustomPluginDirs) > 0) {
		base.CustomPluginDirs = append([]string(nil), override.CustomPluginDirs...)
	}
	if has("NORNS_MAX_CONCURRENT_EXPLORATIONS", override.NornsMaxConcurrentExplorations != 0) {
		base.NornsMaxConcurrentExplorations = override.NornsMaxConcurrentExplorations
	}
	if has("NORNS_EXPLORATION_TIMEOUT", override.NornsExplorationTimeoutSeconds != 0) {
		base.NornsExplorationTimeoutSeconds = override.NornsExplorationTimeoutSeconds
	}
	if has("NORNS_ALLOW_HUMAN_INTERVENTION", override.NornsAllowHumanIntervention) {
		base.NornsAllowHumanIntervention = override.NornsAllowHumanIntervention
	}
	if has("AGENT_EXEC", len(override.AgentExec) > 0) {
		base.AgentExec = append([]string(nil), override.AgentExec...)
	}
	for key := range override.present {
		if base.present == nil {
			base.present = map[string]bool{}
		}
		base.present[key] = true
	}
	if override.present["AGENT_EXEC"] {
		base.agentExecExplicit = override.agentExecExplicit
	}
	if override.Source != "" {
		base.Source = override.Source
	}
	return base
}

func Normalize(c Config) (Config, error) { return normalize(c) }
func (c Config) AgentExecExplicit() bool { return c.agentExecExplicit }
func (c Config) Has(key string) bool     { return c.present != nil && c.present[key] }

func normalize(out Config) (Config, error) {
	if out.Version != version.AgentConfigVersion {
		return out, fmt.Errorf("unsupported agent config version %d", out.Version)
	}
	if strings.TrimSpace(out.LLMURL) == "" || strings.TrimSpace(out.LLMKey) == "" || strings.TrimSpace(out.LLMModel) == "" {
		return out, fmt.Errorf("LLM_URL, LLM_KEY and LLM_MODEL are required")
	}
	if out.LLMFastModel == "" {
		out.LLMFastModel = out.LLMModel
	}
	if out.present["NORNS_SERVER_HOST"] && strings.TrimSpace(out.NornsServerHost) == "" {
		return out, fmt.Errorf("NORNS_SERVER_HOST must not be empty")
	}
	if out.present["NORNS_SERVER_PORT"] && (out.NornsServerPort < 1 || out.NornsServerPort > 65535) {
		return out, fmt.Errorf("NORNS_SERVER_PORT must be between 1 and 65535")
	}
	if out.present["NORNS_SERVER_TOKEN"] && (strings.TrimSpace(out.NornsServerToken) == "" || strings.ContainsAny(out.NornsServerToken, " \t\r\n")) {
		return out, fmt.Errorf("NORNS_SERVER_TOKEN must not be empty or contain whitespace")
	}
	if out.NornsMaxConcurrentExplorations == 0 && out.present["NORNS_MAX_CONCURRENT_EXPLORATIONS"] {
		return out, fmt.Errorf("NORNS_MAX_CONCURRENT_EXPLORATIONS must be a positive integer")
	}
	if out.NornsMaxConcurrentExplorations == 0 {
		out.NornsMaxConcurrentExplorations = 3
	}
	if out.NornsMaxConcurrentExplorations < 1 {
		return out, fmt.Errorf("NORNS_MAX_CONCURRENT_EXPLORATIONS must be a positive integer")
	}
	if out.NornsExplorationTimeoutSeconds == 0 && out.present["NORNS_EXPLORATION_TIMEOUT"] {
		return out, fmt.Errorf("NORNS_EXPLORATION_TIMEOUT must be a positive integer")
	}
	if out.NornsExplorationTimeoutSeconds == 0 {
		out.NornsExplorationTimeoutSeconds = 1200
	}
	if out.NornsExplorationTimeoutSeconds < 1 {
		return out, fmt.Errorf("NORNS_EXPLORATION_TIMEOUT must be a positive integer")
	}
	if len(out.AgentExec) == 0 {
		out.AgentExec = []string{"claude"}
	}
	for _, arg := range out.AgentExec {
		if arg == "" {
			return out, fmt.Errorf("AGENT_EXEC cannot contain empty arguments")
		}
	}
	for _, path := range out.CustomPluginDirs {
		if strings.TrimSpace(path) == "" {
			return out, fmt.Errorf("CUSTOM_PLUGIN_DIRS cannot contain an empty path")
		}
	}
	if out.NornsCustomDomainDir == "" {
		out.Domain = nil
	} else {
		domain, err := LoadDomain(out.NornsCustomDomainDir)
		if err != nil {
			return out, err
		}
		out.Domain = &domain
	}
	return out, nil
}

func Settings(agent string, c Config) (map[string]any, error) {
	if agent != "cc" {
		return nil, fmt.Errorf("settings generation is not implemented for agent %q", agent)
	}
	template, err := agentSettingsTemplates.ReadFile("templates/cc/settings.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded %s settings template: %w", agent, err)
	}
	var settings map[string]any
	if err = json.Unmarshal(template, &settings); err != nil {
		return nil, fmt.Errorf("invalid embedded Claude Code settings template: %w", err)
	}
	env, ok := settings["env"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid embedded Claude Code settings template: env must be an object")
	}
	fastModel := c.LLMFastModel
	if fastModel == "" {
		fastModel = c.LLMModel
	}
	for key, value := range map[string]string{
		"ANTHROPIC_BASE_URL":             c.LLMURL,
		"ANTHROPIC_AUTH_TOKEN":           c.LLMKey,
		"ANTHROPIC_MODEL":                c.LLMModel,
		"ANTHROPIC_SMALL_FAST_MODEL":     fastModel,
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   c.LLMModel,
		"ANTHROPIC_DEFAULT_SONNET_MODEL": c.LLMModel,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  c.LLMModel,
	} {
		env[key] = value
	}
	return settings, nil
}

func marshalPersisted(c *Config) ([]byte, error) {
	value := persistedConfig{
		Version:                        c.Version,
		LLMURL:                         c.LLMURL,
		LLMKey:                         c.LLMKey,
		LLMModel:                       c.LLMModel,
		LLMFastModel:                   c.LLMFastModel,
		NornsMaxConcurrentExplorations: c.NornsMaxConcurrentExplorations,
		NornsExplorationTimeoutSeconds: c.NornsExplorationTimeoutSeconds,
		NornsAllowHumanIntervention:    c.NornsAllowHumanIntervention,
		NornsCustomDomainDir:           c.NornsCustomDomainDir,
		NornsCustomSkillsDir:           c.NornsCustomSkillsDir,
		NornsCustomAgentsDir:           c.NornsCustomAgentsDir,
		CustomAgentsDir:                c.CustomAgentsDir,
		CustomSkillsDir:                c.CustomSkillsDir,
		CustomPluginDirs:               c.CustomPluginDirs,
		AgentExec:                      c.AgentExec,
	}
	if c.Has("NORNS_ENABLE") {
		value.NornsEnable = &c.NornsEnable
	}
	return yaml.Marshal(value)
}

func Publish(agent, subjectPath, runtimeID string, c *Config) (settingsPath string, hash string, err error) {
	if c == nil {
		return "", "", nil
	}
	runtimeRoot := filepath.Join(subjectPath, ".nornsd", "runtimes", runtimeID, "artifacts", "config")
	if err = os.MkdirAll(filepath.Join(runtimeRoot, "agent"), 0700); err != nil {
		return "", "", err
	}
	settingsModel, err := Settings(agent, *c)
	if err != nil {
		return "", "", err
	}
	settings, err := json.MarshalIndent(settingsModel, "", "  ")
	if err != nil {
		return "", "", err
	}
	settings = append(settings, '\n')
	if err = os.WriteFile(filepath.Join(runtimeRoot, "agent", "settings.json"), settings, 0600); err != nil {
		return "", "", err
	}
	yamlBytes, err := marshalPersisted(c)
	if err != nil {
		return "", "", err
	}
	if err = os.WriteFile(filepath.Join(runtimeRoot, "agent.yaml"), yamlBytes, 0600); err != nil {
		return "", "", err
	}
	current := filepath.Join(subjectPath, ".nornsd", "config")
	stage := current + ".stage"
	_ = os.RemoveAll(stage)
	if err = os.MkdirAll(filepath.Join(stage, "agent"), 0700); err != nil {
		return "", "", err
	}
	if err = os.WriteFile(filepath.Join(stage, "agent", "settings.json"), settings, 0600); err != nil {
		return "", "", err
	}
	if err = os.WriteFile(filepath.Join(stage, "agent.yaml"), yamlBytes, 0600); err != nil {
		return "", "", err
	}
	old := current + ".old"
	_ = os.RemoveAll(old)
	_ = os.Rename(current, old)
	if err = os.Rename(stage, current); err != nil {
		_ = os.Rename(old, current)
		return "", "", err
	}
	_ = os.RemoveAll(old)
	sum := sha256.Sum256(settings)
	return filepath.Join(current, "agent", "settings.json"), "sha256:" + hex.EncodeToString(sum[:]), nil
}
