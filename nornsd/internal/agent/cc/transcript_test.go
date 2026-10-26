package cc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveTranscriptUsesDefaultClaudeProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	subjectPath := filepath.Join(t.TempDir(), "subject")
	if err := os.MkdirAll(subjectPath, 0700); err != nil {
		t.Fatal(err)
	}
	key, err := ProjectKey(subjectPath)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := "session-1"
	project := filepath.Join(home, ".claude", "projects", key)
	if err = os.MkdirAll(filepath.Join(project, sessionID), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(project, sessionID+".jsonl"), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(project, sessionID, "subagent.jsonl"), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := ArchiveTranscript(subjectPath, sessionID, "runtime-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "complete" || len(result.Missing) != 0 {
		t.Fatalf("archive result=%#v", result)
	}
	if result.SessionID != sessionID {
		t.Fatalf("archive session ID=%q, want %q", result.SessionID, sessionID)
	}
	for _, relative := range []string{sessionID + ".jsonl", filepath.Join(sessionID, "subagent.jsonl")} {
		if _, err = os.Stat(filepath.Join(subjectPath, ".nornsd", "runtimes", "runtime-1", relative)); err != nil {
			t.Fatalf("archived %s: %v", relative, err)
		}
	}
}

func TestArchiveTranscriptReportsMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	subjectPath := filepath.Join(t.TempDir(), "subject")
	if err := os.MkdirAll(subjectPath, 0700); err != nil {
		t.Fatal(err)
	}
	result, err := ArchiveTranscript(subjectPath, "missing-session", "runtime-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "missing" || len(result.Copied) != 0 || len(result.Missing) != 2 {
		t.Fatalf("archive result=%#v", result)
	}
}
