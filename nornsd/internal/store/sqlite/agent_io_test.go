package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/google/uuid"
)

func TestReplaySettlementHandlesMergedAndNotificationInputs(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	s, _, err := Open(ctx, filepath.Join(t.TempDir(), "nornsd.sqlite"), uuid.NewString(), now)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sid, rid := uuid.NewString(), uuid.NewString()
	text := "first"
	if err = s.CreateSubject(ctx, domain.Subject{ID: sid, DirectoryName: sid}, &text, rid, now); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = s.ClaimNext(ctx, 1, now); err != nil {
		t.Fatal(err)
	}
	dispatch := func() string {
		t.Helper()
		_, group, err := s.DispatchPending(ctx, sid, rid, false, now)
		if err != nil || group == 0 {
			t.Fatalf("dispatch %d: %v", group, err)
		}
		if err = s.ConfirmDispatch(ctx, sid, rid, group, now); err != nil {
			t.Fatal(err)
		}
		return domain.InputUUID(rid, group)
	}
	accept := func(id string) {
		t.Helper()
		if err := s.AcceptInput(ctx, sid, rid, id, now); err != nil {
			t.Fatal(err)
		}
	}
	complete := func(frame string, expectedState domain.RuntimeState, inflight int) domain.Runtime {
		t.Helper()
		r, err := s.CompleteResult(ctx, sid, rid, json.RawMessage(frame), 1, now)
		if err != nil || r.State != expectedState || r.InflightInputs != inflight {
			t.Fatalf("result: state=%s inputs=%d err=%v", r.State, r.InflightInputs, err)
		}
		return r
	}
	first := dispatch()
	for range 2 {
		if _, err = s.SubmitInstruction(ctx, sid, uuid.NewString(), "next", false, now); err != nil {
			t.Fatal(err)
		}
		dispatch()
	}
	// Pure old notifications cannot consume any of the written inputs.
	complete(`{"uuid":"old-notification","origin":{"kind":"task-notification"},"usage":{"input_tokens":0}}`, domain.RuntimeRunning, 3)
	accept("historical-unknown-uuid")
	accept(first)
	accept(first) // repeated replay is idempotent
	complete(`{"uuid":"r1","usage":{"input_tokens":10,"server_tool_use":{"web_search_requests":1}},"modelUsage":{"m":{"inputTokens":10}},"total_cost_usd":0.1}`, domain.RuntimeRunning, 2)
	accept(first) // late duplicate must not be re-settled
	accept(domain.InputUUID(rid, 2))
	// A duplicate old result must not consume a newly accepted input.
	complete(`{"uuid":"r1","usage":{"input_tokens":10}}`, domain.RuntimeRunning, 2)
	accept(domain.InputUUID(rid, 3))
	r := complete(`{"uuid":"r2","origin":{"kind":"task-notification"},"is_error":true,"subtype":"success","api_error_status":429,"usage":{"input_tokens":2,"server_tool_use":{"web_search_requests":2}},"modelUsage":{"m":{"inputTokens":15}},"total_cost_usd":0.2}`, domain.RuntimeWaiting, 0)
	var usage runtimeUsage
	if err = json.Unmarshal(r.UsageJSON, &usage); err != nil {
		t.Fatal(err)
	}
	if usage.ResultCount != 3 || usage.ErrorResultCount != 1 || usage.Totals["input_tokens"] != float64(12) || usage.TotalCostUSD != float64(.2) || usage.ModelUsage.(map[string]any)["m"].(map[string]any)["inputTokens"] != float64(15) {
		t.Fatalf("usage: %s", r.UsageJSON)
	}
	if usage.Totals["server_tool_use"].(map[string]any)["web_search_requests"] != float64(3) {
		t.Fatalf("nested totals: %s", r.UsageJSON)
	}
	// Error results leave the process available for the next user instruction.
	if _, err = s.SubmitInstruction(ctx, sid, uuid.NewString(), "recover", false, now); err != nil {
		t.Fatal(err)
	}
	id := dispatch()
	accept(id)
	complete(`{"uuid":"r3","is_error":false,"usage":{"input_tokens":4}}`, domain.RuntimeWaiting, 0)
	if err = s.SetRuntimeTerminal(ctx, sid, rid, domain.RuntimeCancelled, "stop", "", now, false); err != nil {
		t.Fatal(err)
	}
	if err = s.MarkAgentProcessing(ctx, sid, rid, now); err != nil {
		t.Fatal(err)
	}
	complete(`{"uuid":"trailing-result","usage":{"input_tokens":1}}`, domain.RuntimeCancelled, 0)
	items, _, err := s.DispatchPending(ctx, sid, rid, false, now)
	if err != nil || len(items) != 0 {
		t.Fatalf("terminal dispatch: %v %v", items, err)
	}
	events, err := s.ListEvents(ctx, sid, 100, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	settled := 0
	for _, event := range events {
		if event.Type == "instruction.settled" {
			settled++
		}
	}
	if settled != 3 {
		t.Fatalf("settlement events=%d", settled)
	}
}
