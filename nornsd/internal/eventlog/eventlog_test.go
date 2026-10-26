package eventlog

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	storesqlite "github.com/Ghr07h/norns/nornsd/internal/store/sqlite"
	"github.com/Ghr07h/norns/nornsd/internal/version"
	"github.com/google/uuid"
)

func TestHasCloseSuccessFindsEmbeddedJSON(t *testing.T) {
	value := "command output follows:\n{\"ok\":true,\"data\":{\"id\":\"task\",\"outcome\":\"succeeded\"}}\nfinished"
	if !hasCloseSuccess(value) {
		t.Fatal("embedded success envelope was not recognized")
	}
	if hasCloseSuccess(`{"ok":true,"data":{"id":"task"}}`) {
		t.Fatal("incomplete envelope was accepted")
	}
}

func TestRawProjectionIsAtomicAndReplayIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store, subjectID, runtimeID := projectionStore(t)
	root := t.TempDir()
	logger := New(store, root, subjectID, runtimeID)
	now := time.Now().UTC()
	if _, err := store.AppendEvent(ctx, subjectID, &runtimeID, "runtime.started", "runtime", map[string]any{"slot": 1}, now); err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tool-1","name":"Bash","input":{"command":"norns task close --outcome succeeded"}},{"type":"tool_result","tool_use_id":"tool-1","is_error":false,"content":"{\"ok\":true,\"data\":{\"id\":\"task\",\"outcome\":\"succeeded\"}}"}]}}`)

	result, err := logger.Record(ctx, "stdout", "agent_message", raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CloseSucceeded {
		t.Fatal("close result was not recognized")
	}
	subject, err := store.GetSubject(ctx, subjectID)
	if err != nil {
		t.Fatal(err)
	}
	if subject.LastRawSeq != 1 || subject.LastProjectedRawSeq != 1 {
		t.Fatalf("raw checkpoints = %d/%d", subject.LastRawSeq, subject.LastProjectedRawSeq)
	}
	events, err := store.ListEvents(ctx, subjectID, 10, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Type != "runtime.started" || events[1].Type != "agent.tool_call" || events[2].Type != "agent.tool_result" {
		t.Fatalf("unexpected events: %#v", events)
	}
	var payload map[string]any
	if err = json.Unmarshal(events[2].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if got := uint64(payload["tool_call_seq"].(float64)); got != events[1].Seq {
		t.Fatalf("tool result links to %d, want %d", got, events[1].Seq)
	}
	readable, err := os.ReadFile(filepath.Join(root, ".nornsd", "log", "readable.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(readable), []byte("\n"))
	if len(lines) != len(events) {
		t.Fatalf("readable projection has %d lines, want %d: %s", len(lines), len(events), readable)
	}

	if err = ReplayRaw(ctx, store, root, subjectID); err != nil {
		t.Fatal(err)
	}
	replayed, err := store.ListEvents(ctx, subjectID, 10, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed) != len(events) {
		t.Fatalf("replay created duplicate events: %d -> %d", len(events), len(replayed))
	}
}

func TestProjectionIdentifiesMainAndDomainSubagents(t *testing.T) {
	ctx := context.Background()
	store, subjectID, runtimeID := projectionStore(t)
	logger := New(store, t.TempDir(), subjectID, runtimeID)
	now := time.Now().UTC()
	main := json.RawMessage(`{"type":"assistant","message":{"content":[{"type":"text","text":"main"}]}}`)
	sub := json.RawMessage(`{"type":"assistant","subagent_type":"norns:explorer-for-math","message":{"content":[{"type":"text","text":"sub"}]}}`)
	if _, err := logger.Record(ctx, "stdout", "agent_message", main, now); err != nil {
		t.Fatal(err)
	}
	if _, err := logger.Record(ctx, "stdout", "agent_message", sub, now); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListEvents(ctx, subjectID, 10, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events=%d, want 2", len(events))
	}
	for index, want := range []map[string]string{{"role": "main", "name": "Coordinator"}, {"role": "sub", "name": "Explorer-For-Math", "source": "norns:explorer-for-math"}} {
		var payload struct {
			Agent map[string]string `json:"agent"`
		}
		if err = json.Unmarshal(events[index].Payload, &payload); err != nil {
			t.Fatal(err)
		}
		for key, value := range want {
			if payload.Agent[key] != value {
				t.Fatalf("event %d agent[%s]=%q, want %q", index, key, payload.Agent[key], value)
			}
		}
	}
}

func TestProjectionPreservesResultDiagnostics(t *testing.T) {
	ctx := context.Background()
	store, subjectID, runtimeID := projectionStore(t)
	logger := New(store, t.TempDir(), subjectID, runtimeID)
	raw := json.RawMessage(`{"type":"result","subtype":"error_during_execution","session_id":"runtime-session","is_error":true,"num_turns":2,"usage":{"input_tokens":12,"output_tokens":4},"result":"interrupted","stop_reason":"stop_sequence","terminal_reason":"user_interrupt","api_error_status":529}`)

	message, err := logger.Record(ctx, "stdout", "agent_message", raw, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !message.IsResult || !message.IsError {
		t.Fatalf("message result=%#v", message)
	}
	events, err := store.ListEvents(ctx, subjectID, 10, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "agent.result" {
		t.Fatalf("events=%#v", events)
	}
	var payload struct {
		Subtype        string `json:"subtype"`
		SessionID      string `json:"session_id"`
		IsError        bool   `json:"is_error"`
		NumTurns       int    `json:"num_turns"`
		Result         string `json:"result"`
		StopReason     string `json:"stop_reason"`
		TerminalReason string `json:"terminal_reason"`
		APIErrorStatus int    `json:"api_error_status"`
	}
	if err = json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Subtype != "error_during_execution" || payload.SessionID != "runtime-session" || !payload.IsError || payload.NumTurns != 2 || payload.Result != "interrupted" || payload.StopReason != "stop_sequence" || payload.TerminalReason != "user_interrupt" || payload.APIErrorStatus != 529 {
		t.Fatalf("result payload=%s", events[0].Payload)
	}
}

func TestReplayRefreshesExistingProjectionMetadata(t *testing.T) {
	ctx := context.Background()
	store, subjectID, runtimeID := projectionStore(t)
	root := t.TempDir()
	logger := New(store, root, subjectID, runtimeID)
	now := time.Now().UTC()
	raw := json.RawMessage(`{"type":"assistant","subagent_type":"norns:explorer-for-math","message":{"content":[{"type":"text","text":"sub"}]}}`)
	if _, err := logger.Record(ctx, "stdout", "agent_message", raw, now); err != nil {
		t.Fatal(err)
	}
	_, changed, err := store.ProjectRaw(ctx, subjectID, runtimeID, 1, 1, now, func(uint64) []storesqlite.ProjectedEvent {
		return []storesqlite.ProjectedEvent{{Type: "agent.text", Source: "agent", Payload: map[string]any{"text": "sub"}}}
	})
	if err != nil || !changed {
		t.Fatalf("strip projection metadata: changed=%v err=%v", changed, err)
	}
	if err = ReplayRaw(ctx, store, root, subjectID); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListEvents(ctx, subjectID, 10, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Agent map[string]string `json:"agent"`
	}
	if len(events) != 1 || json.Unmarshal(events[0].Payload, &payload) != nil || payload.Agent["name"] != "Explorer-For-Math" {
		t.Fatalf("projection was not refreshed: %#v", events)
	}
	readable, err := os.ReadFile(filepath.Join(root, ".nornsd", "log", "readable.jsonl"))
	if err != nil || !bytes.Contains(readable, []byte(`"name":"Explorer-For-Math"`)) {
		t.Fatalf("readable projection was not refreshed: %v %s", err, readable)
	}
}

func TestZeroEventRawAdvancesCheckpointAndReplayTruncatesPartialTail(t *testing.T) {
	ctx := context.Background()
	store, subjectID, runtimeID := projectionStore(t)
	root := t.TempDir()
	logger := New(store, root, subjectID, runtimeID)
	if _, err := logger.Record(ctx, "stdin", "agent_message", json.RawMessage(`{"type":"control_request"}`), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	rawPath := filepath.Join(root, ".nornsd", "log", "raw.jsonl")
	info, err := os.Stat(rawPath)
	if err != nil {
		t.Fatal(err)
	}
	completeSize := info.Size()
	file, err := os.OpenFile(rawPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString(`{"raw_seq":2`); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if err = ReplayRaw(ctx, store, root, subjectID); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(rawPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != completeSize {
		t.Fatalf("partial tail was not truncated: got %d want %d", info.Size(), completeSize)
	}
	subject, err := store.GetSubject(ctx, subjectID)
	if err != nil {
		t.Fatal(err)
	}
	if subject.LastProjectedRawSeq != 1 {
		t.Fatalf("checkpoint=%d, want 1", subject.LastProjectedRawSeq)
	}
}

func projectionStore(t *testing.T) (*storesqlite.Store, string, string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	store, _, err := storesqlite.Open(ctx, filepath.Join(t.TempDir(), "nornsd.sqlite"), uuid.NewString(), now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	subjectID, runtimeID := uuid.NewString(), uuid.NewString()
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}
	subject := domain.Subject{ID: subjectID, DirectoryName: now.Format("20060102T150405Z") + "_" + subjectID, Profile: profile, SessionID: subjectID}
	instruction := "test"
	if err = store.CreateSubject(ctx, subject, &instruction, runtimeID, now); err != nil {
		t.Fatal(err)
	}
	return store, subjectID, runtimeID
}
