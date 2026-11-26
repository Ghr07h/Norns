package app

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Ghr07h/norns/norns/internal/domain"
)

func TestFreeWorkspaceCancelsUnfinishedWorkAndPreservesHistory(t *testing.T) {
	ctx := context.Background()
	s, taskID, root := protocolWorkspace(t)
	done := addExplorationForTest(t, s, root, "Done")
	if _, err := s.AssignExploration(ctx, done, "done-agent", "output://done"); err != nil {
		t.Fatal(err)
	}
	finding := addFinding(t, s, done)
	if _, err := s.FinishExploration(ctx, done, "succeeded", "Already done"); err != nil {
		t.Fatal(err)
	}
	doneBefore, err := s.GetExploration(ctx, done)
	if err != nil {
		t.Fatal(err)
	}
	cogBefore, err := s.GetCognition(ctx, finding)
	if err != nil {
		t.Fatal(err)
	}
	todo := addExplorationForTest(t, s, root, "Todo")
	running := addExplorationForTest(t, s, root, "Running")
	if _, err := s.AssignExploration(ctx, running, "running-agent", "output://running"); err != nil {
		t.Fatal(err)
	}
	freed, err := s.FreeWorkspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	data := freed.Data.(WorkspaceFree)
	if !data.Changed || data.RuntimeOccupied || data.ClosedTaskID == nil || *data.ClosedTaskID != taskID || data.CancelledTodo != 1 || data.CancelledRunning != 1 {
		t.Fatalf("incorrect free result: %#v", data)
	}
	for _, id := range []string{todo, running} {
		result, err := s.GetExploration(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		exp := result.Data.(domain.Exploration)
		if exp.Status != "finished" || value(exp.Outcome) != "cancelled" || value(exp.OutcomeReason) == "" || exp.FinishedAt == nil || !reflect.DeepEqual(exp.Sources, []string{root}) {
			t.Fatalf("Exploration was not cancelled correctly: %#v", exp)
		}
		if id == todo && (exp.ExplorerID != nil || exp.OutputRef != nil || exp.AssignedAt != nil) {
			t.Fatalf("todo Exploration gained execution fields: %#v", exp)
		}
		if id == running && (value(exp.ExplorerID) != "running-agent" || value(exp.OutputRef) != "output://running" || exp.AssignedAt == nil) {
			t.Fatalf("running Exploration lost execution fields: %#v", exp)
		}
	}
	doneAfter, err := s.GetExploration(ctx, done)
	if err != nil || !reflect.DeepEqual(doneBefore.Data, doneAfter.Data) {
		t.Fatalf("completed Exploration changed: %#v %v", doneAfter, err)
	}
	cogAfter, err := s.GetCognition(ctx, finding)
	if err != nil || !reflect.DeepEqual(cogBefore.Data, cogAfter.Data) {
		t.Fatalf("Cognition changed: %#v %v", cogAfter, err)
	}
	check, err := s.CheckWorkspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	state := check.Data.(WorkspaceCheck)
	if state.RuntimeOccupied || state.State != "no_open_task" || state.Tasks == nil || len(*state.Tasks) != 1 || value((*state.Tasks)[0].Outcome) != "cancelled" || value((*state.Tasks)[0].OutcomeReason) == "" || (*state.Tasks)[0].ClosedAt == nil {
		t.Fatalf("Workspace is still occupied or Task was not closed: %#v", state)
	}
	s.Clock.(*testClock).Advance(time.Minute)
	again, err := s.FreeWorkspace(ctx)
	if err != nil || again.Data.(WorkspaceFree).Changed {
		t.Fatalf("repeated free was not idempotent: %#v %v", again, err)
	}
	after, err := s.CheckWorkspace(ctx)
	if err != nil || !reflect.DeepEqual(check.Data, after.Data) {
		t.Fatalf("repeated free modified Task history: %#v %v", after, err)
	}
	if _, err := s.OpenTask(ctx, OpenTaskRequest{Purpose: "Continue Subject", Concurrency: 1}); err != nil {
		t.Fatalf("freed Workspace is not reusable: %v", err)
	}
	next := addExplorationForTest(t, s, root, "Next")
	if !domain.IDLess(running, next) {
		t.Fatalf("free reset Exploration numbering: previous=%s next=%s", running, next)
	}
}

func TestFreeWorkspaceEmptyIsIdempotent(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	if _, err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		result, err := s.FreeWorkspace(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if data := result.Data.(WorkspaceFree); data.Changed || data.RuntimeOccupied || data.ClosedTaskID != nil || data.CancelledTodo != 0 || data.CancelledRunning != 0 {
			t.Fatalf("free changed an empty Workspace: %#v", data)
		}
	}
}

func TestFreeWorkspaceRollsBackWhenOccupancyUpdateFails(t *testing.T) {
	ctx := context.Background()
	s, _, root := protocolWorkspace(t)
	todo := addExplorationForTest(t, s, root, "Todo")
	running := addExplorationForTest(t, s, root, "Running")
	if _, err := s.AssignExploration(ctx, running, "agent", "output://running"); err != nil {
		t.Fatal(err)
	}
	before, err := s.CheckWorkspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	execDatabase(t, s.Paths.Database, `CREATE TRIGGER reject_free BEFORE UPDATE OF runtime_occupied ON workspace_meta
        WHEN NEW.runtime_occupied=0
        BEGIN SELECT RAISE(ABORT, 'injected free failure'); END`)
	result, err := s.FreeWorkspace(ctx)
	if err == nil || result.Data != nil {
		t.Fatalf("free accepted a failed transaction: %#v %v", result, err)
	}
	after, err := s.CheckWorkspace(ctx)
	if err != nil || !reflect.DeepEqual(before.Data, after.Data) {
		t.Fatalf("failed free changed Task or occupancy: %#v %v", after, err)
	}
	for id, status := range map[string]string{todo: "todo", running: "running"} {
		exp, err := s.GetExploration(ctx, id)
		if err != nil || exp.Data.(domain.Exploration).Status != status || exp.Data.(domain.Exploration).FinishedAt != nil {
			t.Fatalf("failed free changed Exploration %s: %#v %v", id, exp, err)
		}
	}
}
