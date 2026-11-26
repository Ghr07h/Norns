package plugin

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ccagent "github.com/Ghr07h/norns/nornsd/internal/agent/cc"
	"github.com/Ghr07h/norns/nornsd/internal/config"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	"github.com/Ghr07h/norns/nornsd/internal/runtimeartifact"
)

func putPluginFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func pluginFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(dir, path)
			files[rel] = string(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func savePluginLaunch(t *testing.T, subject, id string, prepared RuntimePlugins) {
	t.Helper()
	_, err := runtimeartifact.Publish(subject, runtimeartifact.Input{
		Runtime: domain.Runtime{ID: id, Profile: domain.RuntimeProfile{Agent: "cc"}},
		Launch:  ccagent.Launch{Exec: []string{"/bin/echo"}, SessionID: id, PluginPaths: prepared.Paths, RootAgent: prepared.RootAgent},
		Managed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPlainMainAgentPublishesUnmodifiedAndSelectsName(t *testing.T) {
	subject := filepath.Join(t.TempDir(), "subject_stamp")
	source := filepath.Join(t.TempDir(), "different-filename.md")
	original := "---\nname: math-main\nunknown: {nested: true}\n---\n{{NORNS_RUNTIME_CONFIGURATION}}\n"
	putPluginFile(t, source, original)
	for _, agents := range []string{"", filepath.Dir(source)} {
		id := "main-only"
		if agents != "" {
			id = "same-file-in-agents"
		}
		prepared, err := Prepare("cc", subject, id, &config.Config{MainAgentFile: source, CustomAgentsDir: agents})
		if err != nil {
			t.Fatal(err)
		}
		if len(prepared.Paths) != 1 || prepared.RootAgent != "nornsd-custom:math-main" {
			t.Fatalf("unexpected main publication: %+v", prepared)
		}
		files := pluginFiles(t, prepared.Paths[0])
		if files[filepath.Join("agents", filepath.Base(source))] != original || files["plugin.manifest.json"] == "" {
			t.Fatalf("main rewritten or absent from plugin: %#v", files)
		}
	}
}

func TestReuseCopiesNornsPluginsWithoutRewriting(t *testing.T) {
	subject := filepath.Join(t.TempDir(), "subject_stamp")
	sourceMain := filepath.Join(t.TempDir(), "main.md")
	putPluginFile(t, sourceMain, "---\nname: specialist\ntools: Read, Agent\nmaxTurns: 50\n---\n{{NORNS_RUNTIME_CONFIGURATION}}\n")
	prepared, err := Prepare("cc", subject, "old", &config.Config{NornsEnable: true, MainAgentFile: sourceMain, NornsMaxConcurrentExplorations: 7})
	if err != nil {
		t.Fatal(err)
	}
	savePluginLaunch(t, subject, "old", prepared)
	oldRuntime := root.RuntimeDataPath(subject, "old")
	before := pluginFiles(t, filepath.Join(oldRuntime, "plugins"))
	newSubject := filepath.Join(t.TempDir(), "newsubject_stamp")
	reused, err := Prepare("cc", newSubject, "new", &config.Config{NornsEnable: true, ReuseRuntimeDir: oldRuntime, LLMModel: "new", NornsMaxConcurrentExplorations: 99, NornsExplorationTimeoutSeconds: 1, NornsAllowHumanIntervention: true})
	if err != nil {
		t.Fatal(err)
	}
	if reused.RootAgent != "norns:specialist" || len(reused.Paths) != 1 {
		t.Fatalf("main not restored: %+v", reused)
	}
	want := filepath.Join(root.RuntimeDataPath(newSubject, "new"), "plugins", "norns-plugin")
	want, err = filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatal(err)
	}
	if reused.Paths[0] != want || !reflect.DeepEqual(before, pluginFiles(t, filepath.Dir(want))) {
		t.Fatal("reused tree changed or references old Runtime")
	}
	savePluginLaunch(t, newSubject, "new", reused)
	launch, _ := os.ReadFile(filepath.Join(root.RuntimeDataPath(newSubject, "new"), "launch.json"))
	if strings.Contains(string(launch), oldRuntime) || !strings.Contains(string(launch), want) {
		t.Fatalf("new startup must reference only new paths: %s", launch)
	}
	putPluginFile(t, filepath.Join(want, "agents", "main.md"), "changed copy")
	if !reflect.DeepEqual(before, pluginFiles(t, filepath.Join(oldRuntime, "plugins"))) {
		t.Fatal("modifying the new Runtime changed its source")
	}
}

func TestReusePreservesPlainPluginOrderAndExecutableHooks(t *testing.T) {
	source := t.TempDir()
	// Intentionally differs from directory order and remains valid after moving.
	plugins := []map[string]string{{"path": "/archived/plugins/external-plugin-2"}, {"path": `C:\old\plugins\external-plugin-10`}}
	launch, _ := json.Marshal(map[string]any{"agent": map[string]any{"type": "cc", "argv": []string{"claude", "--agent=nornsd-custom:math"}}, "plugins": plugins})
	putPluginFile(t, filepath.Join(source, "launch.json"), string(launch))
	for _, name := range []string{"external-plugin-10", "external-plugin-2"} {
		putPluginFile(t, filepath.Join(source, "plugins", name, "plugin.manifest.json"), `{"runtime_id":"old"}`)
	}
	hook := filepath.Join(source, "plugins", "external-plugin-2", "hook.sh")
	putPluginFile(t, hook, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(hook, 0750); err != nil {
		t.Fatal(err)
	}
	subject := filepath.Join(t.TempDir(), "subject_stamp")
	got, err := Prepare("cc", subject, "new", &config.Config{ReuseRuntimeDir: source})
	if err != nil {
		t.Fatal(err)
	}
	if got.RootAgent != "nornsd-custom:math" || len(got.Paths) != 2 || filepath.Base(got.Paths[0]) != "external-plugin-2" || filepath.Base(got.Paths[1]) != "external-plugin-10" {
		t.Fatalf("order/main lost: %+v", got)
	}
	info, err := os.Stat(filepath.Join(got.Paths[0], "hook.sh"))
	if err != nil || info.Mode().Perm() != 0750 {
		t.Fatalf("hook permissions lost: %v %v", info, err)
	}
}

func TestReusePlainRuntimeWithoutPluginsOrCustomMain(t *testing.T) {
	subject := filepath.Join(t.TempDir(), "subject_stamp")
	savePluginLaunch(t, subject, "old", RuntimePlugins{})
	got, err := Prepare("cc", subject, "new", &config.Config{ReuseRuntimeDir: root.RuntimeDataPath(subject, "old")})
	if err != nil || len(got.Paths) != 0 || got.RootAgent != "" {
		t.Fatalf("default CC main not preserved: %+v %v", got, err)
	}
}

func TestReuseRejectsIncompleteOrAmbiguousSource(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{"missing-launch", func(t *testing.T, dir string) { _ = os.Remove(filepath.Join(dir, "launch.json")) }},
		{"invalid-json", func(t *testing.T, dir string) { putPluginFile(t, filepath.Join(dir, "launch.json"), "{") }},
		{"wrong-agent", func(t *testing.T, dir string) {
			putPluginFile(t, filepath.Join(dir, "launch.json"), `{"agent":{"type":"other","argv":["agent"]}}`)
		}},
		{"missing-plugin", func(t *testing.T, dir string) { _ = os.RemoveAll(filepath.Join(dir, "plugins", "norns-plugin")) }},
		{"extra-plugin", func(t *testing.T, dir string) {
			putPluginFile(t, filepath.Join(dir, "plugins", "extra", "file"), "extra")
		}},
		{"symlink-plugin", func(t *testing.T, dir string) {
			if err := os.Symlink(t.TempDir(), filepath.Join(dir, "plugins", "linked")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := t.TempDir()
			putPluginFile(t, filepath.Join(source, "launch.json"), `{"agent":{"type":"cc","argv":["claude","--agent","norns:coordinator"]},"plugins":[{"path":"/old/plugins/norns-plugin"}]}`)
			putPluginFile(t, filepath.Join(source, "plugins", "norns-plugin", "file"), "original")
			tc.setup(t, source)
			if _, err := Prepare("cc", filepath.Join(t.TempDir(), "subject_stamp"), "new", &config.Config{ReuseRuntimeDir: source}); err == nil {
				t.Fatal("invalid reuse source accepted")
			}
		})
	}
	for _, argv := range [][]string{{"claude", "--agent"}, {"claude", "--agent="}, {"claude", "--agent", "--debug"}, {"claude", "--agent", "one", "--agent=two"}} {
		if _, err := savedMainAgent(argv); err == nil {
			t.Fatalf("ambiguous main accepted: %v", argv)
		}
	}
	subject := filepath.Join(t.TempDir(), "subject_stamp")
	source := root.RuntimeDataPath(subject, "same")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare("cc", subject, "same", &config.Config{ReuseRuntimeDir: source}); err == nil {
		t.Fatal("source and target may not be the same Runtime")
	}
}
