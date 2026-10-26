package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	"github.com/Ghr07h/norns/nornsd/internal/version"
	"github.com/google/uuid"
)

func TestRecoverSubjectMaterializationAndIdentityBoundaries(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	owner, err := root.Acquire(ctx, filepath.Join(base, "root"), "test", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}

	materializing := newRecoverySubject(profile)
	if err = owner.Store.CreateSubject(ctx, materializing, nil, "", materializing.CreatedAt); err != nil {
		t.Fatal(err)
	}

	missing := newRecoverySubject(profile)
	if err = owner.Store.CreateSubject(ctx, missing, nil, "", missing.CreatedAt); err != nil {
		t.Fatal(err)
	}
	missingPath, err := owner.MaterializeSubject(ctx, missing)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(missingPath, filepath.Join(base, "missing-backup")); err != nil {
		t.Fatal(err)
	}

	mismatch := newRecoverySubject(profile)
	if err = owner.Store.CreateSubject(ctx, mismatch, nil, "", mismatch.CreatedAt); err != nil {
		t.Fatal(err)
	}
	mismatchPath, err := owner.MaterializeSubject(ctx, mismatch)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := root.ReadSubjectIdentity(mismatchPath)
	if err != nil {
		t.Fatal(err)
	}
	identity.RuntimeProfile.Capabilities = []string{"norns"}
	writeTestJSON(t, filepath.Join(mismatchPath, ".nornsd", "subject.json"), identity)

	orphan := newRecoverySubject(profile)
	orphanPath := filepath.Join(owner.Path, orphan.DirectoryName)
	if err = os.MkdirAll(filepath.Join(orphanPath, ".nornsd", "log"), 0700); err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, filepath.Join(orphanPath, ".nornsd", "subject.json"), root.SubjectIdentity{FormatVersion: 1, SubjectID: orphan.ID, DirectoryName: orphan.DirectoryName, RuntimeProfile: orphan.Profile, SessionID: orphan.SessionID, CreatedAt: orphan.CreatedAt})
	for _, name := range []string{"raw.jsonl", "readable.jsonl"} {
		if err = os.WriteFile(filepath.Join(orphanPath, ".nornsd", "log", name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}

	manager := NewManager(ctx, Options{Owner: owner, Profile: profile, Concurrency: 1})
	if err = manager.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	manager.Start()
	defer manager.Close()

	assertWorkspaceState(t, owner, materializing.ID, domain.WorkspaceMaterialized)
	materializedSubject, err := owner.Store.GetSubject(ctx, materializing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.MaterializeSubject(ctx, materializedSubject); err != nil {
		t.Fatalf("materialization is not idempotent: %v", err)
	}
	events, err := owner.Store.ListEvents(ctx, materializing.ID, 10, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	createdEvents := 0
	for _, event := range events {
		if event.Type == "subject.created" {
			createdEvents++
		}
	}
	if createdEvents != 1 {
		t.Fatalf("subject.created count=%d, want 1", createdEvents)
	}
	assertWorkspaceState(t, owner, missing.ID, domain.WorkspaceUnavailable)
	assertWorkspaceState(t, owner, mismatch.ID, domain.WorkspaceUnavailable)
	recovered, err := owner.Store.GetSubject(ctx, orphan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State != domain.SubjectIdle || recovered.WorkspaceState != domain.WorkspaceMaterialized || recovered.DirectoryName != orphan.DirectoryName {
		t.Fatalf("unexpected recovered orphan: %#v", recovered)
	}
}

func TestRecoveredSessionIDUsesPersistedRuntimeIdentity(t *testing.T) {
	runtimeID := uuid.NewString()
	legacySessionID := uuid.NewString()
	current := domain.Runtime{ID: runtimeID, ProcessJSON: json.RawMessage(`{"session_id":"` + runtimeID + `"}`)}
	if got := recoveredSessionID(current, legacySessionID); got != runtimeID {
		t.Fatalf("recoveredSessionID() = %q, want Runtime ID %q", got, runtimeID)
	}
	legacy := domain.Runtime{ID: uuid.NewString(), ProcessJSON: json.RawMessage(`{"identity":{}}`)}
	if got := recoveredSessionID(legacy, legacySessionID); got != legacySessionID {
		t.Fatalf("legacy recoveredSessionID() = %q, want Subject session %q", got, legacySessionID)
	}
}

func TestRecoverBlocksCurrentRuntimeWithoutVerifiableProcessIdentity(t *testing.T) {
	ctx := context.Background()
	owner, err := root.Acquire(ctx, t.TempDir(), "test", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}
	subject := newRecoverySubject(profile)
	if err = owner.Store.CreateSubject(ctx, subject, nil, "", subject.CreatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.MaterializeSubject(ctx, subject); err != nil {
		t.Fatal(err)
	}
	runtimeID := uuid.NewString()
	if _, err = owner.Store.CreateDirectRuntime(ctx, subject.ID, runtimeID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(ctx, Options{Owner: owner, Profile: profile, Concurrency: 1})
	if err = manager.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	manager.Start()
	defer manager.Close()
	recoveredSubject, err := owner.Store.GetSubject(ctx, subject.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recoveredSubject.State != domain.SubjectUsing || recoveredSubject.CurrentRuntimeID == nil || *recoveredSubject.CurrentRuntimeID != runtimeID {
		t.Fatalf("blocked runtime was incorrectly finalized: %#v", recoveredSubject)
	}
	recoveredRuntime, err := owner.Store.GetRuntime(ctx, subject.ID, runtimeID, false)
	if err != nil {
		t.Fatal(err)
	}
	if recoveredRuntime.State != domain.RuntimeError || recoveredRuntime.ErrorCode == nil || *recoveredRuntime.ErrorCode != "SERVER_INTERRUPTED" {
		t.Fatalf("unexpected blocked runtime: %#v", recoveredRuntime)
	}
	if !json.Valid(recoveredRuntime.ProcessJSON) || !containsJSONText(recoveredRuntime.ProcessJSON, "blocked") {
		t.Fatalf("recovery block marker is missing: %s", recoveredRuntime.ProcessJSON)
	}
}

func newRecoverySubject(profile domain.RuntimeProfile) domain.Subject {
	now := time.Now().UTC()
	id := uuid.NewString()
	return domain.Subject{ID: id, DirectoryName: root.DirectoryName(now, id), WorkspaceState: domain.WorkspaceMaterializing, Profile: profile, SessionID: id, CreatedAt: now, UpdatedAt: now}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func assertWorkspaceState(t *testing.T, owner *root.Owner, subjectID, want string) {
	t.Helper()
	subject, err := owner.Store.GetSubject(context.Background(), subjectID)
	if err != nil {
		t.Fatal(err)
	}
	if subject.WorkspaceState != want {
		t.Fatalf("subject %s workspace state=%s, want %s", subjectID, subject.WorkspaceState, want)
	}
}

func containsJSONText(data []byte, text string) bool {
	return len(data) > 0 && string(data) != "null" && bytes.Contains(data, []byte(text))
}
