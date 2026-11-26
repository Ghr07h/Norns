package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func protocolWorkspace(t *testing.T) (*Service, string, string) {
	t.Helper()
	s, _ := newTestService(t)
	info := "Subject"
	opened, err := s.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Investigate", Concurrency: 2, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	return s, resultID(t, opened), opened.Data.(map[string]any)["init_cognition_ref"].(string)
}

func addUserHint(t *testing.T, s *Service, exp string, relevant bool) string {
	t.Helper()
	r, err := s.AddCognition(context.Background(), AddCognitionRequest{Title: "User input", Content: "An assertion to consider", Origin: "user", ExplorationID: &exp, Relevant: relevant})
	if err != nil {
		t.Fatal(err)
	}
	return resultID(t, r)
}

func addFinding(t *testing.T, s *Service, exp string) string {
	t.Helper()
	r, err := s.AddCognition(context.Background(), AddCognitionRequest{Title: "Finding", Content: "Observed evidence", Origin: "exploration", ExplorationID: &exp})
	if err != nil {
		t.Fatal(err)
	}
	return resultID(t, r)
}

func readTrace(t *testing.T, s *Service, exp string) traceView {
	t.Helper()
	b, _, err := s.TraceView(context.Background(), exp, ViewJSON)
	if err != nil {
		t.Fatal(err)
	}
	var view traceView
	if err := json.Unmarshal(b, &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func TestJSONReferencesAndHintProjection(t *testing.T) {
	ctx := context.Background()
	s, task, root := protocolWorkspace(t)
	global, err := s.AddCognition(ctx, AddCognitionRequest{Title: "Global", Content: "Global assertion", Origin: "user"})
	if err != nil {
		t.Fatal(err)
	}
	if global.Data.(map[string]any)["origin_ref"] != task || global.Data.(map[string]any)["task_id"] != nil {
		t.Fatal("global provenance must reference the adding Task")
	}
	a := addExplorationForTest(t, s, root, "A")
	h := addUserHint(t, s, a, true)
	sources := []string{h, resultID(t, global), root}
	b, err := s.AddExploration(ctx, AddExplorationRequest{Title: "B", Content: "Use every input type", Sources: sources})
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListExplorations(ctx, "todo")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(list.Explorations[0]["hints"].([]string), []string{h}) || len(list.Explorations[1]["hints"].([]string)) != 0 || !slices.Equal(list.Explorations[1]["sources"].([]string), sources) {
		t.Fatalf("wrong query ID arrays: %#v", list)
	}
	db, err := sql.Open("sqlite", s.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var stored string
	if err := db.QueryRow(`SELECT sources FROM explorations WHERE id=?`, resultID(t, b)).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != encodeIDs(sources) {
		t.Fatalf("source order was lost: %s", stored)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('exploration_sources','exploration_hints')`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unexpected relation tables: %d %v", count, err)
	}
	payload, _, err := s.StatusView(ctx, nil, ViewJSON)
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{"source_cognitions", "global_cognitions", "asserted_cognitions"} {
		if strings.Contains(string(payload), old) {
			t.Fatalf("obsolete field %s", old)
		}
	}
	var raw struct {
		Graph struct {
			Cognitions   []map[string]any `json:"cognitions"`
			Explorations []map[string]any `json:"explorations"`
		} `json:"graph"`
		Global []map[string]any `json:"global_hints"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw.Graph.Cognitions) != 1 || raw.Graph.Cognitions[0]["id"] != root || raw.Graph.Cognitions[0]["origin_ref"] != task {
		t.Fatal("only Task-init should be in graph.cognitions")
	}
	if len(raw.Global) != 1 || len(raw.Global[0]) != 3 {
		t.Fatal("global hint projection is not three fields")
	}
	hints := raw.Graph.Explorations[0]["hints"].([]any)
	if len(hints) != 1 || len(hints[0].(map[string]any)) != 3 || hints[0].(map[string]any)["id"] != h {
		t.Fatal("hint projection is not three fields")
	}
	if _, exists := raw.Graph.Explorations[1]["hints"]; exists {
		t.Fatal("empty View hints must be omitted")
	}
}

func TestHintAppendRollsBackCognitionOnFailure(t *testing.T) {
	s, _, root := protocolWorkspace(t)
	e := addExplorationForTest(t, s, root, "Target")
	db, err := sql.Open("sqlite", s.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER fail_hint BEFORE UPDATE OF hints ON explorations BEGIN SELECT RAISE(ABORT,'hint append failed'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddCognition(context.Background(), AddCognitionRequest{Title: "Hint", Content: "Must roll back", Origin: "user", ExplorationID: &e}); errorCode(err) != "INVALID_STATE" {
		t.Fatalf("unexpected failure: %v", err)
	}
	var count int
	var hints string
	if err := db.QueryRow(`SELECT count(*) FROM cognitions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT hints FROM explorations WHERE id=?`, e).Scan(&hints); err != nil {
		t.Fatal(err)
	}
	if count != 1 || hints != "[]" {
		t.Fatalf("partial hint transaction: cognitions=%d hints=%s", count, hints)
	}
}

func TestConcurrentHintAppendsPreserveEveryCognition(t *testing.T) {
	s, _, root := protocolWorkspace(t)
	e := addExplorationForTest(t, s, root, "Target")
	const count = 12
	var wg sync.WaitGroup
	results := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.AddCognition(context.Background(), AddCognitionRequest{Title: "Hint", Content: "Concurrent input", Origin: "user", ExplorationID: &e})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListExplorations(context.Background(), "todo")
	if err != nil {
		t.Fatal(err)
	}
	ids := list.Explorations[0]["hints"].([]string)
	if len(ids) != count {
		t.Fatalf("concurrent appends lost IDs: %v", ids)
	}
	if len(readTrace(t, s, e).Related.Explorations[0].Hints) != count {
		t.Fatal("View lost appended hints")
	}
}

func TestPersistedReferenceArraysRejectCorruption(t *testing.T) {
	for _, test := range []string{"malformed", "null", "empty-sources", "wrong-type", "wrong-id-type", "missing-source", "duplicate-source", "missing-hint", "duplicate-hint", "global-as-hint", "derived-as-hint", "foreign-hint", "unlisted-user-cog", "graph-cycle"} {
		t.Run(test, func(t *testing.T) {
			s, _, root := protocolWorkspace(t)
			a := addExplorationForTest(t, s, root, "A")
			b := addExplorationForTest(t, s, root, "B")
			h := addUserHint(t, s, a, false)
			hb := addUserHint(t, s, b, false)
			derived := addFinding(t, s, a)
			global, err := s.AddCognition(context.Background(), AddCognitionRequest{Title: "Global", Content: "Global input", Origin: "user"})
			if err != nil {
				t.Fatal(err)
			}
			field, value := "sources", "[]"
			switch test {
			case "malformed":
				value = "["
			case "null":
				value = "null"
			case "empty-sources":
				value = "[]"
			case "wrong-type":
				value = `[1]`
			case "wrong-id-type":
				value = `["` + a + `"]`
			case "missing-source":
				value = encodeIDs([]string{"cog_9223372036854775807"})
			case "duplicate-source":
				value = encodeIDs([]string{root, root})
			case "missing-hint":
				field = "hints"
				value = encodeIDs([]string{"cog_9223372036854775807"})
			case "duplicate-hint":
				field = "hints"
				value = encodeIDs([]string{h, h})
			case "global-as-hint":
				field = "hints"
				value = encodeIDs([]string{h, resultID(t, global)})
			case "derived-as-hint":
				field = "hints"
				value = encodeIDs([]string{h, derived})
			case "foreign-hint":
				field = "hints"
				value = encodeIDs([]string{h, hb})
			case "unlisted-user-cog":
				field = "hints"
				value = "[]"
			case "graph-cycle":
				value = encodeIDs([]string{h})
			}
			db, err := sql.Open("sqlite", s.Paths.Database)
			if err != nil {
				t.Fatal(err)
			}
			db.SetMaxOpenConns(1)
			if _, err := db.Exec(`PRAGMA ignore_check_constraints=ON`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE explorations SET `+field+`=? WHERE id=?`, value, a); err != nil {
				t.Fatal(err)
			}
			db.Close()
			if err := s.Store.Read(context.Background(), func(*sql.Tx) error { return nil }); errorCode(err) != "WORKSPACE_STATE_UNTRUSTED" {
				t.Fatalf("accepted %s corruption: %v", test, err)
			}
		})
	}
}

func TestTraceHintSourcesAndRelevantHintsKeepMinimalAncestors(t *testing.T) {
	s, _, root := protocolWorkspace(t)
	a := addExplorationForTest(t, s, root, "A")
	h := addUserHint(t, s, a, false)
	h2 := addUserHint(t, s, a, false)
	unusedA := addFinding(t, s, a)
	b := addExplorationForTest(t, s, h, "B")
	c := addExplorationForTest(t, s, root, "Relevant hint owner")
	relevant := addUserHint(t, s, c, true)
	unusedC := addFinding(t, s, c)
	unrelated := addExplorationForTest(t, s, root, "Unrelated")
	trace := readTrace(t, s, b)
	if !containsHint(trace.Related.Explorations, a, h) || !containsHint(trace.Related.Explorations, a, h2) || !containsHint(trace.Related.Explorations, c, relevant) {
		t.Fatalf("missing hint owners or complete hints: %#v", trace.Related)
	}
	for _, id := range []string{unusedA, unusedC, h, h2, relevant} {
		if containsCognition(trace.Related.Cognitions, id) {
			t.Fatalf("unexpected graph Cognition %s", id)
		}
	}
	if containsExploration(trace.Related.Explorations, unrelated) {
		t.Fatal("unrelated branch was included")
	}
	for _, e := range trace.Related.Explorations {
		if e.ID == b && !slices.Equal(e.Sources, []string{h}) {
			t.Fatal("hint source ID was rewritten")
		}
	}
}

func TestRetryPreservesHistoricalHintsWithoutCopying(t *testing.T) {
	ctx := context.Background()
	s, _, root := protocolWorkspace(t)
	a := addExplorationForTest(t, s, root, "Input owner")
	sourceHint := addUserHint(t, s, a, false)
	unused := addFinding(t, s, a)
	e := addExplorationForTest(t, s, sourceHint, "Work")
	var executions, hints, findings []string
	for i := 0; i < 3; i++ {
		executions = append(executions, e)
		if i > 0 {
			list, err := s.ListExplorations(ctx, "todo")
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range list.Explorations {
				if item["id"] == e && (len(item["hints"].([]string)) != 0 || !slices.Equal(item["sources"].([]string), []string{sourceHint})) {
					t.Fatal("retry copied hints or changed sources")
				}
			}
		}
		hints = append(hints, addUserHint(t, s, e, false))
		findings = append(findings, addFinding(t, s, e))
		if _, err := s.AssignExploration(ctx, e, fmt.Sprintf("executor-%d", i), "output://test"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.FinishExploration(ctx, e, "inconclusive", "More evidence needed"); err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			r, err := s.RetryExploration(ctx, e)
			if err != nil {
				t.Fatal(err)
			}
			e = resultID(t, r)
		}
	}
	trace := readTrace(t, s, e)
	for i, id := range executions {
		if !containsHint(trace.Related.Explorations, id, hints[i]) || !containsCognition(trace.Related.Cognitions, findings[i]) {
			t.Fatal("retry chain lost historical inputs or outputs")
		}
	}
	if containsCognition(trace.Related.Cognitions, unused) {
		t.Fatal("ancestor side output was included")
	}
}

func TestIndependentTaskInitialCognitionsDoNotExpandTrace(t *testing.T) {
	ctx := context.Background()
	s, _, firstRoot := protocolWorkspace(t)
	firstExp := addExplorationForTest(t, s, firstRoot, "Previous work")
	if _, err := s.CloseTask(ctx, "cancelled", "New purpose"); err != nil {
		t.Fatal(err)
	}
	info := "New Subject information"
	second, err := s.OpenTask(ctx, OpenTaskRequest{Purpose: "Second", Concurrency: 1, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	secondRoot := second.Data.(map[string]any)["init_cognition_ref"].(string)
	secondExp := addExplorationForTest(t, s, secondRoot, "New work")
	trace := readTrace(t, s, secondExp)
	if containsCognition(trace.Related.Cognitions, firstRoot) || containsExploration(trace.Related.Explorations, firstExp) || !containsCognition(trace.Related.Cognitions, secondRoot) {
		t.Fatal("Task provenance introduced a nonexistent ancestry")
	}
	status := decodeStatus(t, s, nil)
	if !containsCognition(status.Graph.Cognitions, firstRoot) || !containsCognition(status.Graph.Cognitions, secondRoot) {
		t.Fatal("Status must retain independent initial cognitions")
	}
	if err := s.Store.Read(ctx, func(tx *sql.Tx) error {
		all, err := loadAll(ctx, tx)
		if err != nil {
			return err
		}
		for _, edge := range visualEdges(all) {
			if edge.Kind == "extends" || strings.HasPrefix(edge.From, "task_") || strings.HasPrefix(edge.To, "task_") {
				t.Errorf("invalid knowledge edge: %#v", edge)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTimestampOrderPreservesFractionalSeconds(t *testing.T) {
	t.Run("views and queries", func(t *testing.T) {
		s, clock := newTestService(t)
		info := "Subject"
		opened, err := s.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Order", Concurrency: 1, Info: &info})
		if err != nil {
			t.Fatal(err)
		}
		root := opened.Data.(map[string]any)["init_cognition_ref"].(string)
		first := addExplorationForTest(t, s, root, "First")
		h1 := addUserHint(t, s, first, false)
		clock.Advance(time.Nanosecond)
		second := addExplorationForTest(t, s, root, "Second")
		h2 := addUserHint(t, s, first, false)
		status := decodeStatus(t, s, nil)
		if status.Graph.Explorations[0].ID != first || status.Graph.Explorations[1].ID != second || status.Graph.Explorations[0].Hints[0].ID != h1 || status.Graph.Explorations[0].Hints[1].ID != h2 {
			t.Fatal("View sorted fractional timestamps as text")
		}
		list, err := s.ListExplorations(context.Background(), "todo")
		if err != nil {
			t.Fatal(err)
		}
		if list.Explorations[0]["id"] != first || list.Explorations[1]["id"] != second {
			t.Fatal("query sorted fractional timestamps as text")
		}
	})
	t.Run("latest closed Task", func(t *testing.T) {
		ctx := context.Background()
		s, clock := newTestService(t)
		info := "First input"
		if _, err := s.OpenTask(ctx, OpenTaskRequest{Purpose: "First", Concurrency: 1, Info: &info}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CloseTask(ctx, "succeeded", "Done"); err != nil {
			t.Fatal(err)
		}
		clock.Advance(time.Nanosecond)
		info = "Second input"
		second, err := s.OpenTask(ctx, OpenTaskRequest{Purpose: "Second", Concurrency: 1, Info: &info})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.CloseTask(ctx, "succeeded", "Done"); err != nil {
			t.Fatal(err)
		}
		third, err := s.OpenTask(ctx, OpenTaskRequest{Purpose: "Third", Concurrency: 1})
		if err != nil {
			t.Fatal(err)
		}
		if third.Data.(map[string]any)["init_cognition_ref"] != second.Data.(map[string]any)["init_cognition_ref"] {
			t.Fatal("latest closed Task was selected using lexical timestamp order")
		}
	})
}
