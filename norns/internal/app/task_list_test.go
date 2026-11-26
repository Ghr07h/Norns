package app

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestListTasksFiltersHistoryAndReflectsReopenWithoutPublishingViews(t *testing.T) {
	ctx := context.Background()
	s, clock := newTestService(t)
	if _, err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	list := func(status string) TaskList {
		t.Helper()
		result, err := s.ListTasks(ctx, status)
		if err != nil {
			t.Fatal(err)
		}
		data := result.Data.(TaskList)
		if data.Tasks == nil {
			t.Fatal("empty Task lists must be arrays")
		}
		return data
	}
	if data := list(""); len(data.Tasks) != 0 {
		t.Fatalf("new Workspace has Tasks: %#v", data)
	}
	info := "Subject"
	firstResult, err := s.OpenTask(ctx, OpenTaskRequest{Purpose: "First", Concurrency: 2, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	first := resultID(t, firstResult)
	if _, err := s.CloseTask(ctx, "succeeded", "First done"); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	secondResult, err := s.OpenTask(ctx, OpenTaskRequest{Purpose: "Second", Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	second := resultID(t, secondResult)
	all := list("").Tasks
	if len(all) != 2 || all[0].ID != first || all[1].ID != second {
		t.Fatalf("wrong Task ordering: %#v", all)
	}
	closed := list("closed").Tasks
	if len(closed) != 1 || closed[0].ID != first || closed[0].Status != "closed" || value(closed[0].Outcome) != "succeeded" || value(closed[0].OutcomeReason) != "First done" || closed[0].ClosedAt == nil {
		t.Fatalf("closed Task fields were lost: %#v", closed)
	}
	open := list("open").Tasks
	if len(open) != 1 || open[0].ID != second || open[0].Status != "open" || open[0].Purpose != "Second" || open[0].InitCognitionRef == "" || open[0].MaxConcurrency != 1 || open[0].CreatedAt == "" || open[0].Outcome != nil || open[0].ClosedAt != nil {
		t.Fatalf("open Task fields are incomplete: %#v", open)
	}
	if _, err := s.CloseTask(ctx, "cancelled", "Second stopped"); err != nil {
		t.Fatal(err)
	}
	if len(list("open").Tasks) != 0 {
		t.Fatal("closed Task still listed as open")
	}
	if _, err := s.ReopenTask(ctx, &first); err != nil {
		t.Fatal(err)
	}
	open = list("open").Tasks
	if len(open) != 1 || open[0].ID != first || open[0].Outcome != nil || open[0].OutcomeReason != nil || open[0].ClosedAt != nil {
		t.Fatalf("reopen was not reflected: %#v", open)
	}
	if entries, err := os.ReadDir(s.Paths.View); err != nil && !os.IsNotExist(err) || len(entries) != 0 {
		t.Fatalf("task list published a View: %v %v", entries, err)
	}
}

func TestListTasksRejectsInvalidFiltersBeforeInitializingWorkspace(t *testing.T) {
	s, _ := newTestService(t)
	for _, status := range []string{"running", "all", "OPEN", " ", "open' OR 1=1"} {
		result, err := s.ListTasks(context.Background(), status)
		if errorCode(err) != "INVALID_ARGUMENT" || result.Data != nil || s.Store.Initialized() {
			t.Fatalf("invalid filter %q was accepted: %#v %v", status, result, err)
		}
	}
}
