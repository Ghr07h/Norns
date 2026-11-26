package app

import (
	"context"
	"testing"
	"time"
)

func TestCheckWorkspaceTaskLifecycle(t *testing.T) {
	ctx := context.Background()
	s, clock := newTestService(t)
	if _, err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	check := func(state string, summary WorkspaceSummary) WorkspaceCheck {
		t.Helper()
		result, err := s.CheckWorkspace(ctx)
		if err != nil {
			t.Fatal(err)
		}
		data := result.Data.(WorkspaceCheck)
		if !data.Valid || data.State != state || data.RuntimeOccupied != (state == "open_task") || data.Summary != summary {
			t.Fatalf("unexpected Workspace check: %#v", data)
		}
		return data
	}
	empty := check("empty", WorkspaceSummary{})
	if empty.Task != nil || empty.Tasks == nil || len(*empty.Tasks) != 0 {
		t.Fatalf("empty Workspace must return an empty Task list: %#v", empty)
	}
	info := "Subject"
	opened, err := s.OpenTask(ctx, OpenTaskRequest{Purpose: "First", Concurrency: 2, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	first := resultID(t, opened)
	root := opened.Data.(map[string]any)["init_cognition_ref"].(string)
	addExplorationForTest(t, s, root, "Work")
	open := check("open_task", WorkspaceSummary{TaskCount: 1, CognitionCount: 1, ExplorationCount: 1})
	if open.Tasks != nil || open.Task == nil || open.Task.ID != first || open.Task.Purpose != "First" || open.Task.Status != "open" || open.Task.MaxConcurrency != 2 || open.Task.InitCognitionRef != root || open.Task.CreatedAt == "" || open.Task.ClosedAt != nil {
		t.Fatalf("open Task summary is incomplete: %#v", open)
	}
	if _, err := s.CloseTask(ctx, "succeeded", "First done"); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	secondTask, err := s.OpenTask(ctx, OpenTaskRequest{Purpose: "Second", Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	second := resultID(t, secondTask)
	open = check("open_task", WorkspaceSummary{TaskCount: 2, CognitionCount: 1, ExplorationCount: 1})
	if open.Task == nil || open.Task.ID != second || open.Tasks != nil {
		t.Fatalf("check selected a historical Task: %#v", open)
	}
	if _, err := s.CloseTask(ctx, "cancelled", "Second stopped"); err != nil {
		t.Fatal(err)
	}
	closed := check("no_open_task", WorkspaceSummary{TaskCount: 2, CognitionCount: 1, ExplorationCount: 1})
	if closed.Task != nil || closed.Tasks == nil || len(*closed.Tasks) != 2 {
		t.Fatalf("historical Tasks are missing: %#v", closed)
	}
	tasks := *closed.Tasks
	if tasks[0].ID != first || tasks[1].ID != second || value(tasks[0].Outcome) != "succeeded" || value(tasks[1].Outcome) != "cancelled" || value(tasks[0].OutcomeReason) != "First done" || value(tasks[1].OutcomeReason) != "Second stopped" {
		t.Fatalf("Task history is incomplete or unordered: %#v", tasks)
	}
	for _, task := range tasks {
		if task.Status != "closed" || task.CreatedAt == "" || task.ClosedAt == nil {
			t.Fatalf("Task lifecycle information is missing: %#v", task)
		}
	}
	if _, err := s.ReopenTask(ctx, &first); err != nil {
		t.Fatal(err)
	}
	open = check("open_task", closed.Summary)
	if open.Task == nil || open.Task.ID != first || open.Task.Outcome != nil || open.Task.ClosedAt != nil {
		t.Fatalf("check did not reflect reopen: %#v", open)
	}
}

func TestCheckWorkspaceRejectsInvalidWorkspace(t *testing.T) {
	for _, tc := range []struct {
		name, statement, code string
	}{
		{"schema", `PRAGMA user_version=999`, "UNSUPPORTED_SCHEMA_VERSION"},
		{"protocol", `UPDATE workspace_meta SET protocol_version=999`, "UNSUPPORTED_PROTOCOL_VERSION"},
		{"state", `UPDATE workspace_meta SET runtime_occupied=1`, "WORKSPACE_STATE_UNTRUSTED"},
		{"application", `PRAGMA application_id=0`, "INVALID_DATABASE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newTestService(t)
			if _, err := s.Init(context.Background()); err != nil {
				t.Fatal(err)
			}
			execDatabase(t, s.Paths.Database, tc.statement)
			result, err := s.CheckWorkspace(context.Background())
			if errorCode(err) != tc.code || result.Data != nil {
				t.Fatalf("invalid Workspace reported as valid: result=%#v error=%v", result, err)
			}
			result, err = s.FreeWorkspace(context.Background())
			if errorCode(err) != tc.code || result.Data != nil {
				t.Fatalf("free bypassed invalid Workspace validation: result=%#v error=%v", result, err)
			}
			result, err = s.ListTasks(context.Background(), "open")
			if errorCode(err) != tc.code || result.Data != nil {
				t.Fatalf("list bypassed invalid Workspace validation: result=%#v error=%v", result, err)
			}
		})
	}
}
