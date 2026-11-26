package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/eventlog"
	"github.com/Ghr07h/norns/nornsd/internal/process"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	"github.com/google/uuid"
)

func TestRuntimeRemainsBusyUntilBackgroundWritersFinish(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	owner, err := root.Acquire(ctx, t.TempDir(), "test", now)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(ctx, Options{Owner: owner, Concurrency: 1})
	defer func() { manager.Close(); _ = owner.Close() }()
	id := uuid.NewString()
	subject := domain.Subject{ID: id, DirectoryName: root.DirectoryName(now, id), CreatedAt: now, UpdatedAt: now}
	if err := owner.Store.CreateSubject(ctx, subject, nil, "", now); err != nil {
		t.Fatal(err)
	}
	path, err := owner.MaterializeSubject(ctx, subject)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := owner.Store.CreateDirectRuntime(ctx, subject.ID, "runtime-test", now)
	if err != nil {
		t.Fatal(err)
	}
	mr := &managedRuntime{subject: subject, runtime: runtime, path: path, slot: 1, done: make(chan struct{}), logger: eventlog.New(owner.Store, path, subject.ID, runtime.ID)}
	mr.workers.Add(1)
	finished := make(chan struct{})
	go func() { manager.finishRuntime(mr, process.Result{}, nil); close(finished) }()
	select {
	case <-finished:
		t.Fatal("Runtime finalized while a background writer was still active")
	case <-time.After(20 * time.Millisecond):
	}
	current, err := owner.Store.GetSubject(ctx, subject.ID)
	if err != nil || current.State != domain.SubjectUsing || current.CurrentRuntimeID == nil {
		t.Fatalf("Subject became idle before writers finished: %#v, %v", current, err)
	}
	mr.workers.Done()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("Runtime did not finalize after writers finished")
	}
	current, err = owner.Store.GetSubject(ctx, subject.ID)
	if err != nil || current.State != domain.SubjectIdle || current.CurrentRuntimeID != nil {
		t.Fatalf("Subject not idle after finalization: %#v, %v", current, err)
	}
}
