package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/version"
	"github.com/google/uuid"
)

func TestInstructionFIFOAcrossDispatchesAndPendingLimit(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	store, _, err := Open(ctx, filepath.Join(t.TempDir(), "nornsd.sqlite"), uuid.NewString(), now)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sid, rid := uuid.NewString(), uuid.NewString()
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}
	subject := domain.Subject{ID: sid, DirectoryName: now.Format("20060102T150405Z") + "_" + sid, Profile: profile, SessionID: sid}
	first := "instruction-1"
	if err = store.CreateSubject(ctx, subject, &first, rid, now); err != nil {
		t.Fatal(err)
	}
	for i := 2; i <= version.MaxPendingCount; i++ {
		if _, err = store.SubmitInstruction(ctx, sid, uuid.NewString(), "instruction", profile, false, now.Add(time.Duration(i)*time.Millisecond)); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	if _, err = store.SubmitInstruction(ctx, sid, uuid.NewString(), "overflow", profile, false, now); err == nil || domain.AsError(err).Code != "RUNTIME_INSTRUCTION_REJECTED" {
		t.Fatalf("expected pending limit, got %v", err)
	}
	_, runtime, found, err := store.ClaimNext(ctx, 1, now)
	if err != nil || !found {
		t.Fatalf("claim: %v found=%v", err, found)
	}
	items, group, err := store.DispatchPending(ctx, sid, rid, true, now)
	if err != nil || len(items) != version.MaxPendingCount {
		t.Fatalf("dispatch: count=%d err=%v", len(items), err)
	}
	if err = store.ConfirmDispatch(ctx, sid, rid, group, now); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompleteTurn(ctx, sid, rid, nil, now); err != nil {
		t.Fatal(err)
	}
	later, err := store.SubmitInstruction(ctx, sid, uuid.NewString(), "later", profile, false, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(later.Instructions) != 1 || later.Instructions[0].Sequence != 17 {
		t.Fatalf("unexpected later instruction: %#v", later.Instructions)
	}
	events, err := store.ListEvents(ctx, sid, 100, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.Type == "instruction.dispatched" {
			count++
			var payload map[string]any
			if json.Unmarshal(event.Payload, &payload) != nil || payload["content"] == nil {
				t.Fatalf("missing instruction payload: %s", event.Payload)
			}
		}
	}
	if count != version.MaxPendingCount {
		t.Fatalf("got %d dispatched events", count)
	}
	_ = runtime
}

func TestUnconfirmedDispatchBecomesDeliveryUncertain(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	store, _, err := Open(ctx, filepath.Join(t.TempDir(), "nornsd.sqlite"), uuid.NewString(), now)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sid, rid := uuid.NewString(), uuid.NewString()
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}
	subject := domain.Subject{ID: sid, DirectoryName: now.Format("20060102T150405Z") + "_" + sid, Profile: profile, SessionID: sid}
	instruction := "uncertain"
	if err = store.CreateSubject(ctx, subject, &instruction, rid, now); err != nil {
		t.Fatal(err)
	}
	if _, _, found, claimErr := store.ClaimNext(ctx, 1, now); claimErr != nil || !found {
		t.Fatalf("claim: found=%v err=%v", found, claimErr)
	}
	items, _, err := store.DispatchPending(ctx, sid, rid, true, now)
	if err != nil || len(items) != 1 {
		t.Fatalf("dispatch: %#v err=%v", items, err)
	}
	count, err := store.MarkUncertainDeliveries(ctx, sid, rid, now.Add(time.Second))
	if err != nil || count != 1 {
		t.Fatalf("mark uncertain: count=%d err=%v", count, err)
	}
	items, _, err = store.DispatchPending(ctx, sid, rid, true, now.Add(2*time.Second))
	if err != nil || len(items) != 0 {
		t.Fatalf("uncertain instruction was requeued: %#v err=%v", items, err)
	}
	events, err := store.ListEvents(ctx, sid, 10, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.Type == "diagnostic" && strings.Contains(string(event.Payload), "DELIVERY_UNCERTAIN") {
			found = true
		}
	}
	if !found {
		t.Fatalf("delivery uncertainty diagnostic is missing: %#v", events)
	}
}

func TestNewRuntimeNeverResumesSubjectSession(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	store, _, err := Open(ctx, filepath.Join(t.TempDir(), "nornsd.sqlite"), uuid.NewString(), now)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	subjectID := uuid.NewString()
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}
	subject := domain.Subject{ID: subjectID, DirectoryName: now.Format("20060102T150405Z") + "_" + subjectID, Profile: profile, SessionID: subjectID, SessionInitialized: true}
	if err = store.CreateSubject(ctx, subject, nil, "", now); err != nil {
		t.Fatal(err)
	}
	runtimeID := uuid.NewString()
	runtime, err := store.CreateDirectRuntime(ctx, subjectID, runtimeID, now)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.SessionMode != "new" {
		t.Fatalf("runtime session mode = %q, want new", runtime.SessionMode)
	}
}

func TestResumeSessionForksLatestRuntimeSession(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	store, _, err := Open(ctx, filepath.Join(t.TempDir(), "nornsd.sqlite"), uuid.NewString(), now)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	subjectID := uuid.NewString()
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}
	subject := domain.Subject{ID: subjectID, DirectoryName: now.Format("20060102T150405Z") + "_" + subjectID, Profile: profile, SessionID: subjectID}
	if err = store.CreateSubject(ctx, subject, nil, "", now); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.CreateDirectRuntimeWithSession(ctx, subjectID, uuid.NewString(), true, now); err == nil || domain.AsError(err).Code != "RUNTIME_SESSION_NOT_FOUND" {
		t.Fatalf("resume without history error = %v", err)
	}
	firstID := uuid.NewString()
	if _, err = store.CreateDirectRuntime(ctx, subjectID, firstID, now); err != nil {
		t.Fatal(err)
	}
	if err = store.SetRuntimeTerminal(ctx, subjectID, firstID, domain.RuntimeEnded, "completed", "", now.Add(time.Second), true); err != nil {
		t.Fatal(err)
	}
	secondID := uuid.NewString()
	second, resumedID, err := store.CreateDirectRuntimeWithSession(ctx, subjectID, secondID, true, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if second.SessionMode != "resume" || resumedID != firstID {
		t.Fatalf("resumed Runtime mode=%q source=%q, want resume from %q", second.SessionMode, resumedID, firstID)
	}
	var processInfo struct {
		SessionID       string `json:"session_id"`
		ResumeSessionID string `json:"resume_session_id"`
	}
	if err = json.Unmarshal(second.ProcessJSON, &processInfo); err != nil || processInfo.SessionID != secondID || processInfo.ResumeSessionID != firstID {
		t.Fatalf("process_json=%s parsed=%#v err=%v", second.ProcessJSON, processInfo, err)
	}
	if err = store.SetRuntimeTerminal(ctx, subjectID, secondID, domain.RuntimeEnded, "completed", "", now.Add(3*time.Second), true); err != nil {
		t.Fatal(err)
	}
	thirdID := uuid.NewString()
	_, resumedID, err = store.CreateDirectRuntimeWithSession(ctx, subjectID, thirdID, true, now.Add(4*time.Second))
	if err != nil || resumedID != secondID {
		t.Fatalf("chained resume source=%q, want latest fork %q: %v", resumedID, secondID, err)
	}
}

func TestRuntimeErrorPersistsDetails(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	store, _, err := Open(ctx, filepath.Join(t.TempDir(), "nornsd.sqlite"), uuid.NewString(), now)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	subjectID := uuid.NewString()
	runtimeID := uuid.NewString()
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}
	subject := domain.Subject{ID: subjectID, DirectoryName: now.Format("20060102T150405Z") + "_" + subjectID, Profile: profile, SessionID: subjectID}
	if err = store.CreateSubject(ctx, subject, nil, "", now); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateDirectRuntime(ctx, subjectID, runtimeID, now); err != nil {
		t.Fatal(err)
	}
	details := map[string]any{"message": "agent exited during initialize", "phase": "initialize"}
	if err = store.SetRuntimeTerminalWithDetails(ctx, subjectID, runtimeID, domain.RuntimeError, "runtime failed", "AGENT_PROTOCOL_ERROR", details, now.Add(time.Second), true); err != nil {
		t.Fatal(err)
	}
	runtime, err := store.GetRuntime(ctx, subjectID, runtimeID, false)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err = json.Unmarshal(runtime.ErrorJSON, &stored); err != nil {
		t.Fatalf("error_json = %q: %v", runtime.ErrorJSON, err)
	}
	if stored["message"] != details["message"] || stored["phase"] != details["phase"] {
		t.Fatalf("error_json = %#v, want %#v", stored, details)
	}
}

func TestRuntimeErrorDefaultsDetailsToEndReason(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	store, _, err := Open(ctx, filepath.Join(t.TempDir(), "nornsd.sqlite"), uuid.NewString(), now)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	subjectID := uuid.NewString()
	runtimeID := uuid.NewString()
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}
	subject := domain.Subject{ID: subjectID, DirectoryName: now.Format("20060102T150405Z") + "_" + subjectID, Profile: profile, SessionID: subjectID}
	if err = store.CreateSubject(ctx, subject, nil, "", now); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateDirectRuntime(ctx, subjectID, runtimeID, now); err != nil {
		t.Fatal(err)
	}
	if err = store.SetRuntimeTerminal(ctx, subjectID, runtimeID, domain.RuntimeError, "Norns close grace period expired", "CLOSE_GRACE_TIMEOUT", now.Add(time.Second), true); err != nil {
		t.Fatal(err)
	}
	runtime, err := store.GetRuntime(ctx, subjectID, runtimeID, false)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err = json.Unmarshal(runtime.ErrorJSON, &stored); err != nil || stored["message"] != "Norns close grace period expired" {
		t.Fatalf("error_json = %q (%#v): %v", runtime.ErrorJSON, stored, err)
	}
}

func TestHTTPReadProjections(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	store, _, err := Open(ctx, filepath.Join(t.TempDir(), "nornsd.sqlite"), uuid.NewString(), now)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}
	firstID, secondID := uuid.NewString(), uuid.NewString()
	firstRuntimeID, secondRuntimeID := uuid.NewString(), uuid.NewString()
	firstName, secondName := "first", "second"
	first := domain.Subject{ID: firstID, Name: &firstName, DirectoryName: now.Format("20060102T150405Z") + "_" + firstID, Profile: profile, SessionID: firstID}
	secondTime := now.Add(time.Second)
	second := domain.Subject{ID: secondID, Name: &secondName, DirectoryName: secondTime.Format("20060102T150405Z") + "_" + secondID, Profile: profile, SessionID: secondID}
	instruction := "first instruction"
	if err = store.CreateSubject(ctx, first, &instruction, firstRuntimeID, now); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SubmitInstruction(ctx, firstID, uuid.NewString(), "second instruction", profile, false, now); err != nil {
		t.Fatal(err)
	}
	if err = store.CreateSubject(ctx, second, &instruction, secondRuntimeID, secondTime); err != nil {
		t.Fatal(err)
	}

	subjects, err := store.ListSubjectSummaries(ctx, []domain.SubjectState{domain.SubjectQueuing}, 10, "", "")
	if err != nil || len(subjects) != 2 {
		t.Fatalf("subject summaries: len=%d err=%v", len(subjects), err)
	}
	if subjects[0].ID != secondID || subjects[0].Name == nil || *subjects[0].Name != secondName {
		t.Fatalf("unexpected subject order or projection: %#v", subjects)
	}

	queue, err := store.ListQueueEntries(ctx, 10, 0)
	if err != nil || len(queue) != 2 {
		t.Fatalf("queue entries: len=%d err=%v", len(queue), err)
	}
	if queue[0].SubjectID != firstID || queue[0].Position != 1 || queue[0].InstructionCount != 2 || queue[1].Position != 2 {
		t.Fatalf("unexpected queue projection: %#v", queue)
	}
	next, err := store.ListQueueEntries(ctx, 10, queue[0].Sequence)
	if err != nil || len(next) != 1 || next[0].SubjectID != secondID || next[0].Position != 2 {
		t.Fatalf("queue position was not global across pages: %#v err=%v", next, err)
	}

	runtimes, err := store.ListRuntimeSummaries(ctx, firstID, 10, "", "")
	if err != nil || len(runtimes) != 1 || runtimes[0].ID != firstRuntimeID || !runtimes[0].Current || runtimes[0].TotalCount != 1 {
		t.Fatalf("runtime summaries: %#v err=%v", runtimes, err)
	}
	current, position, err := store.GetCurrentRuntime(ctx, firstID)
	if err != nil || current == nil || current.ID != firstRuntimeID || position == nil || *position != 1 || len(current.Instructions) != 2 {
		t.Fatalf("current runtime: runtime=%#v position=%v err=%v", current, position, err)
	}
	allSubjects, total, err := store.ListSubjects(ctx, nil, 10, "", "")
	if err != nil || total != 2 || len(allSubjects) != 2 || allSubjects[0].ID != secondID || allSubjects[0].Profile.Agent != profile.Agent {
		t.Fatalf("full subjects: total=%d subjects=%#v err=%v", total, allSubjects, err)
	}
	currentRuntimes, err := store.CurrentRuntimes(ctx)
	if err != nil || len(currentRuntimes) != 2 || currentRuntimes[0].ID != firstRuntimeID || len(currentRuntimes[0].Instructions) != 2 {
		t.Fatalf("current runtimes: %#v err=%v", currentRuntimes, err)
	}
	if _, queued, err := store.RequestStop(ctx, firstID, "test_stop", now.Add(2*time.Second)); err != nil || !queued {
		t.Fatalf("stop queued runtime: queued=%v err=%v", queued, err)
	}
	runtimes, err = store.ListRuntimeSummaries(ctx, firstID, 10, "", "")
	if err != nil || len(runtimes) != 1 || runtimes[0].Current || runtimes[0].EndedAt == nil {
		t.Fatalf("idle subject runtime summaries: %#v err=%v", runtimes, err)
	}
	current, position, err = store.GetCurrentRuntime(ctx, firstID)
	if err != nil || current != nil || position != nil {
		t.Fatalf("idle current runtime: runtime=%#v position=%v err=%v", current, position, err)
	}
	if _, _, err = store.GetCurrentRuntime(ctx, uuid.NewString()); err == nil || domain.AsError(err).Code != "SUBJECT_NOT_FOUND" {
		t.Fatalf("expected missing current-runtime subject error, got %v", err)
	}
	if _, err = store.ListRuntimeSummaries(ctx, uuid.NewString(), 10, "", ""); err == nil || domain.AsError(err).Code != "SUBJECT_NOT_FOUND" {
		t.Fatalf("expected missing subject error, got %v", err)
	}
}
