package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/eventlog"
	nornsadapter "github.com/Ghr07h/norns/nornsd/internal/norns"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	runtimemanager "github.com/Ghr07h/norns/nornsd/internal/runtime"
	storesqlite "github.com/Ghr07h/norns/nornsd/internal/store/sqlite"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

type Service struct {
	Owner   *root.Owner
	Store   *storesqlite.Store
	Manager *runtimemanager.Manager
	Profile domain.RuntimeProfile
	Norns   *nornsadapter.Adapter
	locks   sync.Map
}

type Current struct {
	Runtime       *domain.Runtime
	QueuePosition *int
}

type RuntimeArtifacts struct {
	SessionID         string  `json:"-"`
	RawLogPath        *string `json:"raw_log_path"`
	ReadableLogPath   *string `json:"readable_log_path"`
	MemorySamplesPath *string `json:"memory_samples_path"`
	TranscriptPath    *string `json:"transcript_path"`
	TranscriptStatus  string  `json:"transcript_status"`
}

func (s *Service) subjectLock(id string) *sync.Mutex {
	value, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func (s *Service) CreateSubject(ctx context.Context, name *string, instruction *string) (domain.Subject, error) {
	if instruction != nil {
		if err := domain.ValidateInstruction(*instruction, version.MaxInstructionBytes); err != nil {
			return domain.Subject{}, err
		}
	}
	now := time.Now().UTC()
	id := uuid.NewString()
	runtimeID := ""
	if instruction != nil {
		runtimeID = uuid.NewString()
	}
	subject := domain.Subject{ID: id, Name: name, State: domain.SubjectIdle, DirectoryName: root.DirectoryName(now, id), WorkspaceState: domain.WorkspaceMaterializing, Profile: s.Profile, SessionID: id, CreatedAt: now, UpdatedAt: now}
	if err := s.Store.CreateSubject(ctx, subject, instruction, runtimeID, now); err != nil {
		return domain.Subject{}, err
	}
	created, err := s.Store.GetSubject(ctx, id)
	if err != nil {
		return domain.Subject{}, err
	}
	if instruction == nil {
		if _, err = s.Owner.MaterializeSubject(ctx, created); err != nil {
			_ = s.Store.MarkUnavailable(context.Background(), id, map[string]any{"code": "MATERIALIZATION_FAILED"}, time.Now().UTC())
			return domain.Subject{}, err
		}
		if err = s.Manager.RepairReadable(ctx, id); err != nil {
			return domain.Subject{}, err
		}
	} else {
		s.Manager.NotifyQueue()
	}
	return s.Store.GetSubject(ctx, id)
}

func (s *Service) GetSubject(ctx context.Context, id string) (domain.Subject, Current, error) {
	subject, err := s.Store.GetSubject(ctx, id)
	if err != nil {
		return subject, Current{}, err
	}
	current, err := s.current(ctx, subject)
	return subject, current, err
}

func (s *Service) GetSubjectBase(ctx context.Context, id string) (domain.Subject, error) {
	return s.Store.GetSubject(ctx, id)
}

func (s *Service) GetCurrentRuntime(ctx context.Context, subjectID string) (Current, error) {
	runtime, position, err := s.Store.GetCurrentRuntime(ctx, subjectID)
	return Current{Runtime: runtime, QueuePosition: position}, err
}

func (s *Service) current(ctx context.Context, subject domain.Subject) (Current, error) {
	if subject.CurrentRuntimeID == nil {
		return Current{}, nil
	}
	runtime, err := s.Store.GetRuntime(ctx, subject.ID, *subject.CurrentRuntimeID, true)
	if err != nil {
		return Current{}, err
	}
	position, err := s.Store.QueuePosition(ctx, runtime.ID)
	if err != nil {
		return Current{}, err
	}
	return Current{Runtime: &runtime, QueuePosition: position}, nil
}

func (s *Service) Submit(ctx context.Context, subjectID, content string, resumeSession bool) (domain.Runtime, *int, error) {
	if err := domain.ValidateInstruction(content, version.MaxInstructionBytes); err != nil {
		return domain.Runtime{}, nil, err
	}
	lock := s.subjectLock(subjectID)
	lock.Lock()
	defer lock.Unlock()
	runtimeID := uuid.NewString()
	runtime, err := s.Store.SubmitInstruction(ctx, subjectID, runtimeID, content, s.Profile, resumeSession, time.Now().UTC())
	if err != nil {
		return domain.Runtime{}, nil, err
	}
	position, err := s.Store.QueuePosition(ctx, runtime.ID)
	if err != nil {
		return domain.Runtime{}, nil, err
	}
	if runtime.State == domain.RuntimeQueued {
		s.Manager.NotifyQueue()
	} else {
		s.Manager.Wake(runtime.ID)
	}
	return runtime, position, nil
}

func (s *Service) Stop(ctx context.Context, subjectID, mode string) (domain.Runtime, error) {
	if mode != "graceful" && mode != "force" {
		return domain.Runtime{}, domain.NewError("INVALID_REQUEST", "mode must be graceful or force", http.StatusBadRequest)
	}
	lock := s.subjectLock(subjectID)
	lock.Lock()
	defer lock.Unlock()
	runtime, queued, err := s.Store.RequestStop(ctx, subjectID, "user_"+mode+"_stop", time.Now().UTC())
	if err != nil {
		if e := domain.AsError(err); e.Code == "RUNTIME_NOT_FOUND" {
			e.HTTPStatus = http.StatusConflict
		}
		return domain.Runtime{}, err
	}
	if queued {
		subject, subjectErr := s.Store.GetSubject(ctx, subjectID)
		if subjectErr != nil {
			return domain.Runtime{}, subjectErr
		}
		if _, materializeErr := s.Owner.MaterializeSubject(ctx, subject); materializeErr != nil {
			return domain.Runtime{}, materializeErr
		}
		if repairErr := s.Manager.RepairReadable(ctx, subjectID); repairErr != nil {
			return domain.Runtime{}, repairErr
		}
		s.Manager.NotifyQueue()
	} else {
		if repairErr := s.Manager.RepairReadable(ctx, subjectID); repairErr != nil {
			return domain.Runtime{}, repairErr
		}
		_ = s.Manager.Stop(runtime.ID, mode)
	}
	return runtime, nil
}

func (s *Service) SubjectPath(ctx context.Context, subjectID string) (domain.Subject, string, error) {
	subject, err := s.Store.GetSubject(ctx, subjectID)
	if err != nil {
		return subject, "", err
	}
	if subject.WorkspaceState != domain.WorkspaceMaterialized {
		return subject, "", domain.NewError("SUBJECT_NOT_MATERIALIZED", "subject workspace is not materialized", http.StatusConflict)
	}
	path, err := s.Owner.SubjectPath(subject.DirectoryName)
	if err != nil {
		return subject, "", err
	}
	identity, err := root.ReadSubjectIdentity(path)
	if err != nil || identity.SubjectID != subject.ID {
		return subject, "", domain.NewError("WORKSPACE_UNAVAILABLE", "subject workspace identity is invalid", http.StatusConflict)
	}
	return subject, path, nil
}

func (s *Service) StatusView(ctx context.Context, subjectID string, taskID *string) ([]byte, string, error) {
	subject, path, err := s.SubjectPath(ctx, subjectID)
	if err != nil {
		return nil, "", err
	}
	if !subject.Profile.Has("norns") || s.Norns == nil {
		return nil, "", domain.NewError("CAPABILITY_NOT_ENABLED", "Norns is not enabled for this subject", http.StatusConflict)
	}
	return s.Norns.Status(ctx, path, taskID)
}
func (s *Service) TraceView(ctx context.Context, subjectID, explorationID string) ([]byte, string, error) {
	subject, path, err := s.SubjectPath(ctx, subjectID)
	if err != nil {
		return nil, "", err
	}
	if !subject.Profile.Has("norns") || s.Norns == nil {
		return nil, "", domain.NewError("CAPABILITY_NOT_ENABLED", "Norns is not enabled for this subject", http.StatusConflict)
	}
	return s.Norns.Trace(ctx, path, explorationID)
}
func (s *Service) VisualView(ctx context.Context, subjectID, lang string) ([]byte, string, error) {
	subject, path, err := s.SubjectPath(ctx, subjectID)
	if err != nil {
		return nil, "", err
	}
	if !subject.Profile.Has("norns") || s.Norns == nil {
		return nil, "", domain.NewError("CAPABILITY_NOT_ENABLED", "Norns is not enabled for this subject", http.StatusConflict)
	}
	view, err := s.Norns.Visual(ctx, path, lang)
	if err != nil {
		return nil, "", err
	}
	b, err := os.ReadFile(view)
	return b, filepath.Base(view), err
}

func (s *Service) Health(ctx context.Context) error {
	if !s.Manager.Healthy() {
		return domain.NewError("SERVICE_UNAVAILABLE", "scheduler is unavailable", http.StatusServiceUnavailable)
	}
	return s.Store.Health(ctx)
}

func (s *Service) ListSubjects(ctx context.Context, states []domain.SubjectState, limit int, afterTime, afterID string) ([]domain.Subject, int, error) {
	return s.Store.ListSubjects(ctx, states, limit, afterTime, afterID)
}
func (s *Service) ListSubjectSummaries(ctx context.Context, states []domain.SubjectState, limit int, afterTime, afterID string) ([]storesqlite.SubjectSummary, error) {
	return s.Store.ListSubjectSummaries(ctx, states, limit, afterTime, afterID)
}
func (s *Service) ListQueue(ctx context.Context, limit int, after int64) ([]domain.Runtime, int, error) {
	return s.Store.ListQueue(ctx, limit, after)
}
func (s *Service) ListQueueEntries(ctx context.Context, limit int, after int64) ([]storesqlite.QueueEntry, error) {
	return s.Store.ListQueueEntries(ctx, limit, after)
}
func (s *Service) ReadableEvents(ctx context.Context, subjectID, mode string, cursor eventlog.ReadableCursor, limit int) (eventlog.ReadablePage, error) {
	_, subjectPath, err := s.SubjectPath(ctx, subjectID)
	if err != nil {
		return eventlog.ReadablePage{}, err
	}
	path := filepath.Join(subjectPath, ".nornsd", "log", "readable.jsonl")
	switch mode {
	case "latest":
		return eventlog.ReadableLatest(path, limit)
	case "before":
		return eventlog.ReadableBefore(path, cursor, limit)
	case "after":
		return eventlog.ReadableAfter(path, cursor, limit)
	default:
		return eventlog.ReadablePage{}, domain.NewError("INVALID_REQUEST", "invalid event read mode", http.StatusBadRequest)
	}
}

func (s *Service) ReadableEventsUpdated(ctx context.Context, subjectID string, cursor eventlog.ReadableCursor) (bool, error) {
	_, subjectPath, err := s.SubjectPath(ctx, subjectID)
	if err != nil {
		return false, err
	}
	return eventlog.ReadableHasUpdates(filepath.Join(subjectPath, ".nornsd", "log", "readable.jsonl"), cursor)
}
func (s *Service) ListRuntimes(ctx context.Context, subjectID string, limit int, afterTime, afterID string) ([]domain.Runtime, error) {
	if _, err := s.Store.GetSubject(ctx, subjectID); err != nil {
		return nil, err
	}
	return s.Store.ListRuntimes(ctx, subjectID, limit, afterTime, afterID)
}
func (s *Service) ListRuntimeSummaries(ctx context.Context, subjectID string, limit int, afterTime, afterID string) ([]storesqlite.RuntimeSummary, error) {
	return s.Store.ListRuntimeSummaries(ctx, subjectID, limit, afterTime, afterID)
}
func (s *Service) GetRuntime(ctx context.Context, subjectID, runtimeID string) (domain.Runtime, error) {
	return s.Store.GetRuntime(ctx, subjectID, runtimeID, false)
}
func (s *Service) Capacity(ctx context.Context) (int, int, error) { return s.Store.Capacity(ctx) }

func (s *Service) RootID() string { return s.Owner.Identity.RootID }

func (s *Service) WorkspacePath(subject domain.Subject) (*string, error) {
	if subject.WorkspaceState != domain.WorkspaceMaterialized {
		return nil, nil
	}
	path, err := s.Owner.SubjectPath(subject.DirectoryName)
	if err != nil {
		return nil, domain.Wrap("WORKSPACE_UNAVAILABLE", "subject workspace path is invalid", http.StatusConflict, err)
	}
	return &path, nil
}

func (s *Service) RuntimeArtifacts(subject domain.Subject, runtime domain.Runtime) (RuntimeArtifacts, error) {
	out := RuntimeArtifacts{SessionID: runtime.ID, TranscriptStatus: "pending"}
	subjectPath, err := s.WorkspacePath(subject)
	if err != nil || subjectPath == nil {
		return out, err
	}
	raw := filepath.Join(*subjectPath, ".nornsd", "log", "raw.jsonl")
	readable := filepath.Join(*subjectPath, ".nornsd", "log", "readable.jsonl")
	out.RawLogPath = &raw
	out.ReadableLogPath = &readable
	memorySamples := filepath.Join(*subjectPath, ".nornsd", "runtimes", runtime.ID, "memory-samples.jsonl")
	if info, statErr := os.Stat(memorySamples); statErr == nil && info.Mode().IsRegular() {
		out.MemorySamplesPath = &memorySamples
	}
	if len(runtime.TranscriptJSON) == 0 {
		return out, nil
	}
	var archive struct {
		Status    string   `json:"status"`
		SessionID string   `json:"session_id"`
		Copied    []string `json:"copied"`
		Missing   []string `json:"missing"`
	}
	if json.Unmarshal(runtime.TranscriptJSON, &archive) != nil {
		out.TranscriptStatus = "failed"
		return out, nil
	}
	if archive.SessionID != "" {
		out.SessionID = archive.SessionID
	} else if legacySessionID := archivedSessionID(archive.Copied, archive.Missing); legacySessionID != "" {
		out.SessionID = legacySessionID
	} else if runtime.SessionMode == "resume" {
		out.SessionID = subject.SessionID
	}
	if archive.Status == "failed" {
		out.TranscriptStatus = "failed"
		return out, nil
	}
	wanted := out.SessionID + ".jsonl"
	for _, copied := range archive.Copied {
		if copied != wanted {
			continue
		}
		transcript := filepath.Join(*subjectPath, ".nornsd", "runtimes", runtime.ID, wanted)
		if info, statErr := os.Stat(transcript); statErr == nil && info.Mode().IsRegular() {
			out.TranscriptPath = &transcript
			out.TranscriptStatus = "available"
		}
		break
	}
	if out.TranscriptPath == nil && (archive.Status == "complete" || archive.Status == "partial" || archive.Status == "missing") {
		out.TranscriptStatus = "missing"
	}
	return out, nil
}

func archivedSessionID(copied, missing []string) string {
	for _, names := range [][]string{copied, missing} {
		for _, name := range names {
			if strings.Contains(name, "/") || !strings.HasSuffix(name, ".jsonl") {
				continue
			}
			if sessionID := strings.TrimSuffix(name, ".jsonl"); sessionID != "" {
				return sessionID
			}
		}
	}
	return ""
}

func IsNotFound(err error) bool {
	var target *domain.Error
	return errors.As(err, &target) && (target.Code == "SUBJECT_NOT_FOUND" || target.Code == "RUNTIME_NOT_FOUND")
}
