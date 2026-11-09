package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommandsIndependentOfWorkspaceDoNotUseLock(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir(".norns.lock", 0o700); err != nil {
		t.Fatal(err)
	}
	commands := [][]string{
		{"--help"}, {"help"}, {"help", "task", "open"}, {"task", "--help"},
		{"view", "status", "--help"}, {"version"}, {"version", "--json"},
	}
	for _, args := range commands {
		if code, _, stderr := runCLI(args...); code != 0 {
			t.Fatalf("%v unexpectedly depended on the workspace lock: code=%d stderr=%s", args, code, stderr)
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil || len(entries) != 1 || entries[0].Name() != ".norns.lock" || !entries[0].IsDir() {
		t.Fatalf("independent commands changed workspace state: entries=%v err=%v", entries, err)
	}
}

func TestCLIWorkspaceCommandsCleanLifecycleLock(t *testing.T) {
	t.Chdir(t.TempDir())
	commands := [][]string{
		{"workspace", "init"},
		{"view", "status"},
		{"task", "open", "--purpose", "Work", "--concurrency", "1", "--info", "Subject"},
		{"workspace", "reset"},
		{"task", "close", "--outcome", "cancelled", "--outcome-reason", "Done"},
		{"workspace", "reset"},
	}
	for _, args := range commands {
		code, _, stderr := runCLI(args...)
		if code != 0 && errorEnvelopeCode(stderr) != "RESET_OPEN_TASK" {
			t.Fatalf("%v failed unexpectedly: code=%d stderr=%s", args, code, stderr)
		}
		if _, err := os.Stat(filepath.Join(".", ".norns.lock")); !os.IsNotExist(err) {
			t.Fatalf("%v left a lifecycle lock: %v", args, err)
		}
	}
}
