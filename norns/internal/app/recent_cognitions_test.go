package app

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestStatusRecentDerivedCognitionsWindowAndScope(t *testing.T) {
	ctx := context.Background()
	service, clock := newTestService(t)
	info := "Subject"
	first, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "First", Concurrency: 1, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	firstTask := resultID(t, first)
	root := first.Data.(map[string]any)["init_cognition_ref"].(string)
	firstExp := addExplorationForTest(t, service, root, "First exploration")
	old := addFinding(t, service, firstExp)
	clock.Advance(time.Nanosecond)
	boundary := addFinding(t, service, firstExp)
	clock.Advance(30 * time.Minute)
	latest := addFinding(t, service, firstExp)
	if _, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Global", Content: "Assertion", Origin: "user"}); err != nil {
		t.Fatal(err)
	}
	addUserHint(t, service, firstExp, false)
	if _, err := service.CloseTask(ctx, "cancelled", "Next Task"); err != nil {
		t.Fatal(err)
	}
	secondInfo := "Additional Subject information"
	second, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Second", Concurrency: 1, Info: &secondInfo})
	if err != nil {
		t.Fatal(err)
	}
	secondRoot := second.Data.(map[string]any)["init_cognition_ref"].(string)
	secondExp := addExplorationForTest(t, service, secondRoot, "Second exploration")
	otherTaskFinding := addFinding(t, service, secondExp)
	want := []string{latest, otherTaskFinding, boundary}
	for _, requested := range []*string{nil, &firstTask} {
		status := decodeStatus(t, service, requested)
		if !slices.Equal(status.RecentTop10DerivedCognitions, want) {
			t.Fatalf("wrong recent derived IDs: got %v want %v", status.RecentTop10DerivedCognitions, want)
		}
		if !containsCognition(status.Graph.Cognitions, old) {
			t.Fatal("recent summary removed older records from the full graph")
		}
	}
	clock.Advance(time.Nanosecond)
	future := addFinding(t, service, secondExp)
	clock.Advance(-time.Nanosecond)
	if got := decodeStatus(t, service, nil); !slices.Equal(got.RecentTop10DerivedCognitions, want) || !containsCognition(got.Graph.Cognitions, future) {
		t.Fatal("future-dated records must remain in the graph but not enter the recent summary")
	}
	clock.Advance(30*time.Minute + 2*time.Nanosecond)
	for _, format := range []ViewFormat{ViewYAML, ViewJSON} {
		payload, _, err := service.StatusView(ctx, nil, format)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(payload), "recent_top10_derived_cognitions") {
			t.Fatalf("empty recent summary must be omitted: %s", payload)
		}
	}
}

func TestStatusRecentDerivedCognitionsLimitAndSingleLine(t *testing.T) {
	ctx := context.Background()
	service, clock := newTestService(t)
	info := "Subject"
	opened, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Recent", Concurrency: 1, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	root := opened.Data.(map[string]any)["init_cognition_ref"].(string)
	exp := addExplorationForTest(t, service, root, "Discoveries")
	ids := []string{}
	for i := 0; i < 12; i++ {
		clock.Advance(time.Second)
		ids = append(ids, addFinding(t, service, exp))
	}
	slices.Reverse(ids)
	want := ids[:10]
	status := decodeStatus(t, service, nil)
	if !slices.Equal(status.RecentTop10DerivedCognitions, want) || len(status.Graph.Cognitions) != 13 {
		t.Fatalf("wrong top 10 or incomplete graph: %#v", status)
	}
	yaml, _, err := service.StatusView(ctx, nil, ViewYAML)
	if err != nil {
		t.Fatal(err)
	}
	line := "recent_top10_derived_cognitions: [" + strings.Join(want, ", ") + "]\n"
	if !strings.Contains(string(yaml), line) {
		t.Fatalf("recent IDs must be a single-line YAML list: %s", yaml)
	}
	assertOrdered(t, string(yaml), "running_explorations:", "recent_top10_derived_cognitions:", "generated_at:")
	trace := readTrace(t, service, exp)
	if len(trace.Related.Cognitions) != 13 {
		t.Fatal("Status summary changed Trace's cognition collection")
	}
}
