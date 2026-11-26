package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/config"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	storesqlite "github.com/Ghr07h/norns/nornsd/internal/store/sqlite"
	"github.com/google/uuid"
)

func TestQueueExportRollbackRestartAndCrashGap(t *testing.T) {
	ctx := context.Background()
	owner, err := root.Acquire(ctx, t.TempDir(), "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err = config.SaveTemplates(owner.Control, []config.Config{{LLMModel: "old-model", LLMKey: "old-secret"}}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(ctx, Options{Owner: owner, Concurrency: 1})
	defer manager.Close()
	if err = manager.ExportQueued(ctx, "empty"); err != nil {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(filepath.Join(owner.Control, "queue_record"))
	if len(files) != 0 {
		t.Fatal("empty queue exported")
	}
	ids := []string{}
	rids := []string{}
	for i := 0; i < 2; i++ {
		now := time.Now().UTC()
		id, rid := uuid.NewString(), uuid.NewString()
		ids = append(ids, id)
		rids = append(rids, rid)
		subject := domain.Subject{ID: id, DirectoryName: root.DirectoryName(now, id), WorkspaceState: domain.WorkspaceMaterializing, CreatedAt: now}
		instruction := "first"
		if err = owner.Store.CreateSubject(ctx, subject, &instruction, rid, now, json.RawMessage(`{"AGENT_LLM_MODEL":"override"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err = owner.Store.SubmitInstruction(ctx, id, uuid.NewString(), "second", false, now); err != nil {
			t.Fatal(err)
		}
	}
	failure := errors.New("disk failure")
	if err = owner.Store.ExportQueued(ctx, "failure", func(records []storesqlite.QueuedRecord) error {
		if len(records) != 2 {
			t.Fatal(len(records))
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	for i, id := range ids {
		subject, _ := owner.Store.GetSubject(ctx, id)
		r, _ := owner.Store.GetRuntime(ctx, id, rids[i], true)
		if subject.State != domain.SubjectQueuing || r.State != domain.RuntimeQueued || len(r.Instructions) != 2 {
			t.Fatal("failed export changed queue")
		}
	}
	// Publish a complete export but roll back cancellation: simulate the crash gap.
	var export queueExport
	if err = owner.Store.ExportQueued(ctx, "crash", func(records []storesqlite.QueuedRecord) error {
		export = queueExport{RootID: owner.Identity.RootID, Templates: map[string]string{"nornsd.yaml": "old template"}, Records: records}
		templateBytes, _ := os.ReadFile(filepath.Join(owner.Control, "configs", "nornsd.yaml"))
		export.Templates["nornsd.yaml"] = string(templateBytes)
		export.ID, _ = queueExportID(export.RootID, export.Templates, records)
		data, _ := json.Marshal(export)
		if err := os.WriteFile(filepath.Join(owner.Control, "queue_record", "existing.json"), data, 0600); err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err = manager.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	files, _ = os.ReadDir(filepath.Join(owner.Control, "queue_record"))
	if len(files) != 1 {
		t.Fatalf("duplicate export after crash gap: %v", files)
	}
	for i, id := range ids {
		subject, _ := owner.Store.GetSubject(ctx, id)
		r, _ := owner.Store.GetRuntime(ctx, id, rids[i], true)
		if subject.State != domain.SubjectIdle || subject.CurrentRuntimeID != nil || r.State != domain.RuntimeCancelled || len(r.Instructions) != 2 || r.EndReason == nil || *r.EndReason != "server restart" {
			t.Fatalf("not preserved/cancelled: %+v %+v", subject, r)
		}
	}
	if export.Records[0].RuntimeID != rids[0] || export.Records[1].RuntimeID != rids[1] || export.Records[0].Profile != nil || string(export.Records[0].Config) != `{"AGENT_LLM_MODEL":"override"}` {
		t.Fatalf("lost ordering/overrides: %+v", export)
	}
	if err = manager.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	files, _ = os.ReadDir(filepath.Join(owner.Control, "queue_record"))
	if len(files) != 1 {
		t.Fatal("repeated recovery emitted export")
	}
}

func TestQueueExportPublicationFailurePreservesQueue(t *testing.T) {
	ctx := context.Background()
	owner, err := root.Acquire(ctx, t.TempDir(), "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	now := time.Now()
	id, rid := uuid.NewString(), uuid.NewString()
	instruction := "keep me"
	if err = owner.Store.CreateSubject(ctx, domain.Subject{ID: id, DirectoryName: root.DirectoryName(now, id), CreatedAt: now}, &instruction, rid, now); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(owner.Control, "queue_record")
	if err = os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(dir, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(ctx, Options{Owner: owner, Concurrency: 1})
	if err = manager.Recover(ctx); err == nil {
		t.Fatal("recovery proceeded after export failed")
	}
	r, err := owner.Store.GetRuntime(ctx, id, rid, true)
	if err != nil || r.State != domain.RuntimeQueued || len(r.Instructions) != 1 {
		t.Fatalf("queue lost: %+v %v", r, err)
	}
	manager.Close()
}
