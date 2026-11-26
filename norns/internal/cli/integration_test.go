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

	"github.com/Ghr07h/norns/norns/internal/domain"
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
	buildWebsearch := exec.Command("go", "build", "-o", websearchBinary, "./internal/custom_extensions/websearch/cmd/norns-websearch")
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
			ID               string `json:"id"`
			InitCognitionRef string `json:"init_cognition_ref"`
		} `json:"data"`
	}
	if err := json.Unmarshal(opened, &openEnvelope); err != nil || !openEnvelope.OK || openEnvelope.Data.ID == "" || openEnvelope.Data.InitCognitionRef != "cog_1" {
		t.Fatalf("real binary returned an invalid task open envelope: %v\n%s", err, opened)
	}
	added := runBinary(t, binary, workspace, "exploration", "add", "--exploration-title", "Inspect", "--exploration-content", "Inspect the implementation")
	var addEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(added, &addEnvelope); err != nil || !addEnvelope.OK || addEnvelope.Data.ID != "exp_1" {
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
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"Status", []string{"view", "status", "--md"}},
		{"Trace", []string{"view", "trace", "--exploration", addEnvelope.Data.ID, "--md"}},
	} {
		markdown := runBinary(t, binary, workspace, tc.args...)
		if !bytes.HasPrefix(markdown, []byte("# "+tc.name+" View\n\n")) {
			t.Fatalf("real CLI did not return Markdown: %s", markdown)
		}
		var headings []string
		for _, line := range strings.Split(string(markdown), "\n") {
			if strings.HasPrefix(line, "## ") {
				headings = append(headings, line)
			}
		}
		if len(headings) < 2 || headings[1] != "## Graph" || !bytes.Contains(markdown, []byte("exp_1: (todo) Inspect <- ['cog_1']")) {
			t.Fatalf("real CLI lost the compact Graph: %s", markdown)
		}

	}
	global := binaryEntityID(t, runBinary(t, binary, workspace, "cognition", "add", "--origin", "user", "--cognition-title", "Global", "--cognition-content", "Global assertion", "--meta", `{"type":"theorem","scope":"geometry"}`))
	hint := binaryEntityID(t, runBinary(t, binary, workspace, "cognition", "add", "--origin", "user", "--exploration", addEnvelope.Data.ID, "--cognition-title", "Hint", "--cognition-content", "User exploration assertion"))
	if global != "cog_2" || hint != "cog_3" {
		t.Fatalf("wrong Cog sequence: %s %s", global, hint)
	}
	dependent := binaryEntityID(t, runBinary(t, binary, workspace, "exploration", "add", "--exploration-title", "Dependent", "--exploration-content", "Use the user hint", "--source-cognitions", hint, "--source-cognitions", global))
	assigned := runBinary(t, binary, workspace, "exploration", "assign", "--exploration", addEnvelope.Data.ID, "--explorer", "integration-explorer", "--output", "output://integration")
	var assignEnvelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(assigned, &assignEnvelope); err != nil || assignEnvelope.Data["explorer_id"] != "integration-explorer" || assignEnvelope.Data["executor_id"] != nil {
		t.Fatalf("assign used the wrong Explorer field: %s %v", assigned, err)
	}
	cognitionDetail := runBinary(t, binary, workspace, "cognition", "get", "--cognition", global)
	var cognitionEnvelope struct {
		OK   bool             `json:"ok"`
		Data domain.Cognition `json:"data"`
	}
	if err := json.Unmarshal(cognitionDetail, &cognitionEnvelope); err != nil || !cognitionEnvelope.OK || cognitionEnvelope.Data.ID != global || cognitionEnvelope.Data.Title != "Global" || cognitionEnvelope.Data.Meta["type"] != "theorem" || cognitionEnvelope.Data.TaskID != nil {
		t.Fatalf("get lost global Cognition details: %v %s", err, cognitionDetail)
	}
	explorationDetail := runBinary(t, binary, workspace, "exploration", "get", "--exploration", addEnvelope.Data.ID)
	var explorationEnvelope struct {
		OK   bool               `json:"ok"`
		Data domain.Exploration `json:"data"`
	}
	if err := json.Unmarshal(explorationDetail, &explorationEnvelope); err != nil || !explorationEnvelope.OK || explorationEnvelope.Data.Status != "running" || explorationEnvelope.Data.ExplorerID == nil || *explorationEnvelope.Data.ExplorerID != "integration-explorer" || len(explorationEnvelope.Data.Hints) != 1 || explorationEnvelope.Data.Hints[0] != hint {
		t.Fatalf("get lost running Exploration details: %v %s", err, explorationDetail)
	}
	trace := runBinary(t, binary, workspace, "view", "trace", "--exploration", dependent, "--json")
	var traceView struct {
		Related struct {
			Cognitions   []map[string]any `json:"cognitions"`
			Explorations []struct {
				ID      string           `json:"id"`
				Sources []string         `json:"sources"`
				Hints   []map[string]any `json:"hints"`
			} `json:"explorations"`
		} `json:"related_subgraph"`
		Global  []map[string]any `json:"global_hints"`
		Running []struct {
			ID         string `json:"id"`
			ExplorerID string `json:"explorer_id"`
		} `json:"running_explorations"`
	}
	if err := json.Unmarshal(trace, &traceView); err != nil {
		t.Fatal(err)
	}
	if len(traceView.Related.Cognitions) != 1 || len(traceView.Related.Explorations) != 2 || len(traceView.Global) != 1 || traceView.Global[0]["id"] != global || len(traceView.Global[0]) != 3 {
		t.Fatalf("wrong protocol projection: %s", trace)
	}
	if hints := traceView.Related.Explorations[0].Hints; len(hints) != 1 || hints[0]["id"] != hint || len(hints[0]) != 3 {
		t.Fatalf("missing hint owner: %s", trace)
	}
	if sources := traceView.Related.Explorations[1].Sources; len(sources) != 2 || sources[0] != hint || sources[1] != global {
		t.Fatalf("CLI lost source order: %s", trace)
	}
	if len(traceView.Running) != 1 || traceView.Running[0].ID != addEnvelope.Data.ID || traceView.Running[0].ExplorerID != "integration-explorer" {
		t.Fatalf("Trace lost the assigned Explorer ID: %s", trace)
	}
	runBinary(t, binary, workspace, "exploration", "finish", "--exploration", addEnvelope.Data.ID, "--outcome", "cancelled", "--outcome-reason", "Retry integration")
	retried := runBinary(t, binary, workspace, "exploration", "retry", "--exploration", addEnvelope.Data.ID)
	var retryEnvelope struct {
		Data struct {
			Hints []string `json:"hints"`
		} `json:"data"`
	}
	if err := json.Unmarshal(retried, &retryEnvelope); err != nil || retryEnvelope.Data.Hints == nil || len(retryEnvelope.Data.Hints) != 0 {
		t.Fatalf("retry copied hints: %s", retried)
	}
	visual := runBinary(t, binary, workspace, "view", "visual", "--lang", "en")
	var visualEnvelope struct {
		Data struct {
			Path string `json:"path"`
		} `json:"data"`
	}
	if err := json.Unmarshal(visual, &visualEnvelope); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(workspace, visualEnvelope.Data.Path))
	if err != nil || !strings.Contains(string(html), `"meta":{"scope":"geometry","type":"theorem"}`) {
		t.Fatalf("Visual lost persisted metadata: %v", err)
	}
	largeContent := strings.TrimSpace(strings.Repeat("Large finding. ", 600))
	largeCog := binaryEntityID(t, runBinary(t, binary, workspace, "cognition", "add", "--origin", "exploration", "--exploration", dependent, "--cognition-title", "Large", "--cognition-content", largeContent))
	for _, args := range [][]string{
		{"view", "status"},
		{"view", "trace", "--exploration", dependent, "--json"},
		{"view", "status", "--md"},
		{"view", "trace", "--exploration", dependent, "--md"},
	} {
		payload := runBinary(t, binary, workspace, args...)
		var envelope struct {
			OK   bool          `json:"ok"`
			Data publishedView `json:"data"`
		}
		if err := json.Unmarshal(payload, &envelope); err != nil || !envelope.OK || envelope.Data.Note == "" || envelope.Data.SizeBytes <= structuredViewInlineLimit || (args[len(args)-1] == "--md" && (envelope.Data.Format != "md" || !strings.HasSuffix(envelope.Data.Path, ".md"))) || bytes.Contains(payload, []byte(largeContent)) {
			t.Fatalf("large View did not return a file note: %v %s", err, payload)
		}
		content, err := os.ReadFile(filepath.Join(workspace, envelope.Data.Path))
		if err != nil || len(content) != envelope.Data.SizeBytes || !bytes.Contains(content, []byte(largeCog)) {
			t.Fatalf("large View file is incomplete: %v", err)
		}
	}
	runBinary(t, binary, workspace, "task", "close", "--outcome", "succeeded", "--outcome-reason", "Integration done")
	cognitionDetail = runBinary(t, binary, workspace, "cognition", "get", "--cognition", largeCog)
	if err := json.Unmarshal(cognitionDetail, &cognitionEnvelope); err != nil || !cognitionEnvelope.OK || cognitionEnvelope.Data.Content != largeContent {
		t.Fatalf("get did not return full large Cognition after Task close: %v", err)
	}
	explorationDetail = runBinary(t, binary, workspace, "exploration", "get", "--exploration", addEnvelope.Data.ID)
	if err := json.Unmarshal(explorationDetail, &explorationEnvelope); err != nil || !explorationEnvelope.OK || explorationEnvelope.Data.Status != "finished" || explorationEnvelope.Data.FinishedAt == nil {
		t.Fatalf("get did not return historical Exploration: %v %s", err, explorationDetail)
	}
	runBinary(t, binary, workspace, "workspace", "reset")
	opened = runBinary(t, binary, workspace, "task", "open", "--purpose", "Fresh Workspace", "--concurrency", "1", "--info", "Fresh information")
	if err := json.Unmarshal(opened, &openEnvelope); err != nil || openEnvelope.Data.InitCognitionRef != "cog_1" {
		t.Fatalf("reset did not restart Cog sequence: %v %s", err, opened)
	}
	freshExp := binaryEntityID(t, runBinary(t, binary, workspace, "exploration", "add", "--exploration-title", "Fresh", "--exploration-content", "Inspect fresh information"))
	if freshExp != "exp_1" {
		t.Fatalf("reset did not restart Exploration sequence: %s", freshExp)
	}

}

func binaryEntityID(t *testing.T, payload []byte) string {
	t.Helper()
	var value struct {
		OK   bool `json:"ok"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &value); err != nil || !value.OK || value.Data.ID == "" {
		t.Fatalf("invalid entity result: %s", payload)
	}
	return value.Data.ID
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
