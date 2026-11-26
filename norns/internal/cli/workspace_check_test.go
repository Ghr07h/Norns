package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/app"
)

func TestCLIWorkspaceCheck(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	if code, _, stderr := runCLI("workspace", "init"); code != 0 {
		t.Fatalf("init: %d %s", code, stderr)
	}
	check := func(state string) app.WorkspaceCheck {
		t.Helper()
		code, stdout, stderr := runCLI("workspace", "check")
		var result struct {
			OK      bool               `json:"ok"`
			Data    app.WorkspaceCheck `json:"data"`
			Message string             `json:"message"`
		}
		if err := json.Unmarshal([]byte(stdout), &result); err != nil || code != 0 || stderr != "" || !result.OK || !result.Data.Valid || result.Data.State != state {
			t.Fatalf("check: code=%d stdout=%s stderr=%s error=%v", code, stdout, stderr, err)
		}
		var occupancy struct {
			Data struct {
				RuntimeOccupied *bool `json:"runtime_occupied"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(stdout), &occupancy); err != nil || occupancy.Data.RuntimeOccupied == nil || *occupancy.Data.RuntimeOccupied != (state == "open_task") {
			t.Fatalf("runtime occupancy is missing or incorrect: %s error=%v", stdout, err)
		}
		if state == "empty" && result.Message != "当前为空 Workspace" {
			t.Fatalf("empty Workspace message is missing: %s", stdout)
		}
		if entries, err := os.ReadDir(filepath.Join(".norns", "view")); err != nil && !os.IsNotExist(err) || len(entries) != 0 {
			t.Fatalf("check published a View: %v %v", entries, err)
		}
		return result.Data
	}
	empty := check("empty")
	if empty.Tasks == nil || len(*empty.Tasks) != 0 || empty.Task != nil {
		t.Fatalf("wrong empty result: %#v", empty)
	}
	if code, _, stderr := runCLI("task", "open", "--purpose", "Check lifecycle", "--concurrency", "1", "--info", "Subject"); code != 0 {
		t.Fatalf("open: %d %s", code, stderr)
	}
	open := check("open_task")
	if open.Task == nil || open.Task.Purpose != "Check lifecycle" || open.Tasks != nil || open.Summary.TaskCount != 1 || open.Summary.CognitionCount != 1 {
		t.Fatalf("wrong open result: %#v", open)
	}
	if code, _, stderr := runCLI("task", "close", "--outcome", "succeeded", "--outcome-reason", "Done"); code != 0 {
		t.Fatalf("close: %d %s", code, stderr)
	}
	closed := check("no_open_task")
	if closed.Task != nil || closed.Tasks == nil || len(*closed.Tasks) != 1 || (*closed.Tasks)[0].ID != open.Task.ID || (*closed.Tasks)[0].OutcomeReason == nil || *(*closed.Tasks)[0].OutcomeReason != "Done" {
		t.Fatalf("wrong historical result: %#v", closed)
	}
	if code, stdout, stderr := runCLI("workspace", "check", "extra"); code != 2 || stdout != "" || !strings.Contains(stderr, "INVALID_ARGUMENT") {
		t.Fatalf("unexpected argument was accepted: %d %s %s", code, stdout, stderr)
	}
}
