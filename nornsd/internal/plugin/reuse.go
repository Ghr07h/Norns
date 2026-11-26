package plugin

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ghr07h/norns/nornsd/internal/root"
)

func reuseRuntime(agent, subjectPath, runtimeID, source string) (RuntimePlugins, error) {
	if !filepath.IsAbs(source) {
		return RuntimePlugins{}, fmt.Errorf("AGENT_REUSE_RUNTIME_DIR must be an absolute path")
	}
	info, err := os.Lstat(source)
	if err != nil {
		return RuntimePlugins{}, err
	}
	if !info.IsDir() {
		return RuntimePlugins{}, fmt.Errorf("AGENT_REUSE_RUNTIME_DIR must be a Runtime directory, not a symlink")
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return RuntimePlugins{}, err
	}
	targetRuntime := root.RuntimeDataPath(subjectPath, runtimeID)
	if err = os.MkdirAll(targetRuntime, 0700); err != nil {
		return RuntimePlugins{}, err
	}
	targetRuntime, err = filepath.EvalSymlinks(targetRuntime)
	if err != nil {
		return RuntimePlugins{}, err
	}
	if _, err = root.EnsureWithin(source, targetRuntime); err == nil {
		return RuntimePlugins{}, fmt.Errorf("reuse source must be independent of the new Runtime")
	}
	if _, err = root.EnsureWithin(targetRuntime, source); err == nil {
		return RuntimePlugins{}, fmt.Errorf("reuse source must be independent of the new Runtime")
	}
	launchFile := filepath.Join(source, "launch.json")
	info, err = os.Lstat(launchFile)
	if err != nil {
		return RuntimePlugins{}, fmt.Errorf("reuse requires launch.json: %w", err)
	}
	if !info.Mode().IsRegular() {
		return RuntimePlugins{}, fmt.Errorf("reuse launch.json must be a regular file")
	}
	data, err := os.ReadFile(launchFile)
	if err != nil {
		return RuntimePlugins{}, err
	}
	var launch struct {
		Agent struct {
			Type string   `json:"type"`
			Argv []string `json:"argv"`
		} `json:"agent"`
		Plugins []struct {
			Path string `json:"path"`
		} `json:"plugins"`
	}
	if err = json.Unmarshal(data, &launch); err != nil {
		return RuntimePlugins{}, fmt.Errorf("invalid reused launch.json: %w", err)
	}
	if launch.Agent.Type != agent || len(launch.Agent.Argv) == 0 {
		return RuntimePlugins{}, fmt.Errorf("reused launch.json must contain a compatible Agent type and startup argv")
	}
	mainID, err := savedMainAgent(launch.Agent.Argv)
	if err != nil {
		return RuntimePlugins{}, err
	}
	stage := filepath.Join(root.SubjectDataPath(subjectPath), "staging", "reuse-plugins-"+runtimeID)
	_ = os.RemoveAll(stage)
	defer os.RemoveAll(stage)
	pluginsSource := filepath.Join(source, "plugins")
	if _, err = os.Stat(pluginsSource); os.IsNotExist(err) && len(launch.Plugins) == 0 {
		err = os.MkdirAll(stage, 0700) // A plain Runtime may have no plugin resources.
	} else if err == nil {
		err = copyReusedPlugins(pluginsSource, stage)
	}
	if err != nil {
		return RuntimePlugins{}, fmt.Errorf("copy reused plugins: %w", err)
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		return RuntimePlugins{}, err
	}
	available := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			available[entry.Name()] = true
		}
	}
	paths := []string{}
	target := filepath.Join(targetRuntime, "plugins")
	for _, item := range launch.Plugins {
		// Basenames survive moving an archived Runtime to another root/platform.
		name := filepath.Base(filepath.FromSlash(strings.ReplaceAll(item.Path, `\`, "/")))
		if item.Path == "" || name == "." || name == ".." || !available[name] {
			return RuntimePlugins{}, fmt.Errorf("reused launch.json references a missing or duplicate plugin %q", item.Path)
		}
		delete(available, name)
		paths = append(paths, filepath.Join(target, name))
	}
	if len(available) > 0 {
		return RuntimePlugins{}, fmt.Errorf("reused plugins do not match the loading list in launch.json")
	}
	if _, err = os.Lstat(target); err == nil {
		return RuntimePlugins{}, fmt.Errorf("new Runtime plugins directory already exists")
	} else if !os.IsNotExist(err) {
		return RuntimePlugins{}, err
	}
	if err = os.Rename(stage, target); err != nil {
		return RuntimePlugins{}, err
	}
	return RuntimePlugins{Paths: paths, RootAgent: mainID}, nil
}

// Reuse retains executable hooks as well as file contents. Never follow links
// into the old Runtime or copy special files into the new Runtime.
func copyReusedPlugins(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(dest, 0700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("reused plugin must contain only regular files and directories: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err = os.WriteFile(dest, data, 0600); err != nil {
			return err
		}
		return os.Chmod(dest, info.Mode().Perm())
	})
}

func savedMainAgent(argv []string) (string, error) {
	mainID := ""
	found := false
	for i := 0; i < len(argv); i++ {
		value := ""
		switch {
		case argv[i] == "--agent":
			if i+1 >= len(argv) || strings.HasPrefix(argv[i+1], "--") {
				return "", fmt.Errorf("reused launch.json has an incomplete --agent argument")
			}
			i++
			value = argv[i]
		case strings.HasPrefix(argv[i], "--agent="):
			value = strings.TrimPrefix(argv[i], "--agent=")
		default:
			continue
		}
		if found || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("reused launch.json has ambiguous Main Agent arguments")
		}
		mainID, found = value, true
	}
	// Absence is intentional in ordinary mode: retain Claude's default root.
	return mainID, nil
}
