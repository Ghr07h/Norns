package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/domain"
	storesqlite "github.com/Ghr07h/norns/norns/internal/store/sqlite"
)

type failingIDs struct {
	base          *testIDs
	failTask      bool
	failCognition bool
}

func (g *failingIDs) TaskID() (string, error) {
	if g.failTask {
		return "", errors.New("injected Task ID failure")
	}
	return g.base.TaskID()
}

func (g *failingIDs) CognitionID() (string, error) {
	if g.failCognition {
		return "", errors.New("injected Cognition ID failure")
	}
	return g.base.CognitionID()
}

func (g *failingIDs) ExplorationID() (string, error) { return g.base.ExplorationID() }
func (g *failingIDs) Token() (string, error)         { return g.base.Token() }

func TestFirstTaskInitializationFailurePublishesNoWorkspace(t *testing.T) {
	for _, test := range []struct {
		name          string
		failTask      bool
		failCognition bool
	}{
		{"task-id", true, false},
		{"cognition-id", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, _ := newTestService(t)
			ids := &failingIDs{base: &testIDs{}, failTask: test.failTask, failCognition: test.failCognition}
			service.IDs = ids
			service.Store = storesqlite.New(service.Paths, ids, service.Clock)
			info := "Subject"
			if _, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Atomic", Concurrency: 1, Info: &info}); errorCode(err) != "INTERNAL_ERROR" {
				t.Fatalf("first Task returned %v", err)
			}
			if service.Store.Initialized() {
				t.Fatal("failed first Task published a Workspace database")
			}
			entries, err := os.ReadDir(service.Paths.Norns)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.Contains(entry.Name(), ".stage.") || strings.HasSuffix(entry.Name(), "-journal") {
					t.Fatalf("failed first Task left staging content: %s", entry.Name())
				}
			}
		})
	}
}

func TestReopenRollsBackTaskWhenRuntimeUpdateFails(t *testing.T) {
	service, _ := newTestService(t)
	info := "Subject"
	opened, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Reopen", Concurrency: 3, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	taskID := resultID(t, opened)
	if _, err := service.CloseTask(context.Background(), "failed", "Original result"); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", service.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER inject_reopen_failure BEFORE UPDATE OF runtime_occupied ON workspace_meta
		WHEN NEW.runtime_occupied=1
		BEGIN SELECT RAISE(ABORT, 'injected reopen failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReopenTask(context.Background(), &taskID); errorCode(err) != "INVALID_STATE" {
		t.Fatalf("reopen returned %v", err)
	}
	var status, outcome, reason, closedAt string
	var concurrency, runtime int
	if err := db.QueryRow(`SELECT status,outcome,outcome_reason,closed_at,max_concurrency FROM tasks WHERE id=?`, taskID).Scan(&status, &outcome, &reason, &closedAt, &concurrency); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT runtime_occupied FROM workspace_meta`).Scan(&runtime); err != nil {
		t.Fatal(err)
	}
	if status != "closed" || outcome != "failed" || reason != "Original result" || closedAt == "" || concurrency != 3 || runtime != 0 {
		t.Fatalf("reopen partially committed: status=%s outcome=%s reason=%s closed=%s concurrency=%d runtime=%d", status, outcome, reason, closedAt, concurrency, runtime)
	}
}

var _ domain.IDGenerator = (*failingIDs)(nil)
