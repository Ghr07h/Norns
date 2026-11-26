package app

import (
	"context"
	"github.com/google/uuid"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/Ghr07h/norns/nornsd/internal/archive"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
)

// Admission covers the entire check-and-build interval, including filesystem
// writes made by View generation and Subject materialization. Downloads use
// completed temporary files and do not retain admission.
type archiveAdmission struct {
	mu             sync.Mutex
	packing        bool
	packingSubject string // Empty while packing the entire Root.
	work           int
	subjectWork    map[string]int
}

func archiveBusy() error {
	return domain.NewError("ARCHIVE_BUSY", "an archive or conflicting operation is in progress", http.StatusConflict)
}

func (a *archiveAdmission) beginWork(subjectID string) (func(), error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.packing && (a.packingSubject == "" || a.packingSubject == subjectID) {
		return nil, archiveBusy()
	}
	if a.subjectWork == nil {
		a.subjectWork = make(map[string]int)
	}
	a.work++
	a.subjectWork[subjectID]++
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.work--
		a.subjectWork[subjectID]--
		if a.subjectWork[subjectID] == 0 {
			delete(a.subjectWork, subjectID)
		}
	}, nil
}

func (a *archiveAdmission) beginArchive(subjectID string) (func(), error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.packing || (subjectID == "" && a.work != 0) || a.subjectWork[subjectID] != 0 {
		return nil, archiveBusy()
	}
	a.packing = true
	a.packingSubject = subjectID
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.packing = false
		a.packingSubject = ""
	}, nil
}

func (s *Service) SubjectArchive(ctx context.Context, subjectID string) (*archive.File, error) {
	return s.Archive(ctx, "subject", subjectID, "")
}
func (s *Service) RootArchive(ctx context.Context) (*archive.File, error) {
	return s.Archive(ctx, "subject", "", "")
}

// Archive selects one of five explicit directory scopes. Every scope shares
// the same admission quota and database snapshot implementation.
func (s *Service) Archive(ctx context.Context, scope, subjectID, runtimeID string) (*archive.File, error) {
	if scope != "subject" && scope != "rundata" {
		return nil, domain.NewError("INVALID_REQUEST", "scope must be subject or rundata", http.StatusBadRequest)
	}
	if scope == "subject" && runtimeID != "" || runtimeID != "" && subjectID == "" {
		return nil, domain.NewError("INVALID_REQUEST", "invalid archive parameter combination", http.StatusBadRequest)
	}
	if subjectID != "" {
		if _, err := uuid.Parse(subjectID); err != nil {
			return nil, domain.NewError("INVALID_REQUEST", "invalid subject_id", http.StatusBadRequest)
		}
	}
	if runtimeID != "" {
		if _, err := uuid.Parse(runtimeID); err != nil {
			return nil, domain.NewError("INVALID_REQUEST", "invalid runtime_id", http.StatusBadRequest)
		}
	}
	release, err := s.archives.beginArchive(subjectID)
	if err != nil {
		return nil, err
	}
	defer release()
	path := s.Owner.Path
	prefix := "nornsd-root"
	if subjectID == "" {
		idle, err := s.Store.AllSubjectsIdle(ctx)
		if err != nil {
			return nil, err
		}
		if !idle {
			return nil, domain.NewError("SUBJECTS_NOT_IDLE", "all subjects must be idle", http.StatusConflict)
		}
		if scope == "rundata" {
			path = s.Owner.Control
			prefix = "nornsd-rundata"
		}
	} else {
		subject, err := s.Store.GetSubject(ctx, subjectID)
		if err != nil {
			return nil, err
		}
		if subject.State != domain.SubjectIdle || subject.CurrentRuntimeID != nil {
			return nil, domain.NewError("SUBJECT_NOT_IDLE", "subject must be idle", http.StatusConflict)
		}
		prefix = "subject-" + subjectID
		if scope == "subject" {
			_, path, err = s.SubjectPath(ctx, subjectID)
			if err != nil {
				return nil, err
			}
		} else {
			path = s.Owner.SubjectData(subjectID)
			if runtimeID != "" {
				if _, err = s.Store.GetRuntime(ctx, subjectID, runtimeID, false); err != nil {
					return nil, err
				}
				path = filepath.Join(path, "runtimes", runtimeID)
				prefix = "runtime-" + runtimeID
			}
		}
	}
	return archive.Create(ctx, path, s.Owner.Path, prefix)
}
