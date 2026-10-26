package app

import (
	"context"
	"encoding/json"
	"testing"
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
	if len(firstStatus.Todo) != 0 || len(firstStatus.Running) != 0 || !containsGlobal(firstStatus.GlobalCognitions, globalID) {
		t.Fatalf("historical Status context is wrong: %#v", firstStatus)
	}

	secondStatus := decodeStatus(t, service, &secondTask)
	if cognitionRelevant(secondStatus.Graph.Cognitions, crossHintID) != true || cognitionHasRelevantField(secondStatus.Graph.Cognitions, relevantID) || len(secondStatus.Todo) != 1 || secondStatus.Todo[0].ID != secondExp {
		t.Fatalf("Second Task Status context is wrong: %#v", secondStatus)
	}

	traceBytes, _, err := service.TraceView(ctx, relatedExp, true)
	if err != nil {
		t.Fatal(err)
	}
	var trace traceView
	if err := json.Unmarshal(traceBytes, &trace); err != nil {
		t.Fatal(err)
	}
	if trace.OriginTask.Purpose != "First" || !containsCognition(trace.Related.Cognitions, relevantID) || !containsCognition(trace.Related.Cognitions, crossHintID) || !containsGlobal(trace.GlobalCognitions, globalID) {
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
	if allStatus.Tasks == nil || len(*allStatus.Tasks) != 2 || !cognitionRelevant(allStatus.Graph.Cognitions, relevantID) || !cognitionRelevant(allStatus.Graph.Cognitions, crossHintID) {
		t.Fatalf("all-Task Status relevance is wrong: %#v", allStatus)
	}
}

func decodeStatus(t *testing.T, service *Service, taskID *string) statusView {
	t.Helper()
	payload, _, err := service.StatusView(context.Background(), taskID, true)
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
