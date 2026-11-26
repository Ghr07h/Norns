package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	"github.com/Ghr07h/norns/nornsd/internal/version"
	"gopkg.in/yaml.v3"
)

func validatePaths(c Config) error {
	for key, value := range map[string]string{"SERVICE_NORNSD_ROOT_DIR": c.ServiceRoot, "AGENT_MAINAGENT_FILE": c.MainAgentFile, "AGENT_REUSE_RUNTIME_DIR": c.ReuseRuntimeDir, "AGENT_NORNS_EXTENDS_AGENTS_DIR": c.NornsExtendsAgentsDir, "AGENT_NORNS_CUSTOM_AGENTS_DIR": c.NornsCustomAgentsDir, "AGENT_NORNS_CUSTOM_SKILLS_DIR": c.NornsCustomSkillsDir, "AGENT_CUSTOM_AGENTS_DIR": c.CustomAgentsDir, "AGENT_CUSTOM_SKILLS_DIR": c.CustomSkillsDir} {
		if value != "" && !filepath.IsAbs(value) {
			return fmt.Errorf("%s must be an absolute path", key)
		}
	}
	for _, value := range c.CustomPluginDirs {
		if value == "" || !filepath.IsAbs(value) {
			return fmt.Errorf("AGENT_CUSTOM_PLUGIN_DIRS must contain absolute paths")
		}
	}
	if len(c.AgentExec) > 0 && !filepath.IsAbs(c.AgentExec[0]) {
		return fmt.Errorf("AGENT_EXEC executable must be an absolute path")
	}
	return nil
}
func (c Config) Profile() domain.RuntimeProfile {
	caps := []string{}
	if c.NornsEnable {
		caps = append(caps, "norns")
	}
	return domain.RuntimeProfile{Agent: c.AgentType, AgentVersion: version.ClaudeCodeVersion, Capabilities: caps}
}

// ParseOverride distinguishes field presence from zero values. Config null and
// {} mean no override; a null field is always rejected.
func ParseOverride(raw json.RawMessage) (Config, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return Config{present: map[string]bool{}}, nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return Config{}, fmt.Errorf("config must be an object: %w", err)
	}
	if values == nil {
		return Config{}, fmt.Errorf("config must be an object")
	}
	allowed := map[string]bool{}
	typ := reflect.TypeOf(Config{})
	for i := 0; i < typ.NumField(); i++ {
		tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if strings.HasPrefix(tag, "AGENT_") {
			allowed[tag] = true
		}
	}
	for key, value := range values {
		if !allowed[key] {
			return Config{}, fmt.Errorf("config cannot override %s", key)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return Config{}, fmt.Errorf("config.%s cannot be null", key)
		}
	}
	var c Config
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return c, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return c, fmt.Errorf("invalid config JSON")
	}
	c.present = map[string]bool{}
	for key := range values {
		c.present[key] = true
	}
	c.agentExecExplicit = c.Has("AGENT_EXEC") && len(c.AgentExec) > 0
	return c, validateOverride(c)
}
func (c Config) HasOverrides() bool { return len(c.present) > 0 }
func Effective(template Config, raw json.RawMessage) (Config, error) {
	override, err := ParseOverride(raw)
	if err != nil {
		return Config{}, err
	}
	// Merge mutates presence metadata, so give each Runtime an independent map.
	present := map[string]bool{}
	for key, value := range template.present {
		present[key] = value
	}
	template.present = present
	c := Merge(template, override)
	return Normalize(c)
}
func (c *Config) UseService(first Config) {
	c.ServiceRoot = first.ServiceRoot
	c.ServiceConcurrency = first.ServiceConcurrency
	c.NornsServerHost = first.NornsServerHost
	c.NornsServerPort = first.NornsServerPort
	c.NornsServerToken = first.NornsServerToken
	if c.present == nil {
		c.present = map[string]bool{}
	}
	for _, key := range []string{"SERVICE_HOST", "SERVICE_PORT", "SERVICE_TOKEN", "SERVICE_CONCURRENCY", "SERVICE_NORNSD_ROOT_DIR"} {
		c.present[key] = first.Has(key)
	}
}
func SaveTemplates(control string, templates []Config) error {
	stage := filepath.Join(control, "configs.stage")
	_ = os.RemoveAll(stage)
	if err := os.MkdirAll(stage, 0700); err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for i, c := range templates {
		name := "nornsd.yaml"
		if i > 0 {
			name = fmt.Sprintf("nornsd%d.yaml", i+1)
		}
		b, err := yaml.Marshal(c)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(stage, name), b, 0600); err != nil {
			return err
		}
	}
	current := filepath.Join(control, "configs")
	old := current + ".old"
	_ = os.RemoveAll(old)
	if err := os.Rename(current, old); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stage, current); err != nil {
		_ = os.Rename(old, current)
		return err
	}
	return os.RemoveAll(old)
}
func PublishExternal(subjectPath, runtimeID string, c *Config) ([]string, error) {
	paths := []string{}
	if c == nil {
		return paths, nil
	}
	if err := ValidateResources(*c); err != nil {
		return nil, err
	}
	for i, source := range c.CustomPluginDirs {
		target := filepath.Join(root.RuntimeDataPath(subjectPath, runtimeID), "plugins", fmt.Sprintf("external-plugin-%d", i+1))
		if err := root.CopyTreeNoLinks(source, target); err != nil {
			return nil, err
		}
		if err := writeExternalManifest(target, runtimeID); err != nil {
			return nil, err
		}
		paths = append(paths, target)
	}
	return paths, nil
}
