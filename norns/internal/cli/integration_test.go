package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIntegrationBinaryWorkflow(t *testing.T) {
	if os.Getenv("NORNS_RUN_BINARY_INTEGRATION") != "1" {
		t.Skip("set NORNS_RUN_BINARY_INTEGRATION=1 to execute a freshly built binary")
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate integration test source")
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	binaryName := "norns"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	buildDir := t.TempDir()
	if runtime.GOOS != "windows" {
		var err error
		buildDir, err = os.MkdirTemp("/tmp", "norns-integration-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(buildDir) })
	}
	binary := filepath.Join(buildDir, binaryName)
	build := exec.Command("go", "build", "-o", binary, "./cmd/norns")
	build.Dir = moduleRoot
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cannot build real norns binary: %v\n%s", err, output)
	}
	websearchName := "norns-websearch"
	if runtime.GOOS == "windows" {
		websearchName += ".exe"
	}
	websearchBinary := filepath.Join(buildDir, websearchName)
	buildWebsearch := exec.Command("go", "build", "-o", websearchBinary, "./cmd/norns-websearch")
	buildWebsearch.Dir = moduleRoot
	buildWebsearch.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := buildWebsearch.CombinedOutput(); err != nil {
		t.Fatalf("cannot build real norns-websearch binary: %v\n%s", err, output)
	}

	workspace := t.TempDir()
	if runtime.GOOS != "windows" {
		workspace = filepath.Join(buildDir, "workspace")
		if err := os.Mkdir(workspace, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	help := runBinary(t, binary, workspace, "websearch", "--help")
	if !strings.HasPrefix(string(help), "Usage: norns websearch") {
		t.Fatalf("real websearch extension returned unexpected help: %s", help)
	}
	runBinary(t, binary, workspace, "workspace", "init")
	opened := runBinary(t, binary, workspace, "task", "open", "--purpose", "Integration", "--concurrency", "2", "--info", "Subject")
	var openEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(opened, &openEnvelope); err != nil || !openEnvelope.OK || openEnvelope.Data.ID == "" {
		t.Fatalf("real binary returned an invalid task open envelope: %v\n%s", err, opened)
	}
	added := runBinary(t, binary, workspace, "exploration", "add", "--exploration-title", "Inspect", "--exploration-content", "Inspect the implementation")
	var addEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(added, &addEnvelope); err != nil || !addEnvelope.OK || addEnvelope.Data.ID == "" {
		t.Fatalf("real binary returned an invalid exploration envelope: %v\n%s", err, added)
	}
	status := runBinary(t, binary, workspace, "view", "status", "--json")
	var statusView struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
		Todo []struct {
			ID string `json:"id"`
		} `json:"todo_explorations"`
	}
	if err := json.Unmarshal(status, &statusView); err != nil || statusView.Task.ID != openEnvelope.Data.ID || len(statusView.Todo) != 1 || statusView.Todo[0].ID != addEnvelope.Data.ID {
		t.Fatalf("real binary Status does not reflect writes: %v\n%s", err, status)
	}
}

func runBinary(t *testing.T, binary, workspace string, args ...string) []byte {
	t.Helper()
	command := exec.Command(binary, args...)
	command.Dir = workspace
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("norns %v failed: %v\nstdout: %s\nstderr: %s", args, err, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("norns %v wrote stderr on success: %s", args, stderr.String())
	}
	return stdout.Bytes()
}
