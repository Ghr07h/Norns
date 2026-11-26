package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// Validate only fields present in an override. Dependencies on the selected
// template and external resources are checked again when a slot claims it.
func validateOverride(c Config) error {
	if err := validatePaths(c); err != nil {
		return err
	}
	for key, value := range map[string]string{"AGENT_LLM_URL": c.LLMURL, "AGENT_LLM_KEY": c.LLMKey, "AGENT_LLM_MODEL": c.LLMModel} {
		if c.Has(key) && strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s cannot be empty", key)
		}
	}
	if c.Has("AGENT_TYPE") && c.AgentType != "cc" {
		return fmt.Errorf("AGENT_TYPE must be cc")
	}
	if err := validateReuseResources(c); err != nil {
		return err
	}
	for key, value := range map[string]int{"AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS": c.NornsMaxConcurrentExplorations, "AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS": c.NornsExplorationTimeoutSeconds} {
		if c.ReuseRuntimeDir == "" && c.Has(key) && value < 1 {
			return fmt.Errorf("%s must be positive", key)
		}
	}
	if c.Has("AGENT_EXEC") {
		for _, arg := range c.AgentExec {
			if arg == "" {
				return fmt.Errorf("AGENT_EXEC cannot contain empty arguments")
			}
		}
	}
	if c.Has("AGENT_NORNS_ENABLE") || c.hasNornsResources() && c.hasCustomResources() {
		return ValidateResources(c)
	}
	return nil
}

func validateYAMLValues(mapping *yaml.Node) error {
	fields := map[string]reflect.Kind{}
	typ := reflect.TypeOf(Config{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		fields[f.Tag.Get("yaml")] = f.Type.Kind()
	}
	for i := 0; i < len(mapping.Content); i += 2 {
		key, value := mapping.Content[i].Value, mapping.Content[i+1]
		if value.Tag == "!!null" {
			return fmt.Errorf("%s cannot be null", key)
		}
		kind, ok := fields[key]
		if !ok {
			continue
		} // KnownFields reports unknown keys.
		expected := map[reflect.Kind]string{reflect.String: "!!str", reflect.Int: "!!int", reflect.Bool: "!!bool", reflect.Slice: "!!seq"}[kind]
		if value.Tag != expected {
			return fmt.Errorf("%s has an invalid YAML type", key)
		}
		if kind == reflect.Slice {
			for _, item := range value.Content {
				if item.Tag != "!!str" {
					return fmt.Errorf("%s must contain strings", key)
				}
			}
		}
	}
	return nil
}

func writeExternalManifest(dir, runtimeID string) error {
	manifest := CustomPluginManifest{GeneratorVersion: 1, RuntimeID: runtimeID, Files: map[string]string{}}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "plugin.manifest.json" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		manifest.Files[filepath.ToSlash(rel)] = "sha256:" + hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "plugin.manifest.json"), append(b, '\n'), 0600)
}
