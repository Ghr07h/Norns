package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestStatusPurposeScopeAndTraceMinimalSubgraph(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	info := "Subject"
	first, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "First", Concurrency: 2, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	firstTask := resultID(t, first)
	origin := first.Data.(map[string]any)["init_cognition_ref"].(string)
	relatedExp := addExplorationForTest(t, service, origin, "Related")
	unrelatedExp := addExplorationForTest(t, service, origin, "Unrelated")
	if _, err := service.AssignExploration(ctx, relatedExp, "related", "output://related"); err != nil {
		t.Fatal(err)
	}
	relevant, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Relevant", Content: "Supports First", Origin: "exploration", ExplorationID: &relatedExp, Relevant: true})
	if err != nil {
		t.Fatal(err)
	}
	relevantID := resultID(t, relevant)
	if _, err := service.FinishExploration(ctx, relatedExp, "succeeded", "Related done"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AssignExploration(ctx, unrelatedExp, "unrelated", "output://unrelated"); err != nil {
		t.Fatal(err)
	}
	unrelated, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Unrelated", Content: "Other branch", Origin: "exploration", ExplorationID: &unrelatedExp})
	if err != nil {
		t.Fatal(err)
	}
	unrelatedID := resultID(t, unrelated)
	if _, err := service.FinishExploration(ctx, unrelatedExp, "succeeded", "Unrelated done"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CloseTask(ctx, "succeeded", "First done"); err != nil {
		t.Fatal(err)
	}

	second, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Second", Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	secondTask := resultID(t, second)
	global, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Global", Content: "Added later", Origin: "user"})
	if err != nil {
		t.Fatal(err)
	}
	globalID := resultID(t, global)
	crossHint, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Hint", Content: "Second Task hint", Origin: "user", ExplorationID: &relatedExp, Relevant: true})
	if err != nil {
		t.Fatal(err)
	}
	crossHintID := resultID(t, crossHint)
	secondExp := addExplorationForTest(t, service, relevantID, "Second work")

	firstStatus := decodeStatus(t, service, &firstTask)
	if cognitionRelevant(firstStatus.Graph.Cognitions, relevantID) != true || cognitionHasRelevantField(firstStatus.Graph.Cognitions, crossHintID) {
		t.Fatalf("First Task relevance scope is wrong: %#v", firstStatus.Graph.Cognitions)
	}
	if len(firstStatus.Todo) != 0 || len(firstStatus.Running) != 0 || !containsGlobal(firstStatus.GlobalHints, globalID) {
		t.Fatalf("historical Status context is wrong: %#v", firstStatus)
	}

	secondStatus := decodeStatus(t, service, &secondTask)
	if !containsHint(secondStatus.Graph.Explorations, relatedExp, crossHintID) || containsCognition(secondStatus.Graph.Cognitions, crossHintID) || cognitionHasRelevantField(secondStatus.Graph.Cognitions, relevantID) || len(secondStatus.Todo) != 1 || secondStatus.Todo[0].ID != secondExp {
		t.Fatalf("Second Task Status context is wrong: %#v", secondStatus)
	}

	traceBytes, _, err := service.TraceView(ctx, relatedExp, ViewJSON)
	if err != nil {
		t.Fatal(err)
	}
	var trace traceView
	if err := json.Unmarshal(traceBytes, &trace); err != nil {
		t.Fatal(err)
	}
	if trace.ParentTask.Purpose != "First" || !containsCognition(trace.Related.Cognitions, relevantID) || !containsHint(trace.Related.Explorations, relatedExp, crossHintID) || !containsGlobal(trace.GlobalHints, globalID) {
		t.Fatalf("Trace omitted required context: %#v", trace)
	}
	if containsCognition(trace.Related.Cognitions, unrelatedID) || containsExploration(trace.Related.Explorations, unrelatedExp) {
		t.Fatalf("Trace included an unrelated branch: %#v", trace.Related)
	}
	for _, cognition := range trace.Related.Cognitions {
		if cognition.TaskPurposeRelevant != nil {
			t.Fatalf("Trace exposed task_purpose_relevant: %#v", cognition)
		}
	}

	if _, err := service.CloseTask(ctx, "cancelled", "Done"); err != nil {
		t.Fatal(err)
	}
	allStatus := decodeStatus(t, service, nil)
	if allStatus.Tasks == nil || len(*allStatus.Tasks) != 2 || !cognitionRelevant(allStatus.Graph.Cognitions, relevantID) || !containsHint(allStatus.Graph.Explorations, relatedExp, crossHintID) {
		t.Fatalf("all-Task Status relevance is wrong: %#v", allStatus)
	}
}

func TestTraceRunningExplorationsUseParentTaskContext(t *testing.T) {
	ctx := context.Background()
	service, clock := newTestService(t)
	info := "Subject"
	opened, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Parent", Concurrency: 2, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	root := opened.Data.(map[string]any)["init_cognition_ref"].(string)
	target := addExplorationForTest(t, service, root, "Target")
	clock.Advance(time.Second)
	parallel := addExplorationForTest(t, service, root, "Parallel")
	addExplorationForTest(t, service, root, "Todo")
	assigned, err := service.AssignExploration(ctx, target, "target-executor", "output://target")
	if err != nil {
		t.Fatal(err)
	}
	if assigned.Data.(map[string]any)["explorer_id"] != "target-executor" || assigned.Data.(map[string]any)["executor_id"] != nil {
		t.Fatalf("assign returned the wrong Explorer field: %#v", assigned.Data)
	}
	clock.Advance(2 * time.Second)
	if _, err := service.AssignExploration(ctx, parallel, "parallel-executor", "output://parallel"); err != nil {
		t.Fatal(err)
	}
	clock.Advance(3 * time.Second)
	trace := readTrace(t, service, target)
	if len(trace.Running) != 2 || trace.Running[0].ID != target || trace.Running[0].Title != "Target" || trace.Running[0].DurationSeconds != 5 || trace.Running[1].ID != parallel || trace.Running[1].Title != "Parallel" || trace.Running[1].DurationSeconds != 3 {
		t.Fatalf("Trace running summary is incomplete or unordered: %#v", trace.Running)
	}
	if len(trace.Related.Explorations) != 1 || trace.Related.Explorations[0].ID != target {
		t.Fatal("running summary expanded the minimal subgraph")
	}
	status := decodeStatus(t, service, nil)
	if trace.GeneratedAt != status.GeneratedAt || trace.Running[0].runningProjection != status.Running[0] || trace.Running[1].runningProjection != status.Running[1] {
		t.Fatal("Trace and Status computed different running summaries")
	}
	if trace.Running[0].ExplorerID != "target-executor" || trace.Running[1].ExplorerID != "parallel-executor" {
		t.Fatalf("Trace lost the Explorer IDs saved at assign: %#v", trace.Running)
	}
	query, err := service.ListExplorations(ctx, "running")
	if err != nil {
		t.Fatal(err)
	}
	if len(query.Explorations) != 2 || query.Explorations[0]["explorer_id"] != "target-executor" || query.Explorations[1]["explorer_id"] != "parallel-executor" {
		t.Fatalf("running query lost the Explorer IDs: %#v", query)
	}
	for i, item := range query.Explorations {
		if _, exists := item["output_ref"]; exists {
			t.Fatalf("running query exposed the output reference: %#v", item)
		}
		if item["assigned_at"] == "" || item["assigned_at"] == nil || item["duration_seconds"] != trace.Running[i].DurationSeconds {
			t.Fatalf("running query lost assignment timing: %#v", item)
		}
	}
	for _, format := range []ViewFormat{ViewYAML, ViewJSON} {
		payload, _, err := service.TraceView(ctx, target, format)
		if err != nil || strings.Contains(string(payload), "executor_id") || !strings.Contains(string(payload), "explorer_id") {
			t.Fatalf("Trace did not use explorer_id: %v\n%s", err, payload)
		}
	}
	clock.Advance(-10 * time.Second)
	for _, item := range readTrace(t, service, target).Running {
		if item.DurationSeconds != 0 {
			t.Fatal("running duration must not be negative")
		}
	}
	clock.Advance(10 * time.Second)
	if _, err := service.CloseTask(ctx, "cancelled", "End parent Task"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Other Task", Concurrency: 1}); err != nil {
		t.Fatal(err)
	}
	other := addExplorationForTest(t, service, root, "Other running work")
	if _, err := service.AssignExploration(ctx, other, "other-executor", "output://other"); err != nil {
		t.Fatal(err)
	}
	historical := readTrace(t, service, target)
	if historical.Running == nil || len(historical.Running) != 0 || historical.ParentTask.Purpose != "Parent" {
		t.Fatalf("historical Trace used the open Task: %#v", historical)
	}
	yaml, _, err := service.TraceView(ctx, target, ViewYAML)
	if err != nil || !strings.Contains(string(yaml), "running_explorations: []") {
		t.Fatalf("empty running summary must be an explicit list: %v\n%s", err, yaml)
	}
}

func decodeStatus(t *testing.T, service *Service, taskID *string) statusView {
	t.Helper()
	payload, _, err := service.StatusView(context.Background(), taskID, ViewJSON)
	if err != nil {
		t.Fatal(err)
	}
	var view statusView
	if err := json.Unmarshal(payload, &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func cognitionHasRelevantField(items []cognitionProjection, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return item.TaskPurposeRelevant != nil
		}
	}
	return false
}

func cognitionRelevant(items []cognitionProjection, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return item.TaskPurposeRelevant != nil && *item.TaskPurposeRelevant
		}
	}
	return false
}

func containsHint(exps []explorationProjection, eid, cid string) bool {
	for _, e := range exps {
		if e.ID == eid {
			for _, c := range e.Hints {
				if c.ID == cid {
					return true
				}
			}
		}
	}
	return false
}
