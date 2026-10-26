package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyTreeSkipsPluginBackupDirectories(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	for path, content := range map[string]string{
		filepath.Join("cc", "agents", "coordinator.md"):           "current",
		filepath.Join("cc", "backup", "agents", "coordinator.md"): "backup",
	} {
		full := filepath.Join(source, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := copyTree(source, target); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(filepath.Join(target, "cc", "agents", "coordinator.md")); err != nil || string(content) != "current" {
		t.Fatalf("active Agent was not copied: content=%q err=%v", content, err)
	}
	if _, err := os.Stat(filepath.Join(target, "cc", "backup")); !os.IsNotExist(err) {
		t.Fatalf("backup directory was copied: %v", err)
	}
}
