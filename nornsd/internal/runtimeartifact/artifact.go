package runtimeartifact

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	ccagent "github.com/Ghr07h/norns/nornsd/internal/agent/cc"
	"github.com/Ghr07h/norns/nornsd/internal/config"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/version"
	"github.com/google/uuid"
)

type Input struct {
	Subject          domain.Subject
	Runtime          domain.Runtime
	Launch           ccagent.Launch
	Managed          bool
	Config           *config.Config
	ConfigHash       string
	PluginHash       string
	CustomPluginHash string
	Slot             *int
	CreatedAt        time.Time
}

func Publish(subjectPath string, input Input) (map[string]any, error) {
	argv, err := ccagent.BuildArgs(input.Launch, input.Managed)
	if err != nil {
		return nil, err
	}
	createdAt := input.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	launch := map[string]any{
		"version":          1,
		"agent":            input.Subject.Profile.Agent,
		"session_id":       input.Launch.SessionID,
		"runtime_profile":  input.Subject.Profile,
		"argv":             redactArgv(argv, len(input.Launch.Exec)),
		"managed":          input.Managed,
		"settings_enabled": input.Launch.SettingsPath != "",
		"plugin_enabled":   len(input.Launch.PluginPaths) > 0,
		"plugin_count":     len(input.Launch.PluginPaths),
		"created_at":       createdAt,
	}
	if input.Launch.ResumeSessionID != "" {
		launch["resume_session_id"] = input.Launch.ResumeSessionID
	}
	if input.Config != nil && input.Config.Source != "" {
		launch["config_source"] = input.Config.Source
	}
	snapshot := map[string]any{
		"version":            1,
		"runtime_id":         input.Runtime.ID,
		"subject_id":         input.Subject.ID,
		"runtime_profile":    input.Subject.Profile,
		"session_mode":       input.Runtime.SessionMode,
		"session_id":         input.Launch.SessionID,
		"resume_session_id":  nullableString(input.Launch.ResumeSessionID),
		"settings_slot":      input.Slot,
		"config_hash":        nullableString(input.ConfigHash),
		"plugin_hash":        nullableString(input.PluginHash),
		"custom_plugin_hash": nullableString(input.CustomPluginHash),
		"protocol_version": func() any {
			if input.Subject.Profile.Has("norns") {
				return version.NornsProtocol
			}
			return nil
		}(),
		"agent_config_version": version.AgentConfigVersion,
		"created_at":           createdAt,
	}
	configSnapshot := ConfigSnapshot(input.Subject.Profile.Agent, input.Subject.Profile.AgentVersion, input.Config, input.ConfigHash, input.PluginHash, input.CustomPluginHash, input.Slot, createdAt)
	runtimeRoot := filepath.Join(subjectPath, ".nornsd", "runtimes", input.Runtime.ID)
	if err = os.MkdirAll(runtimeRoot, 0700); err != nil {
		return nil, err
	}
	for name, value := range map[string]any{"snapshot.json": snapshot, "launch.json": launch, "config.snapshot.json": configSnapshot} {
		if err = writeJSONAtomic(filepath.Join(runtimeRoot, name), value); err != nil {
			return nil, err
		}
	}
	if err = writeJSONAtomic(filepath.Join(subjectPath, ".nornsd", "config", "launch.json"), launch); err != nil {
		return nil, err
	}
	for _, item := range []struct{ source, target string }{
		{filepath.Join(runtimeRoot, "artifacts", "custom-plugin", "plugin.manifest.json"), filepath.Join(runtimeRoot, "custom-plugin.manifest.json")},
		{filepath.Join(runtimeRoot, "artifacts", "plugin", "plugin.manifest.json"), filepath.Join(runtimeRoot, "plugin.manifest.json")},
	} {
		if err = copyOptional(item.source, item.target); err != nil {
			return nil, err
		}
	}
	return configSnapshot, nil
}

func ConfigSnapshot(agent, agentVersion string, c *config.Config, configHash, pluginHash, customPluginHash string, slot *int, createdAt time.Time) map[string]any {
	snapshot := map[string]any{
		"version":                  1,
		"settings_slot":            slot,
		"config_hash":              nullableString(configHash),
		"plugin_hash":              nullableString(pluginHash),
		"custom_plugin_hash":       nullableString(customPluginHash),
		"agent":                    agent,
		"agent_version":            agentVersion,
		"agent_config_version":     version.AgentConfigVersion,
		"plugin_generator_version": 1,
		"created_at":               createdAt.UTC(),
	}
	if c == nil {
		snapshot["setting_enabled"] = false
		return snapshot
	}
	snapshot["setting_enabled"] = true
	snapshot["config_source"] = c.Source
	snapshot["models"] = map[string]any{"default": c.LLMModel, "fast": c.LLMFastModel}
	snapshot["llm_url"] = redactURL(c.LLMURL)
	snapshot["agent_exec"] = redactExec(c.AgentExec)
	snapshot["custom_sources"] = map[string]any{
		"norns_domain": c.NornsCustomDomainDir != "",
		"norns_agents": c.NornsCustomAgentsDir != "",
		"norns_skills": c.NornsCustomSkillsDir != "",
		"agents":       c.CustomAgentsDir != "",
		"skills":       c.CustomSkillsDir != "",
		"plugin_dirs":  len(c.CustomPluginDirs),
	}
	nornsSnapshot := map[string]any{
		"max_concurrent_explorations": c.NornsMaxConcurrentExplorations,
		"exploration_timeout_seconds": c.NornsExplorationTimeoutSeconds,
		"allow_human_intervention":    c.NornsAllowHumanIntervention,
	}
	if c.Domain != nil {
		nornsSnapshot["domain_name"] = c.Domain.Name
	}
	snapshot["norns"] = nornsSnapshot
	return snapshot
}

func redactArgv(argv []string, execLen int) []string {
	out := append([]string(nil), argv...)
	if execLen > len(out) {
		execLen = len(out)
	}
	for i := 1; i < execLen; i++ {
		out[i] = "<redacted-wrapper-arg>"
	}
	for i := execLen; i < len(out); i++ {
		lower := strings.ToLower(out[i])
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "api-key") || strings.Contains(lower, "api_key") {
			if strings.Contains(out[i], "=") {
				out[i] = strings.SplitN(out[i], "=", 2)[0] + "=<redacted>"
			} else if i+1 < len(out) {
				out[i+1] = "<redacted>"
			}
		}
	}
	return out
}

func redactExec(argv []string) []string {
	if len(argv) == 0 {
		return nil
	}
	out := []string{argv[0]}
	for range argv[1:] {
		out = append(out, "<redacted-wrapper-arg>")
	}
	return out
}

func redactURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "<redacted-url>"
	}
	return parsed.Scheme + "://" + parsed.Host
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temp := path + "." + uuid.NewString() + ".tmp"
	if err = os.WriteFile(temp, data, 0600); err != nil {
		return err
	}
	if err = os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("publish %s: %w", filepath.Base(path), err)
	}
	return nil
}

func copyOptional(source, target string) error {
	data, err := os.ReadFile(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0600)
}
