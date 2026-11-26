package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/app"
)

func TestCLIWorkspaceFree(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	commands := [][]string{
		{"task", "open", "--purpose", "Free workflow", "--concurrency", "1", "--info", "Subject"},
		{"exploration", "add", "--exploration-title", "Pending", "--exploration-content", "Pending work"},
		{"exploration", "add", "--exploration-title", "Running", "--exploration-content", "Running work"},
		{"exploration", "assign", "--exploration", "exp_2", "--explorer", "agent", "--output", "output://running"},
	}
	for _, args := range commands {
		if code, stdout, stderr := runCLI(args...); code != 0 || stderr != "" {
			t.Fatalf("%v: %d %s %s", args, code, stdout, stderr)
		}
	}
	for _, changed := range []bool{true, false} {
		code, stdout, stderr := runCLI("workspace", "free")
		var result struct {
			OK      bool              `json:"ok"`
			Data    app.WorkspaceFree `json:"data"`
			Message string            `json:"message"`
		}
		if err := json.Unmarshal([]byte(stdout), &result); err != nil || code != 0 || stderr != "" || !result.OK || result.Data.Changed != changed || result.Data.RuntimeOccupied || result.Message != "Workspace 占用已解除" {
			t.Fatalf("free: code=%d stdout=%s stderr=%s error=%v", code, stdout, stderr, err)
		}
		if changed && (result.Data.ClosedTaskID == nil || result.Data.CancelledTodo != 1 || result.Data.CancelledRunning != 1) {
			t.Fatalf("free lost cancellation results: %s", stdout)
		}
	}
	code, stdout, stderr := runCLI("workspace", "check")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"state":"no_open_task"`) || !strings.Contains(stdout, `"runtime_occupied":false`) || !strings.Contains(stdout, `"outcome":"cancelled"`) {
		t.Fatalf("free left Workspace occupied: %d %s %s", code, stdout, stderr)
	}
	if code, stdout, stderr := runCLI("workspace", "free", "extra"); code != 2 || stdout != "" || !strings.Contains(stderr, "INVALID_ARGUMENT") {
		t.Fatalf("free accepted an argument: %d %s %s", code, stdout, stderr)
	}
}
