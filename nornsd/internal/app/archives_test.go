package app

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	runtimemanager "github.com/Ghr07h/norns/nornsd/internal/runtime"
)

func TestArchiveAdmissionConflictsAndReleases(t *testing.T) {
	var a archiveAdmission
	first, err := a.beginArchive("subject-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, begin := range []func() (func(), error){func() (func(), error) { return a.beginArchive("subject-1") }, func() (func(), error) { return a.beginArchive("") }, func() (func(), error) { return a.beginWork("subject-1") }} {
		if release, err := begin(); domain.AsError(err).Code != "ARCHIVE_BUSY" {
			if release != nil {
				release()
			}
			t.Fatalf("expected archive conflict: %v", err)
		}
	}
	other, err := a.beginWork("subject-2")
	if err != nil {
		t.Fatal(err)
	}
	other()
	if release, err := a.beginArchive("subject-2"); domain.AsError(err).Code != "ARCHIVE_BUSY" {
		if release != nil {
			release()
		}
		t.Fatalf("different Subjects must share the archive quota: %v", err)
	}
	first()
	second, err := a.beginArchive("subject-2")
	if err != nil {
		t.Fatal(err)
	}
	second()
	all, err := a.beginArchive("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.beginArchive(""); domain.AsError(err).Code != "ARCHIVE_BUSY" {
		t.Fatal(err)
	}
	if _, err := a.beginWork("new-subject"); domain.AsError(err).Code != "ARCHIVE_BUSY" {
		t.Fatal(err)
	}
	if _, err := a.beginArchive("subject-2"); domain.AsError(err).Code != "ARCHIVE_BUSY" {
		t.Fatal(err)
	}
	all()
	write, err := a.beginWork("subject-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.beginArchive("subject-1"); domain.AsError(err).Code != "ARCHIVE_BUSY" {
		t.Fatal(err)
	}
	if _, err := a.beginArchive(""); domain.AsError(err).Code != "ARCHIVE_BUSY" {
		t.Fatal(err)
	}
	write()
}

func TestArchiveConcurrentSharedQuota(t *testing.T) {
	for _, kind := range []string{"root", "subjects", "mixed"} {
		t.Run(kind, func(t *testing.T) {
			var a archiveAdmission
			const callers = 16
			results := make(chan func(), callers)
			start := make(chan struct{})
			var done sync.WaitGroup
			for i := range callers {
				id := ""
				if kind == "subjects" || (kind == "mixed" && i%2 != 0) {
					id = fmt.Sprintf("subject-%d", i)
				}
				done.Add(1)
				go func() {
					defer done.Done()
					<-start
					release, _ := a.beginArchive(id)
					results <- release
				}()
			}
			close(start)
			done.Wait()
			close(results)
			accepted := 0
			for release := range results {
				if release != nil {
					accepted++
					release()
				}
			}
			if accepted != 1 {
				t.Fatalf("accepted %d concurrent archive builders", accepted)
			}
		})
	}
}

func TestArchivesRequireIdleAndBlockMutations(t *testing.T) {
	ctx := context.Background()
	owner, err := root.Acquire(ctx, t.TempDir(), "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	manager := runtimemanager.NewManager(ctx, runtimemanager.Options{Owner: owner, Concurrency: 1})
	defer func() { manager.Close(); _ = owner.Close() }()
	s := &Service{Owner: owner, Store: owner.Store, Manager: manager}
	subject, err := s.CreateSubject(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, all := range []bool{false, true} {
		if all {
			file, err := s.RootArchive(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_ = file.Close()
		} else {
			file, err := s.SubjectArchive(ctx, subject.ID)
			if err != nil {
				t.Fatal(err)
			}
			_ = file.Close()
		}
	}
	for _, id := range []string{subject.ID, ""} {
		release, err := s.archives.beginArchive(id)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.Submit(ctx, subject.ID, "new work", false); domain.AsError(err).Code != "ARCHIVE_BUSY" {
			t.Fatal(err)
		}
		if _, _, err := s.StatusView(ctx, subject.ID, nil, ""); domain.AsError(err).Code != "ARCHIVE_BUSY" {
			t.Fatal(err)
		}
		if id == "" {
			if _, err := s.CreateSubject(ctx, nil, nil); domain.AsError(err).Code != "ARCHIVE_BUSY" {
				t.Fatal(err)
			}
		}
		if _, _, err := s.GetSubject(ctx, subject.ID); err != nil {
			t.Fatal(err)
		}
		release()
	}
	if _, _, err := s.Submit(ctx, subject.ID, "queued work", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SubjectArchive(ctx, subject.ID); domain.AsError(err).Code != "SUBJECT_NOT_IDLE" {
		t.Fatal(err)
	}
	if _, err := s.RootArchive(ctx); domain.AsError(err).Code != "SUBJECTS_NOT_IDLE" {
		t.Fatal(err)
	}
	// Failure must release admission, so another unrelated Subject can be created.
	if _, err := s.CreateSubject(ctx, nil, nil); err != nil {
		t.Fatal(err)
	}
}
