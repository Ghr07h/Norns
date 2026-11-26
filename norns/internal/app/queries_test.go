package app

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/domain"
)

func TestGetEntitiesAcrossOriginsStatesAndClosedTask(t *testing.T) {
	ctx := context.Background()
	s, task, root := protocolWorkspace(t)
	metadata := `{"type":"theorem","scope":"geometry"}`
	global, err := s.AddCognition(ctx, AddCognitionRequest{Title: "Theorem", Content: "Full assertion", Origin: "user", Meta: &metadata})
	if err != nil {
		t.Fatal(err)
	}
	globalID := resultID(t, global)
	exp := addExplorationForTest(t, s, root, "Inspect")
	hints := []string{addUserHint(t, s, exp, true), addUserHint(t, s, exp, false)}
	finding := addFinding(t, s, exp)
	before, err := os.ReadDir(s.Paths.View)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		id, origin, ref string
		global          bool
	}{
		{root, "task", task, false},
		{globalID, "user", task, true},
		{hints[0], "user", exp, false},
		{finding, "exploration", exp, false},
	} {
		result, err := s.GetCognition(ctx, tc.id)
		if err != nil {
			t.Fatal(err)
		}
		c := result.Data.(domain.Cognition)
		if c.ID != tc.id || c.Title == "" || c.Content == "" || c.CreatedAt == "" || c.Origin != tc.origin || value(c.OriginRef) != tc.ref || (c.TaskID == nil) != tc.global {
			t.Fatalf("incomplete Cognition: %#v", c)
		}
		if tc.global && !reflect.DeepEqual(c.Meta, map[string]string{"type": "theorem", "scope": "geometry"}) {
			t.Fatalf("metadata lost: %#v", c)
		}
		if tc.id == hints[0] && !c.TaskPurposeRelevant {
			t.Fatal("relevance lost")
		}
		payload, _ := json.Marshal(c)
		var fields map[string]any
		_ = json.Unmarshal(payload, &fields)
		if _, ok := fields["origin_task_id"]; ok {
			t.Fatalf("physical provenance field exposed: %s", payload)
		}
		if _, ok := fields["origin_exploration_id"]; ok {
			t.Fatalf("physical provenance field exposed: %s", payload)
		}
	}

	assertExp := func(id, status string, wantHints []string) domain.Exploration {
		t.Helper()
		result, err := s.GetExploration(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		e := result.Data.(domain.Exploration)
		if e.ID != id || e.Title != "Inspect" || e.Content == "" || e.TaskID != task || e.Status != status || e.CreatedAt == "" || !reflect.DeepEqual(e.Sources, []string{root}) || !reflect.DeepEqual(e.Hints, wantHints) {
			t.Fatalf("incomplete Exploration: %#v", e)
		}
		return e
	}
	if e := assertExp(exp, "todo", hints); e.ExplorerID != nil || e.FinishedAt != nil {
		t.Fatalf("todo has execution state: %#v", e)
	}
	if _, err := s.AssignExploration(ctx, exp, "explorer-one", "output://inspect"); err != nil {
		t.Fatal(err)
	}
	if e := assertExp(exp, "running", hints); value(e.ExplorerID) != "explorer-one" || value(e.OutputRef) != "output://inspect" || e.AssignedAt == nil {
		t.Fatalf("execution fields lost: %#v", e)
	}
	if _, err := s.FinishExploration(ctx, exp, "succeeded", "Verified"); err != nil {
		t.Fatal(err)
	}
	if e := assertExp(exp, "finished", hints); value(e.Outcome) != "succeeded" || value(e.OutcomeReason) != "Verified" || e.FinishedAt == nil || value(e.ExplorerID) != "explorer-one" {
		t.Fatalf("completion fields lost: %#v", e)
	}
	retry, err := s.RetryExploration(ctx, exp)
	if err != nil {
		t.Fatal(err)
	}
	if e := assertExp(resultID(t, retry), "todo", []string{}); value(e.RetryOf) != exp {
		t.Fatalf("retry reference lost: %#v", e)
	}
	if _, err := s.CloseTask(ctx, "succeeded", "Done"); err != nil {
		t.Fatal(err)
	}
	assertExp(exp, "finished", hints)
	if _, err := s.GetCognition(ctx, finding); err != nil {
		t.Fatalf("closed Task Cognition inaccessible: %v", err)
	}
	after, err := os.ReadDir(s.Paths.View)
	if (err != nil && !os.IsNotExist(err)) || !reflect.DeepEqual(before, after) {
		t.Fatalf("get published View files: %v", err)
	}
}

func TestGetEntitiesValidateIDsAndMissingRecords(t *testing.T) {
	s, _, root := protocolWorkspace(t)
	ctx := context.Background()
	missingCog := "cog_9223372036854775807"
	missingExp := "exp_9223372036854775807"
	for _, tc := range []struct {
		get      func(context.Context, string) (Result, error)
		id, code string
	}{
		{s.GetCognition, "", "INVALID_ARGUMENT"},
		{s.GetExploration, root, "INVALID_ARGUMENT"},
		{s.GetCognition, missingExp, "INVALID_ARGUMENT"},
		{s.GetCognition, missingCog, "COGNITION_NOT_FOUND"},
		{s.GetExploration, missingExp, "EXPLORATION_NOT_FOUND"},
	} {
		if _, err := tc.get(ctx, tc.id); errorCode(err) != tc.code {
			t.Fatalf("get(%q): expected %s, got %v", tc.id, tc.code, err)
		}
	}
}
