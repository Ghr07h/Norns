package app

import (
	"context"
	"database/sql"
	"testing"
)

func TestCognitionOriginMatrixAndExplorationHintStates(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	firstInfo := "First subject"
	first, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "First", Concurrency: 1, Info: &firstInfo})
	if err != nil {
		t.Fatal(err)
	}
	firstTask := resultID(t, first)
	firstOrigin := first.Data.(map[string]any)["init_cognition_ref"].(string)
	if _, err := service.CloseTask(ctx, "succeeded", "First complete"); err != nil {
		t.Fatal(err)
	}
	secondInfo := "Second subject"
	second, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Second", Concurrency: 1, Info: &secondInfo})
	if err != nil {
		t.Fatal(err)
	}
	secondTask := resultID(t, second)
	secondOrigin := second.Data.(map[string]any)["init_cognition_ref"].(string)

	global, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Global", Content: "Workspace context", Origin: "user"})
	if err != nil {
		t.Fatal(err)
	}
	globalID := resultID(t, global)
	explorationID := addExplorationForTest(t, service, secondOrigin, "States")
	todoHint, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Todo hint", Content: "Before assign", Origin: "user", ExplorationID: &explorationID})
	if err != nil {
		t.Fatal(err)
	}
	todoOutput, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Todo output", Content: "Produced before assign", Origin: "exploration", ExplorationID: &explorationID, Relevant: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AssignExploration(ctx, explorationID, "executor", "output://states"); err != nil {
		t.Fatal(err)
	}
	runningHint, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Running hint", Content: "During execution", Origin: "user", ExplorationID: &explorationID, Relevant: true})
	if err != nil {
		t.Fatal(err)
	}
	runningOutput, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Running output", Content: "During execution", Origin: "exploration", ExplorationID: &explorationID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.FinishExploration(ctx, explorationID, "succeeded", "Finished"); err != nil {
		t.Fatal(err)
	}
	finishedHint, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Finished hint", Content: "After finish", Origin: "user", ExplorationID: &explorationID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Late output", Content: "Rejected", Origin: "exploration", ExplorationID: &explorationID}); errorCode(err) != "INVALID_STATE" {
		t.Fatalf("finished Exploration accepted Explorer Cognition: %v", err)
	}
	if _, err := service.CloseTask(ctx, "succeeded", "Second complete"); err != nil {
		t.Fatal(err)
	}
	third, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Third", Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	thirdTask := resultID(t, third)
	historicalHint, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Historical hint", Content: "Cross Task", Origin: "user", ExplorationID: &explorationID, Relevant: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Historical output", Content: "Rejected", Origin: "exploration", ExplorationID: &explorationID}); errorCode(err) != "INVALID_STATE" {
		t.Fatalf("historical Exploration accepted Explorer Cognition: %v", err)
	}

	db, err := sql.Open("sqlite", service.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	assertCognitionRow(t, db, firstOrigin, cognitionRow{taskID: &firstTask, origin: "task", relevant: false})
	assertCognitionRow(t, db, secondOrigin, cognitionRow{taskID: &secondTask, origin: "task", originCognition: &firstOrigin, relevant: false})
	assertCognitionRow(t, db, globalID, cognitionRow{origin: "user", originCognition: &secondOrigin, relevant: false})
	assertCognitionRow(t, db, resultID(t, todoHint), cognitionRow{taskID: &secondTask, origin: "user", originExploration: &explorationID, relevant: false})
	assertCognitionRow(t, db, resultID(t, todoOutput), cognitionRow{taskID: &secondTask, origin: "exploration", originExploration: &explorationID, relevant: true})
	assertCognitionRow(t, db, resultID(t, runningHint), cognitionRow{taskID: &secondTask, origin: "user", originExploration: &explorationID, relevant: true})
	assertCognitionRow(t, db, resultID(t, runningOutput), cognitionRow{taskID: &secondTask, origin: "exploration", originExploration: &explorationID, relevant: false})
	assertCognitionRow(t, db, resultID(t, finishedHint), cognitionRow{taskID: &secondTask, origin: "user", originExploration: &explorationID, relevant: false})
	assertCognitionRow(t, db, resultID(t, historicalHint), cognitionRow{taskID: &thirdTask, origin: "user", originExploration: &explorationID, relevant: true})
}

func TestInvalidCognitionArgumentsNeverInsertRows(t *testing.T) {
	service, _ := newTestService(t)
	info := "Subject"
	opened, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Validate Cognition", Concurrency: 1, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	origin := opened.Data.(map[string]any)["init_cognition_ref"].(string)
	explorationID := addExplorationForTest(t, service, origin, "Target")
	wongTypeID := "task_ffffffff-ffff-4fff-8fff-ffffffffffff"
	tests := []struct {
		name string
		req  AddCognitionRequest
	}{
		{"unknown-origin", AddCognitionRequest{Title: "Bad", Content: "Bad", Origin: "task"}},
		{"exploration-without-reference", AddCognitionRequest{Title: "Bad", Content: "Bad", Origin: "exploration"}},
		{"global-relevant", AddCognitionRequest{Title: "Bad", Content: "Bad", Origin: "user", Relevant: true}},
		{"wrong-reference-type", AddCognitionRequest{Title: "Bad", Content: "Bad", Origin: "user", ExplorationID: &wongTypeID}},
	}
	db, err := sql.Open("sqlite", service.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var before int
	if err := db.QueryRow(`SELECT count(*) FROM cognitions`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.AddCognition(context.Background(), test.req); errorCode(err) != "INVALID_ARGUMENT" {
				t.Fatalf("invalid Cognition returned %v", err)
			}
			var after int
			if err := db.QueryRow(`SELECT count(*) FROM cognitions`).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatalf("invalid Cognition inserted a row: before=%d after=%d", before, after)
			}
		})
	}
	_ = explorationID
}

type cognitionRow struct {
	taskID            *string
	origin            string
	originCognition   *string
	originExploration *string
	relevant          bool
}

func assertCognitionRow(t *testing.T, db *sql.DB, id string, want cognitionRow) {
	t.Helper()
	var taskID, originCognition, originExploration sql.NullString
	var origin string
	var relevant bool
	if err := db.QueryRow(`SELECT task_id,origin,origin_cognition_id,origin_exploration_id,task_purpose_relevant FROM cognitions WHERE id=?`, id).Scan(&taskID, &origin, &originCognition, &originExploration, &relevant); err != nil {
		t.Fatal(err)
	}
	if nullableString(taskID) != stringValue(want.taskID) || origin != want.origin || nullableString(originCognition) != stringValue(want.originCognition) || nullableString(originExploration) != stringValue(want.originExploration) || relevant != want.relevant {
		t.Fatalf("Cognition %s mismatch: task=%q origin=%q origin_cognition=%q origin_exploration=%q relevant=%v want=%#v", id, nullableString(taskID), origin, nullableString(originCognition), nullableString(originExploration), relevant, want)
	}
}

func nullableString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
