package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/app"
)

func TestCLITaskListContract(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	if code, _, stderr := runCLI("workspace", "init"); code != 0 {
		t.Fatalf("init: %d %s", code, stderr)
	}
	check := func(count int, args ...string) app.TaskList {
		t.Helper()
		code, stdout, stderr := runCLI(append([]string{"task", "list"}, args...)...)
		var result struct {
			OK      bool         `json:"ok"`
			Data    app.TaskList `json:"data"`
			Message string       `json:"message"`
		}
		if err := json.Unmarshal([]byte(stdout), &result); err != nil || code != 0 || stderr != "" || !result.OK || result.Data.Tasks == nil || len(result.Data.Tasks) != count || result.Message != "Task 列表已读取" {
			t.Fatalf("list: code=%d stdout=%s stderr=%s error=%v", code, stdout, stderr, err)
		}
		return result.Data
	}
	check(0)
	if code, _, stderr := runCLI("task", "open", "--purpose", "List workflow", "--concurrency", "1", "--info", "Subject"); code != 0 {
		t.Fatalf("open: %d %s", code, stderr)
	}
	check(1)
	if data := check(1, "--status", "open"); data.Tasks[0].Status != "open" {
		t.Fatalf("status filter failed: %#v", data)
	}
	check(0, "--status", "closed")
	if code, _, stderr := runCLI("task", "close", "--outcome", "succeeded", "--outcome-reason", "Done"); code != 0 {
		t.Fatalf("close: %d %s", code, stderr)
	}
	check(0, "--status", "open")
	check(1, "--status", "closed")
	for _, args := range [][]string{{"--status", "running"}, {"extra"}} {
		code, stdout, stderr := runCLI(append([]string{"task", "list"}, args...)...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, "INVALID_ARGUMENT") {
			t.Fatalf("invalid arguments accepted: code=%d stdout=%s stderr=%s", code, stdout, stderr)
		}
	}
}
