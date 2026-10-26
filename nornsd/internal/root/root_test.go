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
	if err = os.Remove(filepath.Join(path, ".nornsd_root", "nornsd.sqlite")); err != nil {
		t.Fatal(err)
	}
	if _, err = Acquire(ctx, path, "test", time.Now().UTC()); err == nil || domain.AsError(err).Code != "UNSUPPORTED_DATABASE" {
		t.Fatalf("expected unsupported database, got %v", err)
	}
}

func TestAcquireRejectsOrphanSubjectWhenControlStateIsAbsent(t *testing.T) {
	path := t.TempDir()
	subject := filepath.Join(path, "subject", ".nornsd")
	if err := os.MkdirAll(subject, 0700); err != nil {
		t.Fatal(err)
	}
	identity := SubjectIdentity{FormatVersion: 1, SubjectID: uuid.NewString(), DirectoryName: "subject", SessionID: uuid.NewString(), CreatedAt: time.Now().UTC()}
	b, _ := json.Marshal(identity)
	if err := os.WriteFile(filepath.Join(subject, "subject.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(context.Background(), path, "test", time.Now().UTC()); err == nil || domain.AsError(err).Code != "UNSUPPORTED_DATABASE" {
		t.Fatalf("expected unsupported database, got %v", err)
	}
}
