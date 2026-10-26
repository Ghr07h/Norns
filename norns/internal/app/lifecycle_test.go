package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

func TestFirstTaskValidationLeavesWorkspaceUninitialized(t *testing.T) {
	ctx := context.Background()
	for _, request := range []OpenTaskRequest{
		{Purpose: "Missing info", Concurrency: 1},
		{Purpose: "   ", Concurrency: 1, Info: stringPointer("Subject")},
		{Purpose: "Bad concurrency", Concurrency: 0, Info: stringPointer("Subject")},
		{Purpose: "Empty info", Concurrency: 1, Info: stringPointer("  ")},
	} {
		service, _ := newTestService(t)
		if _, err := service.OpenTask(ctx, request); errorCode(err) != "INVALID_ARGUMENT" {
			t.Fatalf("request %#v returned %v", request, err)
		}
		if service.Store.Initialized() {
			t.Fatalf("invalid first Task left an initialized Workspace: %#v", request)
		}
	}
}

func TestCloseCancelsPendingExplorationsAndReopenPreservesThem(t *testing.T) {
	ctx := context.Background()
	service, clock := newTestService(t)
	info := "Subject"
	opened, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Lifecycle", Concurrency: 3, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	origin := opened.Data.(map[string]any)["init_cognition_ref"].(string)

	finishedID := addExplorationForTest(t, service, origin, "Finished")
	if _, err := service.AssignExploration(ctx, finishedID, "finished-executor", "output://finished"); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	if _, err := service.FinishExploration(ctx, finishedID, "succeeded", "Finished before close"); err != nil {
		t.Fatal(err)
	}

	todoID := addExplorationForTest(t, service, origin, "Todo")
	if _, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Early", Content: "Produced before assign", Origin: "exploration", ExplorationID: &todoID}); err != nil {
		t.Fatal(err)
	}
	runningID := addExplorationForTest(t, service, origin, "Running")
	if _, err := service.AssignExploration(ctx, runningID, "running-executor", "output://running"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Output", Content: "Keep this", Origin: "exploration", ExplorationID: &runningID}); err != nil {
		t.Fatal(err)
	}
	clock.Advance(5 * time.Second)
	closed, err := service.CloseTask(ctx, "failed", "Runtime stopped")
	if err != nil {
		t.Fatal(err)
	}
	closedAt := closed.Data.(map[string]any)["closed_at"].(string)
	if closed.Data.(map[string]any)["cancelled_todo"] != int64(1) || closed.Data.(map[string]any)["cancelled_running"] != int64(1) {
		t.Fatalf("unexpected close counts: %#v", closed.Data)
	}

	db, err := sql.Open("sqlite", service.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	assertExplorationState(t, db, finishedID, "finished", "succeeded", true, "")
	assertExplorationState(t, db, todoID, "finished", "cancelled", false, closedAt)
	assertExplorationState(t, db, runningID, "finished", "cancelled", true, closedAt)

	if _, err := service.ReopenTask(ctx, nil); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{finishedID, todoID, runningID} {
		var status string
		if err := db.QueryRow(`SELECT status FROM explorations WHERE id=?`, id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "finished" {
			t.Fatalf("reopen changed Exploration %s to %s", id, status)
		}
	}
	var taskStatus string
	var outcome, reason, taskClosed sql.NullString
	if err := db.QueryRow(`SELECT status,outcome,outcome_reason,closed_at FROM tasks`).Scan(&taskStatus, &outcome, &reason, &taskClosed); err != nil {
		t.Fatal(err)
	}
	if taskStatus != "open" || outcome.Valid || reason.Valid || taskClosed.Valid {
		t.Fatalf("reopen did not clear Task completion fields")
	}
}

func TestCloseRollsBackEveryStateChangeOnBatchFailure(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	info := "Subject"
	opened, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Atomic close", Concurrency: 2, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	origin := opened.Data.(map[string]any)["init_cognition_ref"].(string)
	todoID := addExplorationForTest(t, service, origin, "Todo")
	runningID := addExplorationForTest(t, service, origin, "Running")
	if _, err := service.AssignExploration(ctx, runningID, "executor", "output://running"); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", service.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER inject_close_failure BEFORE UPDATE ON explorations
		WHEN OLD.status='running' AND NEW.status='finished'
		BEGIN SELECT RAISE(ABORT, 'injected close failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CloseTask(ctx, "failed", "Should roll back"); err == nil {
		t.Fatal("close unexpectedly succeeded")
	}
	var taskStatus string
	var runtime int
	if err := db.QueryRow(`SELECT status FROM tasks`).Scan(&taskStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT runtime_occupied FROM workspace_meta`).Scan(&runtime); err != nil {
		t.Fatal(err)
	}
	var todoStatus, runningStatus string
	if err := db.QueryRow(`SELECT status FROM explorations WHERE id=?`, todoID).Scan(&todoStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT status FROM explorations WHERE id=?`, runningID).Scan(&runningStatus); err != nil {
		t.Fatal(err)
	}
	if taskStatus != "open" || runtime != 1 || todoStatus != "todo" || runningStatus != "running" {
		t.Fatalf("close partially committed: task=%s runtime=%d todo=%s running=%s", taskStatus, runtime, todoStatus, runningStatus)
	}
}

func TestReopenDefaultSelectionIdempotencyAndConflict(t *testing.T) {
	ctx := context.Background()
	service, clock := newTestService(t)
	firstInfo := "First subject"
	first, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "First", Concurrency: 1, Info: &firstInfo})
	if err != nil {
		t.Fatal(err)
	}
	firstID := resultID(t, first)
	if _, err := service.CloseTask(ctx, "succeeded", "First done"); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	secondInfo := "Second subject"
	second, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Second", Concurrency: 1, Info: &secondInfo})
	if err != nil {
		t.Fatal(err)
	}
	secondID := resultID(t, second)
	if _, err := service.CloseTask(ctx, "failed", "Second done"); err != nil {
		t.Fatal(err)
	}

	reopened, err := service.ReopenTask(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resultID(t, reopened) != secondID || reopened.Data.(map[string]any)["changed"] != true {
		t.Fatalf("default reopen did not select latest Task: %#v", reopened.Data)
	}
	idempotent, err := service.ReopenTask(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resultID(t, idempotent) != secondID || idempotent.Data.(map[string]any)["changed"] != false {
		t.Fatalf("open Task reopen was not idempotent: %#v", idempotent.Data)
	}
	if _, err := service.ReopenTask(ctx, &firstID); errorCode(err) != "OPEN_TASK_EXISTS" {
		t.Fatalf("reopening a closed Task beside an open Task returned %v", err)
	}
}

func TestCrossTaskHintsTraceAndRetry(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	info := "Subject"
	first, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "First", Concurrency: 1, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	origin := first.Data.(map[string]any)["init_cognition_ref"].(string)
	explorationID := addExplorationForTest(t, service, origin, "Historical")
	if _, err := service.AssignExploration(ctx, explorationID, "executor", "output://historical"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.FinishExploration(ctx, explorationID, "inconclusive", "Need another Task"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Finished hint", Content: "Allowed", Origin: "user", ExplorationID: &explorationID}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Late output", Content: "Rejected", Origin: "exploration", ExplorationID: &explorationID}); errorCode(err) != "INVALID_STATE" {
		t.Fatalf("finished Explorer Cognition returned %v", err)
	}
	if _, err := service.CloseTask(ctx, "unachievable", "Continue separately"); err != nil {
		t.Fatal(err)
	}
	second, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Second", Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	secondID := resultID(t, second)
	global, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Global later", Content: "Visible everywhere", Origin: "user"})
	if err != nil {
		t.Fatal(err)
	}
	hint, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Cross Task", Content: "Current Task hint", Origin: "user", ExplorationID: &explorationID, Relevant: true})
	if err != nil {
		t.Fatal(err)
	}
	if hint.Data.(map[string]any)["task_id"] != secondID {
		t.Fatalf("cross-Task hint has wrong task_id: %#v", hint.Data)
	}
	retry, err := service.RetryExploration(ctx, explorationID)
	if err != nil {
		t.Fatal(err)
	}
	retryID := resultID(t, retry)

	traceBytes, _, err := service.TraceView(ctx, explorationID, true)
	if err != nil {
		t.Fatal(err)
	}
	var trace traceView
	if err := json.Unmarshal(traceBytes, &trace); err != nil {
		t.Fatal(err)
	}
	if trace.OriginTask.Purpose != "First" || !containsCognition(trace.Related.Cognitions, resultID(t, hint)) || !containsGlobal(trace.GlobalCognitions, resultID(t, global)) {
		t.Fatalf("historical Trace omitted later context: %#v", trace)
	}

	db, err := sql.Open("sqlite", service.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var retryTask, status string
	if err := db.QueryRow(`SELECT task_id,status FROM explorations WHERE id=?`, retryID).Scan(&retryTask, &status); err != nil {
		t.Fatal(err)
	}
	if retryTask != secondID || status != "todo" {
		t.Fatalf("cross-Task retry is invalid: task=%s status=%s", retryTask, status)
	}
}

func TestExplorationRejectsInvalidTransitionsAndSources(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	info := "Subject"
	opened, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Validate", Concurrency: 1, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	origin := opened.Data.(map[string]any)["init_cognition_ref"].(string)
	if _, err := service.AddExploration(ctx, AddExplorationRequest{Title: "Duplicate", Content: "Bad", Sources: []string{origin, origin}}); errorCode(err) != "INVALID_ARGUMENT" {
		t.Fatalf("duplicate sources returned %v", err)
	}
	unknown := "cog_ffffffff-ffff-4fff-8fff-ffffffffffff"
	if _, err := service.AddExploration(ctx, AddExplorationRequest{Title: "Unknown", Content: "Bad", Sources: []string{unknown}}); errorCode(err) != "COGNITION_NOT_FOUND" {
		t.Fatalf("unknown source returned %v", err)
	}
	id := addExplorationForTest(t, service, origin, "Todo")
	if _, err := service.RetryExploration(ctx, id); errorCode(err) != "INVALID_STATE" {
		t.Fatalf("retry todo returned %v", err)
	}
	if _, err := service.FinishExploration(ctx, id, "succeeded", "Not allowed"); errorCode(err) != "INVALID_STATE" {
		t.Fatalf("todo succeeded finish returned %v", err)
	}
	if _, err := service.AssignExploration(ctx, id, " ", "output://x"); errorCode(err) != "INVALID_ARGUMENT" {
		t.Fatalf("empty executor returned %v", err)
	}
	if _, err := service.AssignExploration(ctx, id, "executor", " "); errorCode(err) != "INVALID_ARGUMENT" {
		t.Fatalf("empty output returned %v", err)
	}
	db, err := sql.Open("sqlite", service.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	assertExplorationState(t, db, id, "todo", "", false, "")
	if _, err := service.AssignExploration(ctx, id, "executor", "output://x"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RetryExploration(ctx, id); errorCode(err) != "INVALID_STATE" {
		t.Fatalf("retry running returned %v", err)
	}
	if _, err := service.FinishExploration(ctx, id, "failed", " "); errorCode(err) != "INVALID_ARGUMENT" {
		t.Fatalf("empty finish reason returned %v", err)
	}
	assertExplorationState(t, db, id, "running", "", true, "")
	if _, err := service.FinishExploration(ctx, id, "cancelled", "Executed cancellation"); err != nil {
		t.Fatal(err)
	}
	todoCancelled := addExplorationForTest(t, service, origin, "Cancel before assign")
	if _, err := service.FinishExploration(ctx, todoCancelled, "cancelled", "No execution"); err != nil {
		t.Fatal(err)
	}
	assertExplorationState(t, db, todoCancelled, "finished", "cancelled", false, "")
	if _, err := service.AssignExploration(ctx, id, "executor", "output://x"); errorCode(err) != "INVALID_STATE" {
		t.Fatalf("assign finished returned %v", err)
	}
	if _, err := service.FinishExploration(ctx, id, "cancelled", "Again"); errorCode(err) != "INVALID_STATE" {
		t.Fatalf("repeat finish returned %v", err)
	}
}

func TestTaskInitSelectionUsesDocumentedTieBreakers(t *testing.T) {
	t.Run("reuse closed Task", func(t *testing.T) {
		service, _ := newTestService(t)
		firstInfo := "First subject"
		first, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "First", Concurrency: 1, Info: &firstInfo})
		if err != nil {
			t.Fatal(err)
		}
		firstOrigin := first.Data.(map[string]any)["init_cognition_ref"].(string)
		if _, err := service.CloseTask(context.Background(), "succeeded", "First done"); err != nil {
			t.Fatal(err)
		}
		secondInfo := "Second subject"
		if _, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Second", Concurrency: 1, Info: &secondInfo}); err != nil {
			t.Fatal(err)
		}
		if _, err := service.CloseTask(context.Background(), "succeeded", "Second done at same time"); err != nil {
			t.Fatal(err)
		}
		third, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Third", Concurrency: 1})
		if err != nil {
			t.Fatal(err)
		}
		if got := third.Data.(map[string]any)["init_cognition_ref"]; got != firstOrigin {
			t.Fatalf("closed_at tie must select id ASC; got %v want %s", got, firstOrigin)
		}
	})

	t.Run("extend latest Task-init", func(t *testing.T) {
		service, _ := newTestService(t)
		firstInfo := "First subject"
		first, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "First", Concurrency: 1, Info: &firstInfo})
		if err != nil {
			t.Fatal(err)
		}
		firstOrigin := first.Data.(map[string]any)["init_cognition_ref"].(string)
		if _, err := service.CloseTask(context.Background(), "succeeded", "First done"); err != nil {
			t.Fatal(err)
		}
		secondInfo := "Second subject"
		if _, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Second", Concurrency: 1, Info: &secondInfo}); err != nil {
			t.Fatal(err)
		}
		if _, err := service.CloseTask(context.Background(), "succeeded", "Second done"); err != nil {
			t.Fatal(err)
		}
		thirdInfo := "Third subject"
		third, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Third", Concurrency: 1, Info: &thirdInfo})
		if err != nil {
			t.Fatal(err)
		}
		thirdOrigin := third.Data.(map[string]any)["init_cognition_ref"].(string)
		db, err := sql.Open("sqlite", service.Paths.Database)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var predecessor string
		if err := db.QueryRow(`SELECT origin_cognition_id FROM cognitions WHERE id=?`, thirdOrigin).Scan(&predecessor); err != nil {
			t.Fatal(err)
		}
		if predecessor != firstOrigin {
			t.Fatalf("created_at tie must select id ASC; got %s want %s", predecessor, firstOrigin)
		}
	})
}

func TestValidationRejectsTamperedTaskInitPredecessor(t *testing.T) {
	ctx := context.Background()
	service, clock := newTestService(t)
	firstInfo := "First"
	first, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "First", Concurrency: 1, Info: &firstInfo})
	if err != nil {
		t.Fatal(err)
	}
	firstOrigin := first.Data.(map[string]any)["init_cognition_ref"].(string)
	if _, err := service.CloseTask(ctx, "succeeded", "Done"); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	secondInfo := "Second"
	second, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Second", Concurrency: 1, Info: &secondInfo})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CloseTask(ctx, "succeeded", "Done"); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	thirdInfo := "Third"
	third, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Third", Concurrency: 1, Info: &thirdInfo})
	if err != nil {
		t.Fatal(err)
	}
	thirdOrigin := third.Data.(map[string]any)["init_cognition_ref"].(string)
	if second.Data.(map[string]any)["init_cognition_ref"] == firstOrigin {
		t.Fatal("test setup did not create a second Task-init Cognition")
	}
	db, err := sql.Open("sqlite", service.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE cognitions SET origin_cognition_id=? WHERE id=?`, firstOrigin, thirdOrigin); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := service.Store.Read(ctx, func(*sql.Tx) error { return nil }); errorCode(err) != "WORKSPACE_STATE_UNTRUSTED" {
		t.Fatalf("tampered predecessor was accepted: %v", err)
	}
}

func addExplorationForTest(t *testing.T, service *Service, source, title string) string {
	t.Helper()
	result, err := service.AddExploration(context.Background(), AddExplorationRequest{Title: title, Content: title + " content", Sources: []string{source}})
	if err != nil {
		t.Fatal(err)
	}
	return resultID(t, result)
}

func assertExplorationState(t *testing.T, db *sql.DB, id, status, outcome string, executed bool, finishedAt string) {
	t.Helper()
	var actualStatus string
	var actualOutcome, executor, output, assigned, actualFinished sql.NullString
	if err := db.QueryRow(`SELECT status,outcome,executor_id,output_ref,assigned_at,finished_at FROM explorations WHERE id=?`, id).Scan(&actualStatus, &actualOutcome, &executor, &output, &assigned, &actualFinished); err != nil {
		t.Fatal(err)
	}
	if actualStatus != status || actualOutcome.String != outcome || executor.Valid != executed || output.Valid != executed || assigned.Valid != executed {
		t.Fatalf("unexpected Exploration state for %s", id)
	}
	if finishedAt != "" && actualFinished.String != finishedAt {
		t.Fatalf("finished_at=%s, want %s", actualFinished.String, finishedAt)
	}
}

func containsGlobal(items []globalProjection, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func stringPointer(value string) *string { return &value }
