package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/Ghr07h/norns/nornsd/internal/root"
)

type CustomPluginManifest struct {
	GeneratorVersion int               `json:"generator_version"`
	RuntimeID        string            `json:"runtime_id"`
	Files            map[string]string `json:"files"`
}

var customPluginMetadata = []byte(`{
  "name": "nornsd-custom",
  "version": "1.0.0",
  "description": "Runtime-specific custom agents and skills managed by nornsd"
}
`)

func PublishCustom(subjectPath, runtimeID string, c *Config) (pluginPath string, hash string, err error) {
	if c == nil || (c.CustomAgentsDir == "" && c.CustomSkillsDir == "") {
		return "", "", nil
	}
	stage := filepath.Join(subjectPath, ".nornsd", "staging", "custom-plugin-"+runtimeID)
	_ = os.RemoveAll(stage)
	defer os.RemoveAll(stage)
	if err = os.MkdirAll(filepath.Join(stage, ".claude-plugin"), 0700); err != nil {
		return "", "", err
	}
	if err = os.WriteFile(filepath.Join(stage, ".claude-plugin", "plugin.json"), customPluginMetadata, 0600); err != nil {
		return "", "", err
	}
	for _, source := range []struct{ path, target string }{
		{c.CustomAgentsDir, "agents"},
		{c.CustomSkillsDir, "skills"},
	} {
		if source.path == "" {
			continue
		}
		if err = root.CopyTreeNoLinks(source.path, filepath.Join(stage, source.target)); err != nil {
			return "", "", err
		}
	}
	manifest := CustomPluginManifest{GeneratorVersion: 1, RuntimeID: runtimeID, Files: map[string]string{}}
	var paths []string
	if err = filepath.WalkDir(stage, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			rel, relErr := filepath.Rel(stage, path)
			if relErr != nil {
				return relErr
			}
			paths = append(paths, rel)
		}
		return nil
	}); err != nil {
		return "", "", err
	}
	sort.Strings(paths)
	for _, rel := range paths {
		fileBytes, readErr := os.ReadFile(filepath.Join(stage, rel))
		if readErr != nil {
			return "", "", readErr
		}
		sum := sha256.Sum256(fileBytes)
		manifest.Files[filepath.ToSlash(rel)] = "sha256:" + hex.EncodeToString(sum[:])
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", "", err
	}
	manifestBytes = append(manifestBytes, '\n')
	if err = os.WriteFile(filepath.Join(stage, "plugin.manifest.json"), manifestBytes, 0600); err != nil {
		return "", "", err
	}
	artifact := filepath.Join(subjectPath, ".nornsd", "runtimes", runtimeID, "artifacts", "custom-plugin")
	if err = os.MkdirAll(filepath.Dir(artifact), 0700); err != nil {
		return "", "", err
	}
	_ = os.RemoveAll(artifact)
	if err = os.Rename(stage, artifact); err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(manifestBytes)
	return artifact, "sha256:" + hex.EncodeToString(sum[:]), nil
}
