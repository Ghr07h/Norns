package app

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sync"
	"testing"

	storesqlite "github.com/Ghr07h/norns/norns/internal/store/sqlite"
)

func TestSequentialIDsAcrossTasksRetryAndReset(t *testing.T) {
	ctx := context.Background()
	s, _, root := protocolWorkspace(t)
	if root != "cog_1" {
		t.Fatalf("first cognition: %s", root)
	}
	a := addExplorationForTest(t, s, root, "First")
	if a != "exp_1" || addUserHint(t, s, a, false) != "cog_2" || addFinding(t, s, a) != "cog_3" {
		t.Fatal("independent entity sequences failed")
	}
	if _, err := s.FinishExploration(ctx, a, "cancelled", "Retry"); err != nil {
		t.Fatal(err)
	}
	retry, err := s.RetryExploration(ctx, a)
	if err != nil || resultID(t, retry) != "exp_2" {
		t.Fatalf("retry must allocate a new ID: %v", err)
	}
	if _, err := s.CloseTask(ctx, "cancelled", "Next"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenTask(ctx, OpenTaskRequest{Purpose: "Reuse", Concurrency: 1}); err != nil {
		t.Fatal(err)
	}
	if addExplorationForTest(t, s, root, "Second Task") != "exp_3" {
		t.Fatal("Task transition reset Exploration numbering")
	}
	if _, err := s.CloseTask(ctx, "cancelled", "Next"); err != nil {
		t.Fatal(err)
	}
	info := "New initial cognition"
	opened, err := s.OpenTask(ctx, OpenTaskRequest{Purpose: "New info", Concurrency: 1, Info: &info})
	if err != nil || opened.Data.(map[string]any)["init_cognition_ref"] != "cog_4" {
		t.Fatalf("Task transition reset Cognition numbering: %v", err)
	}
	if _, err := s.CloseTask(ctx, "cancelled", "Reset"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	opened, err = s.OpenTask(ctx, OpenTaskRequest{Purpose: "New Workspace", Concurrency: 1, Info: &info})
	if err != nil || opened.Data.(map[string]any)["init_cognition_ref"] != "cog_1" {
		t.Fatalf("reset did not start a fresh sequence: %v", err)
	}
	if addExplorationForTest(t, s, "cog_1", "New Workspace") != "exp_1" {
		t.Fatal("reset inherited Exploration counter")
	}
	fresh, err := New(s.Paths.Root)
	if err != nil {
		t.Fatal(err)
	}
	if addUserHint(t, fresh, "exp_1", false) != "cog_2" || addExplorationForTest(t, fresh, "cog_1", "New process") != "exp_2" {
		t.Fatal("counters did not survive Service restart")
	}
}

func TestSequentialIDAllocationRollsBackAndRejectsExhaustion(t *testing.T) {
	ctx := context.Background()
	s, _, root := protocolWorkspace(t)
	db, err := sql.Open("sqlite", s.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER fail_cog BEFORE INSERT ON cognitions WHEN NEW.origin='user' BEGIN SELECT RAISE(ABORT,'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddCognition(ctx, AddCognitionRequest{Title: "Fail", Content: "Fail", Origin: "user"}); err == nil {
		t.Fatal("injected failure did not fail")
	}
	if _, err := db.Exec(`DROP TRIGGER fail_cog`); err != nil {
		t.Fatal(err)
	}
	first := addExplorationForTest(t, s, root, "First")
	if addUserHint(t, s, first, false) != "cog_2" {
		t.Fatal("rolled-back cognition consumed an ID")
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_exp BEFORE INSERT ON explorations BEGIN SELECT RAISE(ABORT,'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddExploration(ctx, AddExplorationRequest{Title: "Fail", Content: "Fail"}); err == nil {
		t.Fatal("injected failure did not fail")
	}
	if _, err := db.Exec(`DROP TRIGGER fail_exp`); err != nil {
		t.Fatal(err)
	}
	if addExplorationForTest(t, s, root, "Second") != "exp_2" {
		t.Fatal("rolled-back Exploration consumed an ID")
	}
	if _, err := db.Exec(`UPDATE workspace_meta SET last_cognition_number=?,last_exploration_number=?`, int64(math.MaxInt64-1), int64(math.MaxInt64-1)); err != nil {
		t.Fatal(err)
	}
	lastCog, err := s.AddCognition(ctx, AddCognitionRequest{Title: "Last", Content: "Last", Origin: "user"})
	if err != nil || resultID(t, lastCog) != "cog_9223372036854775807" {
		t.Fatalf("last Cognition allocation: %v", err)
	}
	lastExp, err := s.AddExploration(ctx, AddExplorationRequest{Title: "Last", Content: "Last"})
	if err != nil || resultID(t, lastExp) != "exp_9223372036854775807" {
		t.Fatalf("last Exploration allocation: %v", err)
	}
	if _, err := s.AddCognition(ctx, AddCognitionRequest{Title: "Overflow", Content: "Overflow", Origin: "user"}); errorCode(err) != "INVALID_STATE" {
		t.Fatalf("Cognition overflow: %v", err)
	}
	if _, err := s.AddExploration(ctx, AddExplorationRequest{Title: "Overflow", Content: "Overflow"}); errorCode(err) != "INVALID_STATE" {
		t.Fatalf("Exploration overflow: %v", err)
	}
}

func TestSequentialIDsAreUniqueAcrossConcurrentServices(t *testing.T) {
	ctx := context.Background()
	s, _, root := protocolWorkspace(t)
	const count = 12
	var wg sync.WaitGroup
	errors := make(chan error, count)
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service, err := New(s.Paths.Root)
			if err == nil {
				_, err = service.AddExploration(ctx, AddExplorationRequest{Title: "Concurrent", Content: "Concurrent", Sources: []string{root}})
			}
			if err == nil {
				_, err = service.AddCognition(ctx, AddCognitionRequest{Title: "Concurrent", Content: "Concurrent", Origin: "user"})
			}
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= count; i++ {
		if _, err := s.GetExploration(ctx, fmt.Sprintf("exp_%d", i)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetCognition(ctx, fmt.Sprintf("cog_%d", i+1)); err != nil {
			t.Fatal(err)
		}
	}
	// Populate equal timestamps on both sides of the 9/10 boundary.
	if err := s.Store.WriteQ(ctx, func(q storesqlite.Queryer) error {
		_, err := q.ExecContext(ctx, `UPDATE explorations SET created_at=(SELECT created_at FROM workspace_meta)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.WriteQ(ctx, func(q storesqlite.Queryer) error {
		_, err := q.ExecContext(ctx, `UPDATE cognitions SET created_at=(SELECT created_at FROM workspace_meta)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	status := decodeStatus(t, s, nil)
	for i, cog := range status.GlobalHints {
		if cog.ID != fmt.Sprintf("cog_%d", i+2) {
			t.Fatalf("Cognition numeric order lost: %#v", status.GlobalHints)
		}
	}
	list, err := s.ListExplorations(ctx, "todo")
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range list.Explorations {
		if item["id"] != fmt.Sprintf("exp_%d", i+1) {
			t.Fatalf("numeric creation order lost: %#v", list)
		}
	}
}

func TestSequentialIDCountersRejectCorruption(t *testing.T) {
	for _, statement := range []string{`UPDATE workspace_meta SET last_cognition_number=0`, `UPDATE workspace_meta SET last_exploration_number=0`, `UPDATE workspace_meta SET last_cognition_number=-1`, `UPDATE workspace_meta SET last_exploration_number=1.5`} {
		t.Run(statement, func(t *testing.T) {
			s, _, root := protocolWorkspace(t)
			addExplorationForTest(t, s, root, "Existing")
			db, err := sql.Open("sqlite", s.Paths.Database)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec(`PRAGMA ignore_check_constraints=ON`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			if _, _, err := s.StatusView(context.Background(), nil, ViewYAML); errorCode(err) != "WORKSPACE_STATE_UNTRUSTED" {
				t.Fatalf("corrupt counter accepted: %v", err)
			}
		})
	}
}

func TestSequentialInitialAllocationFailureKeepsWorkspaceEmpty(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)
	if _, err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.WriteQ(ctx, func(q storesqlite.Queryer) error {
		_, err := q.ExecContext(ctx, `UPDATE workspace_meta SET last_cognition_number=?`, int64(math.MaxInt64))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	info := "Subject"
	if _, err := s.OpenTask(ctx, OpenTaskRequest{Purpose: "Exhausted", Concurrency: 1, Info: &info}); errorCode(err) != "INVALID_STATE" {
		t.Fatalf("initial ID overflow: %v", err)
	}
	status := decodeStatus(t, s, nil)
	if status.Tasks == nil || len(*status.Tasks) != 0 || len(status.Graph.Cognitions) != 0 {
		t.Fatal("failed initial allocation partially opened a Task")
	}
}

func TestSequentialDerivedCognitionsUseNumericTieBreak(t *testing.T) {
	s, _, root := protocolWorkspace(t)
	exp := addExplorationForTest(t, s, root, "Findings")
	for range 13 {
		addFinding(t, s, exp)
	}
	status := decodeStatus(t, s, nil)
	for i, cog := range status.Graph.Cognitions {
		if cog.ID != fmt.Sprintf("cog_%d", i+1) {
			t.Fatalf("same-time graph ordering: %#v", status.Graph.Cognitions)
		}
	}
	if len(status.RecentTop10DerivedCognitions) != 10 {
		t.Fatal("wrong recent count")
	}
	for i, id := range status.RecentTop10DerivedCognitions {
		if id != fmt.Sprintf("cog_%d", i+2) {
			t.Fatalf("same-time recent ordering: %#v", status.RecentTop10DerivedCognitions)
		}
	}
}
