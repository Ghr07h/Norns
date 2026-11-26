package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Ghr07h/norns/nornsd/configs"
	"github.com/Ghr07h/norns/nornsd/internal/root"
)

type Config struct {
	AgentType                      string   `yaml:"AGENT_TYPE" json:"AGENT_TYPE"`
	ServiceRoot                    string   `yaml:"SERVICE_NORNSD_ROOT_DIR" json:"SERVICE_NORNSD_ROOT_DIR"`
	ServiceConcurrency             int      `yaml:"SERVICE_CONCURRENCY" json:"SERVICE_CONCURRENCY"`
	LLMURL                         string   `yaml:"AGENT_LLM_URL" json:"AGENT_LLM_URL"`
	LLMKey                         string   `yaml:"AGENT_LLM_KEY" json:"AGENT_LLM_KEY"`
	LLMModel                       string   `yaml:"AGENT_LLM_MODEL" json:"AGENT_LLM_MODEL"`
	LLMFastModel                   string   `yaml:"AGENT_LLM_FAST_MODEL" json:"AGENT_LLM_FAST_MODEL"`
	NornsEnable                    bool     `yaml:"AGENT_NORNS_ENABLE" json:"AGENT_NORNS_ENABLE"`
	NornsServerHost                string   `yaml:"SERVICE_HOST" json:"SERVICE_HOST,omitempty"`
	NornsServerPort                int      `yaml:"SERVICE_PORT" json:"SERVICE_PORT,omitempty"`
	NornsServerToken               string   `yaml:"SERVICE_TOKEN" json:"SERVICE_TOKEN"`
	NornsMaxConcurrentExplorations int      `yaml:"AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS" json:"AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS"`
	NornsExplorationTimeoutSeconds int      `yaml:"AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS" json:"AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS"`
	NornsAllowHumanIntervention    bool     `yaml:"AGENT_NORNS_ALLOW_HUMAN_INTERVENTION" json:"AGENT_NORNS_ALLOW_HUMAN_INTERVENTION"`
	NornsExtendsAgentsDir          string   `yaml:"AGENT_NORNS_EXTENDS_AGENTS_DIR" json:"AGENT_NORNS_EXTENDS_AGENTS_DIR,omitempty"`
	NornsCustomSkillsDir           string   `yaml:"AGENT_NORNS_CUSTOM_SKILLS_DIR" json:"AGENT_NORNS_CUSTOM_SKILLS_DIR,omitempty"`
	MainAgentFile                  string   `yaml:"AGENT_MAINAGENT_FILE" json:"AGENT_MAINAGENT_FILE,omitempty"`
	NornsCustomAgentsDir           string   `yaml:"AGENT_NORNS_CUSTOM_AGENTS_DIR" json:"AGENT_NORNS_CUSTOM_AGENTS_DIR,omitempty"`
	CustomAgentsDir                string   `yaml:"AGENT_CUSTOM_AGENTS_DIR" json:"AGENT_CUSTOM_AGENTS_DIR,omitempty"`
	CustomSkillsDir                string   `yaml:"AGENT_CUSTOM_SKILLS_DIR" json:"AGENT_CUSTOM_SKILLS_DIR,omitempty"`
	CustomPluginDirs               []string `yaml:"AGENT_CUSTOM_PLUGIN_DIRS" json:"AGENT_CUSTOM_PLUGIN_DIRS,omitempty"`
	AgentExec                      []string `yaml:"AGENT_EXEC" json:"AGENT_EXEC"`
	ReuseRuntimeDir                string   `yaml:"AGENT_REUSE_RUNTIME_DIR" json:"AGENT_REUSE_RUNTIME_DIR,omitempty"`
	Source                         string   `yaml:"-" json:"-"`
	Domain                         *Domain  `yaml:"-" json:"-"`
	present                        map[string]bool
	agentExecExplicit              bool
}

func Load(path string) (Config, error) {
	out, err := load(path)
	if err != nil {
		return out, err
	}
	return normalize(out)
}

// LoadPartial parses one merge layer. Required fields are checked only after
// all layers have been merged. Nonempty configuration paths must be absolute.
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
		if err := validateYAMLValues(mapping); err != nil {
			return out, err
		}
		for i := 0; i < len(mapping.Content); i += 2 {
			key := mapping.Content[i].Value
			out.present[key] = true
			if key == "AGENT_CUSTOM_PLUGIN_DIRS" && mapping.Content[i+1].Kind != yaml.SequenceNode {
				return out, fmt.Errorf("invalid setting: AGENT_CUSTOM_PLUGIN_DIRS must be a YAML list")
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
	if err := validatePaths(out); err != nil {
		return out, err
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
	if has("AGENT_TYPE", override.AgentType != "") {
		base.AgentType = override.AgentType
	}
	if has("AGENT_LLM_URL", override.LLMURL != "") {
		base.LLMURL = override.LLMURL
	}
	if has("AGENT_LLM_KEY", override.LLMKey != "") {
		base.LLMKey = override.LLMKey
	}
	if has("AGENT_LLM_MODEL", override.LLMModel != "") {
		base.LLMModel = override.LLMModel
	}
	if has("AGENT_LLM_FAST_MODEL", override.LLMFastModel != "") {
		base.LLMFastModel = override.LLMFastModel
	}
	if has("AGENT_NORNS_ENABLE", override.NornsEnable) {
		base.NornsEnable = override.NornsEnable
	}
	if has("SERVICE_HOST", override.NornsServerHost != "") {
		base.NornsServerHost = override.NornsServerHost
	}
	if has("SERVICE_PORT", override.NornsServerPort != 0) {
		base.NornsServerPort = override.NornsServerPort
	}
	if has("SERVICE_TOKEN", override.NornsServerToken != "") {
		base.NornsServerToken = override.NornsServerToken
	}
	if has("AGENT_NORNS_EXTENDS_AGENTS_DIR", override.NornsExtendsAgentsDir != "") {
		base.NornsExtendsAgentsDir = override.NornsExtendsAgentsDir
		base.Domain = nil
	}
	if has("AGENT_NORNS_CUSTOM_SKILLS_DIR", override.NornsCustomSkillsDir != "") {
		base.NornsCustomSkillsDir = override.NornsCustomSkillsDir
	}
	if has("AGENT_NORNS_CUSTOM_AGENTS_DIR", override.NornsCustomAgentsDir != "") {
		base.NornsCustomAgentsDir = override.NornsCustomAgentsDir
	}
	if has("AGENT_MAINAGENT_FILE", override.MainAgentFile != "") {
		base.MainAgentFile = override.MainAgentFile
	}
	if has("AGENT_REUSE_RUNTIME_DIR", override.ReuseRuntimeDir != "") {
		base.ReuseRuntimeDir = override.ReuseRuntimeDir
	}
	if has("AGENT_CUSTOM_AGENTS_DIR", override.CustomAgentsDir != "") {
		base.CustomAgentsDir = override.CustomAgentsDir
	}
	if has("AGENT_CUSTOM_SKILLS_DIR", override.CustomSkillsDir != "") {
		base.CustomSkillsDir = override.CustomSkillsDir
	}
	if has("AGENT_CUSTOM_PLUGIN_DIRS", len(override.CustomPluginDirs) > 0) {
		base.CustomPluginDirs = append([]string(nil), override.CustomPluginDirs...)
	}
	if has("AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS", override.NornsMaxConcurrentExplorations != 0) {
		base.NornsMaxConcurrentExplorations = override.NornsMaxConcurrentExplorations
	}
	if has("AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS", override.NornsExplorationTimeoutSeconds != 0) {
		base.NornsExplorationTimeoutSeconds = override.NornsExplorationTimeoutSeconds
	}
	if has("AGENT_NORNS_ALLOW_HUMAN_INTERVENTION", override.NornsAllowHumanIntervention) {
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
	if out.AgentType == "" && !out.Has("AGENT_TYPE") {
		out.AgentType = "cc"
	}
	if out.AgentType != "cc" {
		return out, fmt.Errorf("agent %q is not implemented", out.AgentType)
	}
	if out.NornsServerHost == "" && !out.Has("SERVICE_HOST") {
		out.NornsServerHost = "127.0.0.1"
	}
	if out.NornsServerPort == 0 && !out.Has("SERVICE_PORT") {
		out.NornsServerPort = 28081
	}
	if out.ServiceConcurrency == 0 && !out.Has("SERVICE_CONCURRENCY") {
		out.ServiceConcurrency = 1
	}
	if out.ServiceConcurrency < 1 {
		return out, fmt.Errorf("SERVICE_CONCURRENCY must be positive")
	}
	if err := validatePaths(out); err != nil {
		return out, err
	}
	if err := ValidateResources(out); err != nil {
		return out, err
	}
	if strings.TrimSpace(out.LLMURL) == "" || strings.TrimSpace(out.LLMKey) == "" || strings.TrimSpace(out.LLMModel) == "" {
		return out, fmt.Errorf("AGENT_LLM_URL, AGENT_LLM_KEY and AGENT_LLM_MODEL are required")
	}
	if out.LLMFastModel == "" {
		out.LLMFastModel = out.LLMModel
	}
	if out.present["SERVICE_HOST"] && strings.TrimSpace(out.NornsServerHost) == "" {
		return out, fmt.Errorf("SERVICE_HOST must not be empty")
	}
	if out.present["SERVICE_PORT"] && (out.NornsServerPort < 1 || out.NornsServerPort > 65535) {
		return out, fmt.Errorf("SERVICE_PORT must be between 1 and 65535")
	}
	if out.present["SERVICE_TOKEN"] && (strings.TrimSpace(out.NornsServerToken) == "" || strings.ContainsAny(out.NornsServerToken, " \t\r\n")) {
		return out, fmt.Errorf("SERVICE_TOKEN must not be empty or contain whitespace")
	}
	if out.ReuseRuntimeDir == "" {
		if out.NornsMaxConcurrentExplorations == 0 && out.present["AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS"] {
			return out, fmt.Errorf("AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS must be a positive integer")
		}
		if out.NornsMaxConcurrentExplorations == 0 {
			out.NornsMaxConcurrentExplorations = 3
		}
		if out.NornsMaxConcurrentExplorations < 1 {
			return out, fmt.Errorf("AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS must be a positive integer")
		}
		if out.NornsExplorationTimeoutSeconds == 0 && out.present["AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS"] {
			return out, fmt.Errorf("AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS must be a positive integer")
		}
		if out.NornsExplorationTimeoutSeconds == 0 {
			out.NornsExplorationTimeoutSeconds = 1200
		}
		if out.NornsExplorationTimeoutSeconds < 1 {
			return out, fmt.Errorf("AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS must be a positive integer")
		}
	}
	if len(out.AgentExec) == 0 {
		out.agentExecExplicit = false
		command, err := exec.LookPath("claude")
		if err != nil {
			return out, fmt.Errorf("AGENT_EXEC is required when claude is not on PATH: %w", err)
		}
		command, err = filepath.Abs(command)
		if err != nil {
			return out, err
		}
		out.AgentExec = []string{command}
	}
	for _, arg := range out.AgentExec {
		if arg == "" {
			return out, fmt.Errorf("AGENT_EXEC cannot contain empty arguments")
		}
	}
	for _, path := range out.CustomPluginDirs {
		if strings.TrimSpace(path) == "" {
			return out, fmt.Errorf("AGENT_CUSTOM_PLUGIN_DIRS cannot contain an empty path")
		}
	}
	if err := validatePaths(out); err != nil {
		return out, err
	}
	if out.NornsExtendsAgentsDir == "" {
		out.Domain = nil
	} else {
		domain, err := LoadDomain(out.NornsExtendsAgentsDir)
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
	template, err := configs.AgentSettingsTemplates.ReadFile("cc/settings.json")
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

func marshalPersisted(c *Config) ([]byte, error) { return yaml.Marshal(c) }

func Publish(agent, subjectPath, runtimeID string, c *Config) (settingsPath string, hash string, err error) {
	if c == nil {
		return "", "", nil
	}
	runtimeRoot := filepath.Join(root.RuntimeDataPath(subjectPath, runtimeID), "configs")
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
	if err = os.WriteFile(filepath.Join(runtimeRoot, "nornsd.yaml"), yamlBytes, 0600); err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(yamlBytes)
	return filepath.Join(runtimeRoot, "agent", "settings.json"), "sha256:" + hex.EncodeToString(sum[:]), nil
}
