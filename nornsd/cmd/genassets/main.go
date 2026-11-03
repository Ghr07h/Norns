package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type manifest struct {
	ProtocolVersion int               `json:"protocol_version"`
	Generator       int               `json:"generator_version"`
	Files           map[string]string `json:"files"`
}

func main() {
	var versionPath, protocol, plugins, output string
	flag.StringVar(&versionPath, "versions", "", "root version manifest")
	flag.StringVar(&protocol, "protocol", "", "protocol directory")
	flag.StringVar(&plugins, "plugins", "", "agent plugin source root")
	flag.StringVar(&output, "output", "", "generated output directory")
	flag.Parse()
	if versionPath == "" || protocol == "" || plugins == "" || output == "" {
		fatalf("versions, protocol, plugins and output are required")
	}
	versionManifest, err := readVersions(versionPath)
	if err != nil {
		fatalf("read versions: %v", err)
	}
	stage := output + ".stage"
	_ = os.RemoveAll(stage)
	if err := os.MkdirAll(stage, 0755); err != nil {
		fatalf("create stage: %v", err)
	}
	defer os.RemoveAll(stage)
	pluginsTarget := filepath.Join(stage, "plugins")
	if err := copyTree(plugins, pluginsTarget); err != nil {
		fatalf("copy plugins: %v", err)
	}
	for name, pluginVersion := range versionManifest.Nornsd.Plugins {
		metadataPath := filepath.Join(pluginsTarget, name, ".claude-plugin", "plugin.json")
		if err := updateJSON(metadataPath, func(metadata map[string]any) {
			metadata["version"] = fmt.Sprintf("%d.0.0", pluginVersion)
		}); err != nil {
			fatalf("update %s plugin metadata: %v", name, err)
		}
		settingsPath := filepath.Join(pluginsTarget, name, "settings.json")
		if err := updateJSON(settingsPath, func(settings map[string]any) {
			env, ok := settings["env"].(map[string]any)
			if !ok {
				env = map[string]any{}
			}
			env["NORNS_PROTOCOL_VERSION"] = fmt.Sprint(versionManifest.Nornsd.SupportProtocol)
			settings["env"] = env
		}); err != nil {
			fatalf("update %s plugin settings: %v", name, err)
		}
	}
	protocolBytes, err := os.ReadFile(filepath.Join(protocol, "protocol.md"))
	if err != nil {
		fatalf("read protocol: %v", err)
	}
	entries, err := os.ReadDir(pluginsTarget)
	if err != nil {
		fatalf("read generated plugins: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skill := filepath.Join(pluginsTarget, entry.Name(), "skills", "norns-loop", "SKILL.md")
		if _, statErr := os.Stat(skill); os.IsNotExist(statErr) {
			continue
		} else if statErr != nil {
			fatalf("inspect %s plugin: %v", entry.Name(), statErr)
		}
		reference := filepath.Join(filepath.Dir(skill), "references", "protocol.md")
		if err = os.MkdirAll(filepath.Dir(reference), 0755); err != nil {
			fatalf("create %s protocol reference: %v", entry.Name(), err)
		}
		if err = os.WriteFile(reference, protocolBytes, 0644); err != nil {
			fatalf("write %s protocol reference: %v", entry.Name(), err)
		}
	}
	m := manifest{ProtocolVersion: versionManifest.Protocol, Generator: 1, Files: map[string]string{}}
	var paths []string
	if err = filepath.WalkDir(stage, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(stage, path)
		if relErr != nil {
			return relErr
		}
		if filepath.ToSlash(rel) != "manifest.generated.json" {
			paths = append(paths, rel)
		}
		return nil
	}); err != nil {
		fatalf("walk generated assets: %v", err)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		b, readErr := os.ReadFile(filepath.Join(stage, rel))
		if readErr != nil {
			fatalf("hash %s: %v", rel, readErr)
		}
		sum := sha256.Sum256(b)
		m.Files[filepath.ToSlash(rel)] = "sha256:" + hex.EncodeToString(sum[:])
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		fatalf("encode generated manifest: %v", err)
	}
	b = append(b, '\n')
	if err = os.WriteFile(filepath.Join(stage, "manifest.generated.json"), b, 0644); err != nil {
		fatalf("write generated manifest: %v", err)
	}
	_ = os.RemoveAll(output)
	if err = os.Rename(stage, output); err != nil {
		fatalf("publish: %v", err)
	}
}

type versionManifest struct {
	Protocol int `json:"protocol"`
	Nornsd   struct {
		SupportProtocol int            `json:"support_protocol"`
		Plugins         map[string]int `json:"plugins"`
	} `json:"nornsd"`
}

func readVersions(path string) (versionManifest, error) {
	var manifest versionManifest
	source, err := os.ReadFile(path)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(source, &manifest); err != nil {
		return manifest, err
	}
	if manifest.Protocol < 1 || manifest.Nornsd.SupportProtocol < 1 || manifest.Nornsd.Plugins["cc"] < 1 {
		return manifest, fmt.Errorf("protocol, nornsd.support_protocol and nornsd.plugins.cc must be positive integers")
	}
	for name, version := range manifest.Nornsd.Plugins {
		if version < 1 {
			return manifest, fmt.Errorf("nornsd.plugins.%s must be a positive integer", name)
		}
	}
	return manifest, nil
}

func updateJSON(path string, update func(map[string]any)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var value map[string]any
	if err = json.Unmarshal(data, &value); err != nil {
		return err
	}
	update(value)
	data, err = json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(target, 0755)
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if d.IsDir() && len(parts) == 2 && parts[1] == "backup" {
			return fs.SkipDir
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", path)
		}
		dst := filepath.Join(target, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0644)
	})
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "genassets: "+strings.TrimSpace(format)+"\n", args...)
	os.Exit(1)
}
