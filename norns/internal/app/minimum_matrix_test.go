package app

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestUninitializedWorkspaceRejectsAllCommandsExceptInitAndTaskOpen(t *testing.T) {
	service, _ := newTestService(t)
	id := "exp_ffffffff-ffff-4fff-8fff-ffffffffffff"
	tests := []struct {
		name string
		call func() error
	}{
		{"task-close", func() error { _, err := service.CloseTask(context.Background(), "cancelled", "No Task"); return err }},
		{"task-reopen", func() error { _, err := service.ReopenTask(context.Background(), nil); return err }},
		{"cognition-add", func() error {
			_, err := service.AddCognition(context.Background(), AddCognitionRequest{Title: "Hint", Content: "Content", Origin: "user"})
			return err
		}},
		{"exploration-add", func() error {
			_, err := service.AddExploration(context.Background(), AddExplorationRequest{Title: "Work", Content: "Content"})
			return err
		}},
		{"exploration-retry", func() error { _, err := service.RetryExploration(context.Background(), id); return err }},
		{"exploration-assign", func() error {
			_, err := service.AssignExploration(context.Background(), id, "executor", "output://one")
			return err
		}},
		{"exploration-finish", func() error {
			_, err := service.FinishExploration(context.Background(), id, "cancelled", "No Task")
			return err
		}},
		{"exploration-todo", func() error { _, err := service.ListExplorations(context.Background(), "todo"); return err }},
		{"exploration-running", func() error { _, err := service.ListExplorations(context.Background(), "running"); return err }},
		{"view-status", func() error { _, _, err := service.StatusView(context.Background(), nil, false); return err }},
		{"view-trace", func() error { _, _, err := service.TraceView(context.Background(), id, false); return err }},
		{"view-visual", func() error { _, err := service.VisualView(context.Background(), false, ""); return err }},
		{"workspace-reset", func() error { _, err := service.Reset(context.Background()); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); errorCode(err) != "WORKSPACE_NOT_INITIALIZED" {
				t.Fatalf("command returned %q, want WORKSPACE_NOT_INITIALIZED: %v", errorCode(err), err)
			}
			if service.Store.Initialized() {
				t.Fatal("rejected command initialized the Workspace")
			}
		})
	}
}

func TestNoOpenTaskRejectsGraphLifecycleCommands(t *testing.T) {
	service, _ := newTestService(t)
	if _, err := service.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	id := "exp_ffffffff-ffff-4fff-8fff-ffffffffffff"
	tests := []struct {
		name string
		call func() error
	}{
		{"cognition-add", func() error {
			_, err := service.AddCognition(context.Background(), AddCognitionRequest{Title: "Hint", Content: "Content", Origin: "user"})
			return err
		}},
		{"exploration-add", func() error {
			_, err := service.AddExploration(context.Background(), AddExplorationRequest{Title: "Work", Content: "Content"})
			return err
		}},
		{"exploration-retry", func() error { _, err := service.RetryExploration(context.Background(), id); return err }},
		{"exploration-assign", func() error {
			_, err := service.AssignExploration(context.Background(), id, "executor", "output://one")
			return err
		}},
		{"exploration-finish", func() error {
			_, err := service.FinishExploration(context.Background(), id, "cancelled", "No Task")
			return err
		}},
		{"exploration-todo", func() error { _, err := service.ListExplorations(context.Background(), "todo"); return err }},
		{"exploration-running", func() error { _, err := service.ListExplorations(context.Background(), "running"); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); errorCode(err) != "NO_OPEN_TASK" {
				t.Fatalf("command returned %q, want NO_OPEN_TASK: %v", errorCode(err), err)
			}
		})
	}
}

func TestIdempotentInitValidatesExistingWorkspaceBeforeSuccess(t *testing.T) {
	tests := []struct {
		name      string
		statement string
		wantCode  string
	}{
		{"schema", `PRAGMA user_version=999`, "UNSUPPORTED_SCHEMA_VERSION"},
		{"protocol", `UPDATE workspace_meta SET protocol_version=999`, "UNSUPPORTED_PROTOCOL_VERSION"},
		{"state", `UPDATE workspace_meta SET runtime_occupied=1`, "WORKSPACE_STATE_UNTRUSTED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, _ := newTestService(t)
			if _, err := service.Init(context.Background()); err != nil {
				t.Fatal(err)
			}
			execDatabase(t, service.Paths.Database, test.statement)
			if _, err := service.Init(context.Background()); errorCode(err) != test.wantCode {
				t.Fatalf("init returned %q, want %q: %v", errorCode(err), test.wantCode, err)
			}
		})
	}
}

func TestTaskCloseMissingReasonDoesNotModifyState(t *testing.T) {
	service, _ := newTestService(t)
	info := "Subject"
	opened, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Remain open", Concurrency: 1, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	taskID := resultID(t, opened)
	if _, err := service.CloseTask(context.Background(), "failed", "  "); errorCode(err) != "INVALID_ARGUMENT" {
		t.Fatalf("close returned %v", err)
	}
	db, err := sql.Open("sqlite", service.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var status string
	var outcome, reason, closedAt sql.NullString
	var runtime int
	if err := db.QueryRow(`SELECT status,outcome,outcome_reason,closed_at FROM tasks WHERE id=?`, taskID).Scan(&status, &outcome, &reason, &closedAt); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT runtime_occupied FROM workspace_meta`).Scan(&runtime); err != nil {
		t.Fatal(err)
	}
	if status != "open" || outcome.Valid || reason.Valid || closedAt.Valid || runtime != 1 {
		t.Fatalf("invalid close modified state: status=%s outcome=%v reason=%v closed=%v runtime=%d", status, outcome, reason, closedAt, runtime)
	}
}

func TestResetArchivesViewsAndExtraFilesAndRequiresNewSubjectInfo(t *testing.T) {
	service, _ := newTestService(t)
	createClosedTask(t, service)
	view, viewPath, err := service.StatusView(context.Background(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	extraPath := filepath.Join(service.Paths.Norns, "plugin-state")
	if err := os.WriteFile(extraPath, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	reset, err := service.Reset(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	archiveRelative := reset.Data.(map[string]any)["archive_path"].(string)
	archive := filepath.Join(service.Paths.Root, filepath.FromSlash(archiveRelative))
	archivedView, err := os.ReadFile(filepath.Join(archive, "view", filepath.Base(viewPath)))
	if err != nil || string(archivedView) != string(view) {
		t.Fatalf("published View was not archived intact: %q %v", archivedView, err)
	}
	archivedExtra, err := os.ReadFile(filepath.Join(archive, "plugin-state"))
	if err != nil || string(archivedExtra) != "preserve" {
		t.Fatalf("extra Workspace file was not archived intact: %q %v", archivedExtra, err)
	}
	if _, err := os.Stat(filepath.Join(service.Paths.Norns, "plugin-state")); !os.IsNotExist(err) {
		t.Fatalf("new Workspace retained an old extra file: %v", err)
	}
	if entries, err := os.ReadDir(service.Paths.View); err == nil && len(entries) != 0 {
		t.Fatalf("new Workspace retained archived Views: %v", entries)
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "New Subject", Concurrency: 1}); errorCode(err) != "INVALID_ARGUMENT" {
		t.Fatalf("first Task after reset without info returned %v", err)
	}
	viewAfter, _, err := service.StatusView(context.Background(), nil, true)
	if err != nil {
		t.Fatal(err)
	}
	var taskCount int
	db, err := sql.Open("sqlite", service.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`SELECT count(*) FROM tasks`).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 0 || len(viewAfter) == 0 {
		t.Fatalf("failed first Task left state after reset: tasks=%d", taskCount)
	}
}
