package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/workspace"
)

type failingFileOps struct {
	base              workspace.FileOps
	failRename        func(oldPath, newPath string) bool
	failSyncFile      func(path string) bool
	failSyncDir       func(path string) bool
	failJournalRename int
	journalRenames    int
	failed            bool
}

func (f *failingFileOps) Rename(oldPath, newPath string) error {
	if newPath == filepath.Join(filepath.Dir(newPath), ".norns-reset.json") {
		f.journalRenames++
		if !f.failed && f.failJournalRename == f.journalRenames {
			f.failed = true
			return errors.New("injected journal rename failure")
		}
	}
	if !f.failed && f.failRename != nil && f.failRename(oldPath, newPath) {
		f.failed = true
		return errors.New("injected reset rename failure")
	}
	return f.base.Rename(oldPath, newPath)
}

func (f *failingFileOps) SyncFile(path string) error {
	if !f.failed && f.failSyncFile != nil && f.failSyncFile(path) {
		f.failed = true
		return errors.New("injected file sync failure")
	}
	return f.base.SyncFile(path)
}

func (f *failingFileOps) SyncDir(path string) error {
	if !f.failed && f.failSyncDir != nil && f.failSyncDir(path) {
		f.failed = true
		return errors.New("injected directory sync failure")
	}
	return f.base.SyncDir(path)
}

func (f *failingFileOps) RemoveAll(path string) error { return f.base.RemoveAll(path) }

func TestResetRecoversFromInjectedPhaseFailures(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Service, *failingFileOps)
		keepsOld  bool
	}{
		{"prepared-log", func(_ *Service, files *failingFileOps) { files.failJournalRename = 1 }, true},
		{"old-root-rename", func(service *Service, files *failingFileOps) {
			files.failRename = func(oldPath, newPath string) bool {
				return oldPath == service.Paths.Norns && strings.HasPrefix(filepath.Base(newPath), ".norns.old.")
			}
		}, true},
		{"old-renamed-log", func(_ *Service, files *failingFileOps) { files.failJournalRename = 2 }, false},
		{"new-root-publish", func(service *Service, files *failingFileOps) {
			files.failRename = func(oldPath, newPath string) bool {
				return strings.HasPrefix(filepath.Base(oldPath), ".norns.new.") && newPath == service.Paths.Norns
			}
		}, false},
		{"new-published-log", func(_ *Service, files *failingFileOps) { files.failJournalRename = 3 }, false},
		{"old-root-archive", func(service *Service, files *failingFileOps) {
			files.failRename = func(oldPath, newPath string) bool {
				return strings.HasPrefix(filepath.Base(oldPath), ".norns.old.") && filepath.Dir(newPath) == service.Paths.History
			}
		}, false},
		{"archived-log", func(_ *Service, files *failingFileOps) { files.failJournalRename = 4 }, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, _ := newTestService(t)
			createClosedTask(t, service)
			files := &failingFileOps{base: workspace.OSFileOps{}}
			test.configure(service, files)
			service.Files = files
			if _, err := service.Reset(context.Background()); err == nil || !files.failed {
				t.Fatalf("reset did not reach injected failure: %v", err)
			}
			service.Files = workspace.OSFileOps{}
			if _, err := service.Init(context.Background()); err != nil {
				t.Fatalf("next command could not recover reset: %v", err)
			}
			assertRecoveredTaskCount(t, service, boolCount(test.keepsOld))
			if _, err := os.Stat(service.Paths.ResetLog); !os.IsNotExist(err) {
				t.Fatalf("reset journal remains after recovery: %v", err)
			}
			if !test.keepsOld {
				entries, err := os.ReadDir(service.Paths.History)
				if err != nil || len(entries) != 1 {
					t.Fatalf("old Workspace was not archived: entries=%v err=%v", entries, err)
				}
			}
		})
	}
}

func TestResetRecoversAfterPartialHistoryTransfer(t *testing.T) {
	service, _ := newTestService(t)
	createClosedTask(t, service)
	for _, name := range []string{"a", "b"} {
		directory := filepath.Join(service.Paths.History, name)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "marker"), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files := &failingFileOps{base: workspace.OSFileOps{}}
	files.failRename = func(oldPath, newPath string) bool {
		return filepath.Base(oldPath) == "b" && filepath.Dir(newPath) == service.Paths.History
	}
	service.Files = files
	if _, err := service.Reset(context.Background()); err == nil || !files.failed {
		t.Fatalf("reset did not fail during history transfer: %v", err)
	}
	service.Files = workspace.OSFileOps{}
	if _, err := service.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertRecoveredTaskCount(t, service, 0)
	for _, name := range []string{"a", "b"} {
		content, err := os.ReadFile(filepath.Join(service.Paths.History, name, "marker"))
		if err != nil || string(content) != name {
			t.Fatalf("history %s was lost: %q %v", name, content, err)
		}
	}
}

func TestResetRefusalDoesNotMoveWorkspace(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(*testing.T, *Service)
		wantCode string
		verifyDB func(*testing.T, *sql.DB)
	}{
		{
			name: "open-task",
			prepare: func(t *testing.T, service *Service) {
				t.Helper()
				info := "Subject"
				if _, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Open", Concurrency: 1, Info: &info}); err != nil {
					t.Fatal(err)
				}
			},
			wantCode: "RESET_OPEN_TASK",
			verifyDB: func(t *testing.T, db *sql.DB) {
				t.Helper()
				var status string
				if err := db.QueryRow(`SELECT status FROM tasks`).Scan(&status); err != nil || status != "open" {
					t.Fatalf("open Task changed after reset refusal: status=%q err=%v", status, err)
				}
			},
		},
		{
			name: "unsupported-schema",
			prepare: func(t *testing.T, service *Service) {
				t.Helper()
				createClosedTask(t, service)
				execDatabase(t, service.Paths.Database, `PRAGMA user_version = 999`)
			},
			wantCode: "UNSUPPORTED_SCHEMA_VERSION",
			verifyDB: func(t *testing.T, db *sql.DB) {
				t.Helper()
				var version int
				if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 999 {
					t.Fatalf("schema version changed after reset refusal: version=%d err=%v", version, err)
				}
			},
		},
		{
			name: "unsupported-protocol",
			prepare: func(t *testing.T, service *Service) {
				t.Helper()
				createClosedTask(t, service)
				execDatabase(t, service.Paths.Database, `UPDATE workspace_meta SET protocol_version = 999`)
			},
			wantCode: "UNSUPPORTED_PROTOCOL_VERSION",
			verifyDB: func(t *testing.T, db *sql.DB) {
				t.Helper()
				var version int
				if err := db.QueryRow(`SELECT protocol_version FROM workspace_meta`).Scan(&version); err != nil || version != 999 {
					t.Fatalf("protocol version changed after reset refusal: version=%d err=%v", version, err)
				}
			},
		},
		{
			name: "untrusted-state",
			prepare: func(t *testing.T, service *Service) {
				t.Helper()
				info := "Subject"
				if _, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Corrupt", Concurrency: 1, Info: &info}); err != nil {
					t.Fatal(err)
				}
				execDatabase(t, service.Paths.Database, `UPDATE workspace_meta SET runtime_occupied = 0`)
			},
			wantCode: "WORKSPACE_STATE_UNTRUSTED",
			verifyDB: func(t *testing.T, db *sql.DB) {
				t.Helper()
				var runtime int
				if err := db.QueryRow(`SELECT runtime_occupied FROM workspace_meta`).Scan(&runtime); err != nil || runtime != 0 {
					t.Fatalf("untrusted state changed after reset refusal: runtime=%d err=%v", runtime, err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, _ := newTestService(t)
			test.prepare(t, service)
			if _, err := service.Reset(context.Background()); errorCode(err) != test.wantCode {
				t.Fatalf("reset returned %q, want %q: %v", errorCode(err), test.wantCode, err)
			}
			if _, err := os.Stat(service.Paths.Database); err != nil {
				t.Fatalf("Workspace database moved after reset refusal: %v", err)
			}
			for _, path := range []string{service.Paths.ResetLog} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("reset artifact exists after refusal: %s: %v", path, err)
				}
			}
			entries, err := os.ReadDir(service.Paths.Root)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".norns.old.") || strings.HasPrefix(entry.Name(), ".norns.new.") {
					t.Fatalf("reset staging artifact exists after refusal: %s", entry.Name())
				}
			}
			history, err := os.ReadDir(service.Paths.History)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if len(history) != 0 {
				t.Fatalf("reset refusal created an archive: %v", history)
			}
			db, err := sql.Open("sqlite", service.Paths.Database)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			test.verifyDB(t, db)
		})
	}
}

func execDatabase(t *testing.T, path, statement string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(statement); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertRecoveredTaskCount(t *testing.T, service *Service, want int) {
	t.Helper()
	view, _, err := service.StatusView(context.Background(), nil, true)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Tasks []any `json:"tasks"`
	}
	if err := json.Unmarshal(view, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Tasks) != want {
		t.Fatalf("recovered Workspace has %d Tasks, want %d", len(decoded.Tasks), want)
	}
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
