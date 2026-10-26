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
	var protocol, plugins, output string
	flag.StringVar(&protocol, "protocol", "", "protocol directory")
	flag.StringVar(&plugins, "plugins", "", "agent plugin source root")
	flag.StringVar(&output, "output", "", "generated output directory")
	flag.Parse()
	if protocol == "" || plugins == "" || output == "" {
		fatalf("protocol, plugins and output are required")
	}
	manifestBytes, err := os.ReadFile(filepath.Join(protocol, "manifest.json"))
	if err != nil {
		fatalf("read manifest: %v", err)
	}
	var protocolManifest struct {
		ProtocolVersion int `json:"protocol_version"`
	}
	if err = json.Unmarshal(manifestBytes, &protocolManifest); err != nil {
		fatalf("parse manifest: %v", err)
	}
	stage := output + ".stage"
	_ = os.RemoveAll(stage)
	if err = os.MkdirAll(stage, 0755); err != nil {
		fatalf("create stage: %v", err)
	}
	defer os.RemoveAll(stage)
	pluginsTarget := filepath.Join(stage, "plugins")
	if err = copyTree(plugins, pluginsTarget); err != nil {
		fatalf("copy plugins: %v", err)
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
	m := manifest{ProtocolVersion: protocolManifest.ProtocolVersion, Generator: 1, Files: map[string]string{}}
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
