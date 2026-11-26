package root

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/google/uuid"
)

func TestAcquireRejectsMissingDatabaseForExistingRoot(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir()
	owner, err := Acquire(ctx, path, "test", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(path, ".nornsd", "service", "nornsd.sqlite")); err != nil {
		t.Fatal(err)
	}
	if _, err = Acquire(ctx, path, "test", time.Now().UTC()); err == nil || domain.AsError(err).Code != "UNSUPPORTED_DATABASE" {
		t.Fatalf("expected unsupported database, got %v", err)
	}
}

func TestAcquireRejectsOrphanSubjectWhenControlStateIsAbsent(t *testing.T) {
	path := t.TempDir()
	subject := filepath.Join(path, ".nornsd", "subjects", "subject")
	if err := os.MkdirAll(subject, 0700); err != nil {
		t.Fatal(err)
	}
	identity := SubjectIdentity{FormatVersion: 2, SubjectID: uuid.NewString(), DirectoryName: "subject", CreatedAt: time.Now().UTC()}
	b, _ := json.Marshal(identity)
	if err := os.WriteFile(filepath.Join(subject, "subject.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(context.Background(), path, "test", time.Now().UTC()); err == nil || domain.AsError(err).Code != "UNSUPPORTED_DATABASE" {
		t.Fatalf("expected unsupported database, got %v", err)
	}
}

func TestLayoutIsolationLockAndSubjectSymlinkRejection(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	base := t.TempDir()
	owner, err := Acquire(ctx, base, "test", now)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if other, err := Acquire(ctx, base, "test", now); domain.AsError(err).Code != "ROOT_IN_USE" {
		if other != nil {
			other.Close()
		}
		t.Fatalf("root lock lost: %v", err)
	}
	for _, name := range []string{"root.json", "owner.json", "nornsd_lock", "nornsd.sqlite"} {
		if _, err := os.Stat(filepath.Join(owner.Control, "service", name)); err != nil {
			t.Fatal(err)
		}
	}
	id := uuid.NewString()
	subject := domain.Subject{ID: id, DirectoryName: DirectoryName(now, id), CreatedAt: now, WorkspaceState: domain.WorkspaceMaterializing}
	if err := owner.Store.CreateSubject(ctx, subject, nil, "", now); err != nil {
		t.Fatal(err)
	}
	workspace, err := owner.MaterializeSubject(ctx, subject)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 0 {
		t.Fatalf("management files leaked into workspace: %v %v", entries, err)
	}
	if _, err := os.Stat(filepath.Join(owner.SubjectData(id), "subject.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(workspace); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), workspace); err != nil {
		t.Skip("symlink not available")
	}
	if _, err := owner.MaterializeSubject(ctx, subject); err == nil {
		t.Fatal("symlink workspace accepted")
	}
}

func TestLegacyLayoutCannotBootstrapNewIdentity(t *testing.T) {
	base := t.TempDir()
	control := filepath.Join(base, ".nornsd")
	if err := os.MkdirAll(control, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(control, "root.json"), []byte(`{"format_version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if owner, err := Acquire(context.Background(), base, "test", time.Now()); domain.AsError(err).Code != "UNSUPPORTED_DATABASE" {
		if owner != nil {
			owner.Close()
		}
		t.Fatalf("legacy root reused: %v", err)
	}
}
