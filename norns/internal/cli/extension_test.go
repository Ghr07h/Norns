package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/extension"
)

func TestCLIExtensionListWithoutWorkspace(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("NORNS_PLUGIN_PATH", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	var fixtures []string
	for _, name := range []string{"zulu", "alpha", "version", "extension", "task", "help"} {
		fixtures = append(fixtures, writeSiblingCLIExtension(t, name, "must not execute"))
	}
	code, stdout, stderr := runCLI("extension", "list")
	var envelope struct {
		OK   bool             `json:"ok"`
		Data []extension.Info `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || code != 0 || stderr != "" || !envelope.OK || len(envelope.Data) != 2 {
		t.Fatalf("list: code=%d stdout=%s stderr=%s err=%v", code, stdout, stderr, err)
	}
	if envelope.Data[0].Name != "alpha" || envelope.Data[1].Name != "zulu" {
		t.Fatalf("list must sort names and exclude core commands: %+v", envelope.Data)
	}
	for _, item := range envelope.Data {
		found, err := extension.Find(item.Name)
		if err != nil || found != item.Path || !filepath.IsAbs(item.Path) {
			t.Fatalf("wrong selected path: %+v found=%s err=%v", item, found, err)
		}
	}
	for _, filename := range fixtures {
		if err := os.Remove(filename); err != nil {
			t.Fatal(err)
		}
	}
	code, stdout, stderr = runCLI("extension", "list")
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || code != 0 || stderr != "" || envelope.Data == nil || len(envelope.Data) != 0 {
		t.Fatalf("empty list: code=%d stdout=%s stderr=%s err=%v", code, stdout, stderr, err)
	}
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 0 {
		t.Fatalf("listing must not create Workspace files: %v %v", entries, err)
	}
}

func TestCLIExtensionHelpAndInvalidArguments(t *testing.T) {
	t.Chdir(t.TempDir())
	code, stdout, stderr := runCLI("help", "extension")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "list") {
		t.Fatalf("help: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	for _, args := range [][]string{{"extension", "list", "extra"}, {"extension", "list", "--unknown"}, {"extension", "missing"}} {
		code, stdout, stderr := runCLI(args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, `"code":"INVALID_ARGUMENT"`) {
			t.Fatalf("%v: code=%d stdout=%s stderr=%s", args, code, stdout, stderr)
		}
	}
}

func TestCLIRejectsExtensionsOutsideExecutableDirectory(t *testing.T) {
	workspace, plugins, pathDir := t.TempDir(), t.TempDir(), t.TempDir()
	t.Chdir(workspace)
	t.Setenv("NORNS_PLUGIN_PATH", plugins)
	t.Setenv("PATH", pathDir)
	filename := "norns-external-only"
	if runtime.GOOS == "windows" {
		filename += ".exe"
	}
	for _, directory := range []string{workspace, plugins, pathDir} {
		if err := os.WriteFile(filepath.Join(directory, filename), []byte("must not execute"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	code, stdout, stderr := runCLI("external-only")
	if code != 1 || stdout != "" || !strings.Contains(stderr, `"code":"EXTENSION_NOT_FOUND"`) {
		t.Fatalf("external extension was not rejected: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestCLIExtensionMetadataAndSafeFallback(t *testing.T) {
	t.Chdir(t.TempDir())
	plugin := writeSiblingCLIExtension(t, "metadata-probe", "must not execute")
	filename := filepath.Join(filepath.Dir(plugin), "norns.exts.meta.json")
	t.Cleanup(func() { _ = os.Remove(filename) })
	read := func() map[string]any {
		t.Helper()
		code, stdout, stderr := runCLI("extension", "list")
		var response map[string]any
		if err := json.Unmarshal([]byte(stdout), &response); err != nil || code != 0 || stderr != "" {
			t.Fatalf("code=%d stdout=%s stderr=%s err=%v", code, stdout, stderr, err)
		}
		return response
	}
	if item := read()["data"].([]any)[0].(map[string]any); len(item) != 2 {
		t.Fatalf("missing catalog should preserve name/path: %v", item)
	}
	source := `[{"name":"metadata-probe","version":"0.1.0","description":"Static description","requirements":[]},{"name":"not-installed","version":"1","description":"Absent","requirements":[]}]`
	if err := os.WriteFile(filename, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	response := read()
	items := response["data"].([]any)
	item := items[0].(map[string]any)
	if len(items) != 1 || item["version"] != "0.1.0" || item["description"] != "Static description" || len(item["requirements"].([]any)) != 0 || len(response["warnings"].([]any)) != 0 {
		t.Fatalf("unexpected metadata projection: %v", response)
	}
	if err := os.WriteFile(filename, []byte(`{"secret":"DO_NOT_PRINT_THIS"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	response = read()
	if len(response["data"].([]any)[0].(map[string]any)) != 2 || len(response["warnings"].([]any)) != 1 {
		t.Fatalf("invalid catalog should warn and fall back: %v", response)
	}
	encoded, _ := json.Marshal(response)
	if strings.Contains(string(encoded), "DO_NOT_PRINT_THIS") {
		t.Fatal("raw metadata error leaked into output")
	}
}

func writeSiblingCLIExtension(t *testing.T, name, content string) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	filename := "norns-" + name
	if runtime.GOOS == "windows" {
		filename += ".exe"
	}
	path := filepath.Join(filepath.Dir(executable), filename)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	_, writeErr := file.WriteString(content)
	closeErr := file.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	return path
}
