package config

import "fmt"

func (c Config) hasNornsResources() bool {
	return c.NornsExtendsAgentsDir != "" || c.NornsCustomAgentsDir != "" || c.NornsCustomSkillsDir != ""
}
func (c Config) hasCustomResources() bool {
	return c.CustomAgentsDir != "" || c.CustomSkillsDir != "" || len(c.CustomPluginDirs) > 0
}

// ValidateResources checks the effective plugin resource combinations.
// Empty resource values are allowed in either mode, including when clearing
// inherited resources while switching a Runtime to the other mode.
func ValidateResources(c Config) error {
	if err := validateReuseResources(c); err != nil {
		return err
	}
	if c.ReuseRuntimeDir != "" {
		return nil
	}
	if c.NornsEnable {
		for _, field := range []struct {
			name       string
			configured bool
		}{
			{"AGENT_CUSTOM_AGENTS_DIR", c.CustomAgentsDir != ""},
			{"AGENT_CUSTOM_SKILLS_DIR", c.CustomSkillsDir != ""},
			{"AGENT_CUSTOM_PLUGIN_DIRS", len(c.CustomPluginDirs) > 0},
		} {
			if field.configured {
				return fmt.Errorf("%s requires AGENT_NORNS_ENABLE=false", field.name)
			}
		}
	} else {
		for _, field := range []struct{ name, value string }{
			{"AGENT_NORNS_EXTENDS_AGENTS_DIR", c.NornsExtendsAgentsDir},
			{"AGENT_NORNS_CUSTOM_AGENTS_DIR", c.NornsCustomAgentsDir},
			{"AGENT_NORNS_CUSTOM_SKILLS_DIR", c.NornsCustomSkillsDir},
		} {
			if field.value != "" {
				return fmt.Errorf("%s requires AGENT_NORNS_ENABLE=true", field.name)
			}
		}
	}
	return nil
}

func validateReuseResources(c Config) error {
	if c.ReuseRuntimeDir == "" {
		return nil
	}
	for _, field := range []struct {
		key string
		set bool
	}{
		{"AGENT_MAINAGENT_FILE", c.MainAgentFile != ""},
		{"AGENT_NORNS_EXTENDS_AGENTS_DIR", c.NornsExtendsAgentsDir != ""},
		{"AGENT_NORNS_CUSTOM_AGENTS_DIR", c.NornsCustomAgentsDir != ""},
		{"AGENT_NORNS_CUSTOM_SKILLS_DIR", c.NornsCustomSkillsDir != ""},
		{"AGENT_CUSTOM_AGENTS_DIR", c.CustomAgentsDir != ""},
		{"AGENT_CUSTOM_SKILLS_DIR", c.CustomSkillsDir != ""},
		{"AGENT_CUSTOM_PLUGIN_DIRS", len(c.CustomPluginDirs) > 0},
	} {
		if field.set {
			return fmt.Errorf("AGENT_REUSE_RUNTIME_DIR cannot be combined with %s", field.key)
		}
	}
	return nil
}
