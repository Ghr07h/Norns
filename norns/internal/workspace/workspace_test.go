package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ghr07h/norns/norns/internal/domain"
)

func TestResolveUsesLifecyclePathsOutsideNornsDirectory(t *testing.T) {
	root := t.TempDir()
	paths, err := Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if paths.Database != filepath.Join(root, ".norns", "norns.sqlite") || paths.Lock != filepath.Join(root, ".norns.lock") || paths.ResetLog != filepath.Join(root, ".norns-reset.json") {
		t.Fatalf("unexpected Workspace paths: %#v", paths)
	}
}

func TestLifecycleLockAllowsSharedAndExcludesExclusive(t *testing.T) {
	paths, err := Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := NewLock(paths)
	second := NewLock(paths)
	unlockFirst, err := first.Shared(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	unlockSecond, err := second.Shared(context.Background())
	if err != nil {
		t.Fatalf("second shared lock was rejected: %v", err)
	}
	unlockSecond()

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if _, err := second.Exclusive(ctx); errorCode(err) != "WORKSPACE_BUSY" {
		t.Fatalf("exclusive lock should wait for readers, got %v", err)
	} else {
		var typed *domain.Error
		if !errors.As(err, &typed) || typed.Details["resource"] != "workspace_lifecycle" {
			t.Fatalf("busy error has wrong details: %#v", err)
		}
	}

	unlockFirst()
	unlockExclusive, err := second.Exclusive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlockExclusive()
	ctx, cancel = context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if _, err := first.Shared(ctx); errorCode(err) != "WORKSPACE_BUSY" {
		t.Fatalf("shared lock should wait for exclusive holder, got %v", err)
	}
}

func errorCode(err error) string {
	var typed *domain.Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return ""
}
