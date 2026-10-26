package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

//go:embed schema.sql
var schemaSQL string

const timeFormat = time.RFC3339Nano

type Store struct {
	db      *sql.DB
	writeMu sync.Mutex
	rootID  string
	key     []byte
}

type dbtx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Metadata struct {
	RootID        string
	CursorHMACKey []byte
	CreatedAt     time.Time
}

type SubjectSummary struct {
	ID        string
	Name      *string
	State     domain.SubjectState
	CreatedAt time.Time
}

type QueueEntry struct {
	Sequence         int64
	Position         int
	RuntimeID        string
	SubjectID        string
	SubjectName      *string
	InstructionCount int
	QueuedAt         time.Time
}

type RuntimeSummary struct {
	ID         string
	State      domain.RuntimeState
	Current    bool
	CreatedAt  time.Time
	StartedAt  *time.Time
	EndedAt    *time.Time
	TotalCount int
}

func Open(ctx context.Context, path string, rootID string, now time.Time) (*Store, bool, error) {
	_, statErr := os.Stat(path)
	isNew := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !isNew {
		return nil, false, domain.Wrap("SERVICE_UNAVAILABLE", "cannot inspect nornsd database", http.StatusServiceUnavailable, statErr)
	}
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(FULL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, false, domain.Wrap("SERVICE_UNAVAILABLE", "cannot open nornsd database", http.StatusServiceUnavailable, err)
	}
	db.SetMaxOpenConns(8)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, false, domain.Wrap("SERVICE_UNAVAILABLE", "cannot access nornsd database", http.StatusServiceUnavailable, err)
	}
	s := &Store{db: db}
	if isNew {
		if rootID == "" {
			rootID = uuid.NewString()
		}
		if err := s.initialize(ctx, rootID, now.UTC()); err != nil {
			_ = db.Close()
			return nil, false, err
		}
	} else if err := s.validate(ctx, rootID); err != nil {
		_ = db.Close()
		return nil, false, err
	}
	var created string
	if err := db.QueryRowContext(ctx, `SELECT root_id,cursor_hmac_key,created_at FROM metadata WHERE singleton=1`).Scan(&s.rootID, &s.key, &created); err != nil {
		_ = db.Close()
		return nil, false, domain.Wrap("UNSUPPORTED_DATABASE", "nornsd metadata is missing", http.StatusInternalServerError, err)
	}
	return s, isNew, nil
}

func (s *Store) initialize(ctx context.Context, rootID string, now time.Time) error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return domain.Wrap("INTERNAL_ERROR", "cannot create cursor signing key", http.StatusInternalServerError, err)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, `PRAGMA application_id = `+fmt.Sprint(version.SQLiteAppID)); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `PRAGMA user_version = `+fmt.Sprint(version.DatabaseSchema)); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `PRAGMA journal_mode = WAL`); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	if _, err = conn.ExecContext(ctx, schemaSQL); err != nil {
		return domain.Wrap("UNSUPPORTED_DATABASE", "cannot initialize nornsd schema", http.StatusInternalServerError, err)
	}
	t := formatTime(now)
	if _, err = conn.ExecContext(ctx, `INSERT INTO metadata(singleton,root_id,cursor_hmac_key,created_at,updated_at) VALUES(1,?,?,?,?)`, rootID, key, t, t); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	ok = true
	return nil
}

func (s *Store) validate(ctx context.Context, expectedRootID string) error {
	var appID, schema int
	if err := s.db.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&appID); err != nil {
		return domain.Wrap("UNSUPPORTED_DATABASE", "cannot read database application id", http.StatusInternalServerError, err)
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&schema); err != nil {
		return domain.Wrap("UNSUPPORTED_DATABASE", "cannot read database schema version", http.StatusInternalServerError, err)
	}
	if appID != version.SQLiteAppID || schema != version.DatabaseSchema {
		return NewUnsupported(appID, schema)
	}
	var rootID string
	if err := s.db.QueryRowContext(ctx, `SELECT root_id FROM metadata WHERE singleton=1`).Scan(&rootID); err != nil {
		return domain.Wrap("UNSUPPORTED_DATABASE", "nornsd metadata is invalid", http.StatusInternalServerError, err)
	}
	if expectedRootID != "" && rootID != expectedRootID {
		return domain.NewError("UNSUPPORTED_DATABASE", "database root identity does not match root.json", http.StatusInternalServerError)
	}
	return nil
}

func NewUnsupported(appID, schema int) error {
	e := domain.NewError("UNSUPPORTED_DATABASE", "unsupported nornsd database", http.StatusInternalServerError)
	e.Details = map[string]any{"application_id": appID, "schema_version": schema}
	return e
}

func (s *Store) Close() error {
	_, _ = s.db.Exec(`PRAGMA wal_checkpoint(PASSIVE)`)
	return s.db.Close()
}

func (s *Store) RootID() string             { return s.rootID }
func (s *Store) CursorKey() []byte          { return append([]byte(nil), s.key...) }
func formatTime(t time.Time) string         { return t.UTC().Format(timeFormat) }
func parseTime(v string) (time.Time, error) { return time.Parse(timeFormat, v) }

func (s *Store) Health(ctx context.Context) error {
	var one int
	return s.db.QueryRowContext(ctx, `SELECT 1`).Scan(&one)
}

func (s *Store) withWrite(ctx context.Context, fn func(dbtx) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return domain.Wrap("SERVICE_UNAVAILABLE", "cannot begin database transaction", http.StatusServiceUnavailable, err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	if err = fn(conn); err != nil {
		return err
	}
	if err = validateInvariants(ctx, conn); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `COMMIT`); err != nil {
		return domain.Wrap("SERVICE_UNAVAILABLE", "cannot commit database transaction", http.StatusServiceUnavailable, err)
	}
	committed = true
	return nil
}

func validateInvariants(ctx context.Context, q dbtx) error {
	checks := []string{
		`SELECT COUNT(*) FROM subjects s WHERE (s.state='idle') <> (s.current_runtime_id IS NULL)`,
		`SELECT COUNT(*) FROM subjects s LEFT JOIN runtimes r ON r.id=s.current_runtime_id AND r.subject_id=s.id WHERE s.current_runtime_id IS NOT NULL AND r.id IS NULL`,
		`SELECT COUNT(*) FROM runtime_queue q JOIN runtimes r ON r.id=q.runtime_id WHERE r.state<>'queued' OR r.subject_id<>q.subject_id`,
		`SELECT COUNT(*) FROM runtimes r WHERE (SELECT COUNT(*) FROM runtime_instructions i WHERE i.runtime_id=r.id AND i.dispatch_committed_at IS NULL)>16`,
	}
	for _, query := range checks {
		var count int
		if err := q.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return domain.NewError("INTERNAL_ERROR", "database invariant violation", http.StatusInternalServerError)
		}
	}
	return nil
}

func profileJSON(profile domain.RuntimeProfile) string {
	b, _ := json.Marshal(profile)
	return string(b)
}

func payloadJSON(value any) string {
	if value == nil {
		return "{}"
	}
	b, _ := json.Marshal(value)
	return string(b)
}

func (s *Store) CreateSubject(ctx context.Context, subject domain.Subject, instruction *string, runtimeID string, now time.Time) error {
	return s.withWrite(ctx, func(q dbtx) error {
		state, workspaceState := string(domain.SubjectIdle), domain.WorkspaceMaterializing
		var current any
		if instruction != nil {
			state, workspaceState, current = string(domain.SubjectQueuing), domain.WorkspacePlanned, runtimeID
		}
		_, err := q.ExecContext(ctx, `INSERT INTO subjects(id,name,state,directory_name,workspace_state,profile_json,session_id,session_initialized,current_runtime_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, subject.ID, subject.Name, state, subject.DirectoryName, workspaceState, profileJSON(subject.Profile), subject.SessionID, boolInt(subject.SessionInitialized), current, formatTime(now), formatTime(now))
		if err != nil {
			return mapSQLError(err)
		}
		if instruction == nil {
			return nil
		}
		return insertQueuedRuntime(ctx, q, subject.ID, runtimeID, *instruction, "", now)
	})
}

func (s *Store) CreateDirectRuntime(ctx context.Context, subjectID, runtimeID string, now time.Time) (domain.Runtime, error) {
	runtime, _, err := s.CreateDirectRuntimeWithSession(ctx, subjectID, runtimeID, false, now)
	return runtime, err
}

func (s *Store) CreateDirectRuntimeWithSession(ctx context.Context, subjectID, runtimeID string, resumeSession bool, now time.Time) (domain.Runtime, string, error) {
	var out domain.Runtime
	var resumeSessionID string
	err := s.withWrite(ctx, func(q dbtx) error {
		subject, err := getSubject(ctx, q, subjectID)
		if err != nil {
			return err
		}
		if subject.State != domain.SubjectIdle {
			return domain.NewError("SUBJECT_RUNTIME_ACTIVE", "subject already has an active runtime", http.StatusConflict)
		}
		if resumeSession {
			resumeSessionID, err = latestRuntimeSessionID(ctx, q, subject)
			if err != nil {
				return err
			}
		}
		sessionMode := "new"
		if resumeSessionID != "" {
			sessionMode = "resume"
		}
		t := formatTime(now)
		if _, err = q.ExecContext(ctx, `INSERT INTO runtimes(id,subject_id,state,session_mode,process_json,started_at,state_changed_at,created_at) VALUES(?,?,'running',?,?,?,?,?)`, runtimeID, subjectID, sessionMode, payloadJSON(runtimeSessionMetadata(runtimeID, resumeSessionID)), t, t, t); err != nil {
			return err
		}
		if _, err = q.ExecContext(ctx, `UPDATE subjects SET state='using',current_runtime_id=?,updated_at=? WHERE id=? AND state='idle'`, runtimeID, t, subjectID); err != nil {
			return err
		}
		out, err = getRuntime(ctx, q, subjectID, runtimeID, false)
		return err
	})
	return out, resumeSessionID, err
}

func (s *Store) MarkStopping(ctx context.Context, subjectID, runtimeID string, state domain.RuntimeState, reason, errorCode string, now time.Time) error {
	return s.SetRuntimeTerminal(ctx, subjectID, runtimeID, state, reason, errorCode, now, false)
}

func (s *Store) SaveConfigSnapshot(ctx context.Context, runtimeID string, snapshot any, now time.Time) error {
	return s.withWrite(ctx, func(q dbtx) error {
		_, err := q.ExecContext(ctx, `INSERT OR REPLACE INTO runtime_config_snapshots(runtime_id,snapshot_json,created_at) VALUES(?,?,?)`, runtimeID, payloadJSON(snapshot), formatTime(now))
		return err
	})
}

func insertQueuedRuntime(ctx context.Context, q dbtx, subjectID, runtimeID, instruction, resumeSessionID string, now time.Time) error {
	t := formatTime(now)
	sessionMode := "new"
	if resumeSessionID != "" {
		sessionMode = "resume"
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO runtimes(id,subject_id,state,session_mode,process_json,queued_at,state_changed_at,created_at) VALUES(?,?,'queued',?,?,?,?,?)`, runtimeID, subjectID, sessionMode, payloadJSON(runtimeSessionMetadata(runtimeID, resumeSessionID)), t, t, t); err != nil {
		return mapSQLError(err)
	}
	result, err := q.ExecContext(ctx, `INSERT INTO runtime_queue(runtime_id,subject_id,queued_at) VALUES(?,?,?)`, runtimeID, subjectID, t)
	if err != nil {
		return mapSQLError(err)
	}
	seq, err := result.LastInsertId()
	if err != nil {
		return err
	}
	if _, err = q.ExecContext(ctx, `UPDATE runtimes SET queue_sequence=? WHERE id=?`, seq, runtimeID); err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO runtime_instructions(runtime_id,sequence,content,byte_length,created_at) VALUES(?,1,?,?,?)`, runtimeID, instruction, len([]byte(instruction)), t)
	return err
}

func (s *Store) MarkMaterialized(ctx context.Context, subjectID string, now time.Time) error {
	return s.withWrite(ctx, func(q dbtx) error {
		var state string
		if err := q.QueryRowContext(ctx, `SELECT workspace_state FROM subjects WHERE id=?`, subjectID).Scan(&state); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.NewError("SUBJECT_NOT_FOUND", "subject was not found", http.StatusNotFound)
			}
			return err
		}
		if state == domain.WorkspaceMaterialized {
			return nil
		}
		if state != domain.WorkspaceMaterializing && state != domain.WorkspacePlanned {
			return domain.NewError("WORKSPACE_UNAVAILABLE", "subject workspace cannot be materialized", http.StatusConflict)
		}
		result, err := q.ExecContext(ctx, `UPDATE subjects SET workspace_state='materialized',workspace_error=NULL,updated_at=? WHERE id=? AND workspace_state IN ('materializing','planned')`, formatTime(now), subjectID)
		if err != nil {
			return err
		}
		if err = requireAffected(result, "SUBJECT_NOT_FOUND", "subject was not found", http.StatusNotFound); err != nil {
			return err
		}
		_, err = appendEvent(ctx, q, subjectID, nil, "subject.created", "server", map[string]any{"workspace_state": domain.WorkspaceMaterialized}, now, nil, nil)
		return err
	})
}

func (s *Store) MarkUnavailable(ctx context.Context, subjectID string, detail any, now time.Time) error {
	return s.withWrite(ctx, func(q dbtx) error {
		_, err := q.ExecContext(ctx, `UPDATE subjects SET workspace_state='unavailable',workspace_error=?,updated_at=? WHERE id=?`, payloadJSON(detail), formatTime(now), subjectID)
		return err
	})
}

func (s *Store) GetSubject(ctx context.Context, id string) (domain.Subject, error) {
	return getSubject(ctx, s.db, id)
}

const subjectColumns = `id,name,state,directory_name,workspace_state,workspace_error,profile_json,session_id,session_initialized,current_runtime_id,last_event_seq,last_raw_seq,last_projected_raw_seq,created_at,updated_at`

type subjectRow struct {
	value                       domain.Subject
	name, workspaceErr, current sql.NullString
	profile, created, updated   string
	initialized                 int
}

func (r *subjectRow) destinations() []any {
	return []any{&r.value.ID, &r.name, &r.value.State, &r.value.DirectoryName, &r.value.WorkspaceState, &r.workspaceErr, &r.profile, &r.value.SessionID, &r.initialized, &r.current, &r.value.LastEventSeq, &r.value.LastRawSeq, &r.value.LastProjectedRawSeq, &r.created, &r.updated}
}

func (r *subjectRow) subject() (domain.Subject, error) {
	out := r.value
	if r.name.Valid {
		out.Name = &r.name.String
	}
	if r.workspaceErr.Valid {
		out.WorkspaceError = json.RawMessage(r.workspaceErr.String)
	}
	if r.current.Valid {
		out.CurrentRuntimeID = &r.current.String
	}
	out.SessionInitialized = r.initialized != 0
	if err := json.Unmarshal([]byte(r.profile), &out.Profile); err != nil {
		return out, err
	}
	out.CreatedAt, _ = parseTime(r.created)
	out.UpdatedAt, _ = parseTime(r.updated)
	return out, nil
}

func getSubject(ctx context.Context, q dbtx, id string) (domain.Subject, error) {
	var record subjectRow
	if err := q.QueryRowContext(ctx, `SELECT `+subjectColumns+` FROM subjects WHERE id=?`, id).Scan(record.destinations()...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Subject{}, domain.NewError("SUBJECT_NOT_FOUND", "subject was not found", http.StatusNotFound)
		}
		return domain.Subject{}, err
	}
	return record.subject()
}

func (s *Store) ListSubjects(ctx context.Context, states []domain.SubjectState, limit int, afterTime, afterID string) ([]domain.Subject, int, error) {
	args := []any{}
	where := []string{"1=1"}
	if len(states) > 0 {
		marks := make([]string, len(states))
		for i, state := range states {
			marks[i] = "?"
			args = append(args, state)
		}
		where = append(where, "state IN ("+strings.Join(marks, ",")+")")
	}
	filter := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subjects WHERE `+filter, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if afterTime != "" {
		where = append(where, `(created_at < ? OR (created_at = ? AND id > ?))`)
		args = append(args, afterTime, afterTime, afterID)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `SELECT `+subjectColumns+` FROM subjects WHERE `+strings.Join(where, " AND ")+` ORDER BY created_at DESC,id ASC LIMIT ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var result []domain.Subject
	for rows.Next() {
		var record subjectRow
		if err := rows.Scan(record.destinations()...); err != nil {
			return nil, 0, err
		}
		subject, err := record.subject()
		if err != nil {
			return nil, 0, err
		}
		result = append(result, subject)
	}
	return result, total, rows.Err()
}

// ListSubjectSummaries returns the minimal projection used by the HTTP subject list.
func (s *Store) ListSubjectSummaries(ctx context.Context, states []domain.SubjectState, limit int, afterTime, afterID string) ([]SubjectSummary, error) {
	args := []any{}
	where := []string{"1=1"}
	if len(states) > 0 {
		marks := make([]string, len(states))
		for i, state := range states {
			marks[i] = "?"
			args = append(args, state)
		}
		where = append(where, "state IN ("+strings.Join(marks, ",")+")")
	}
	if afterTime != "" {
		where = append(where, `(created_at < ? OR (created_at = ? AND id > ?))`)
		args = append(args, afterTime, afterTime, afterID)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,state,created_at FROM subjects WHERE `+strings.Join(where, " AND ")+` ORDER BY created_at DESC,id ASC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SubjectSummary{}
	for rows.Next() {
		var item SubjectSummary
		var name sql.NullString
		var created string
		if err := rows.Scan(&item.ID, &name, &item.State, &created); err != nil {
			return nil, err
		}
		if name.Valid {
			item.Name = &name.String
		}
		item.CreatedAt, _ = parseTime(created)
		result = append(result, item)
	}
	return result, rows.Err()
}

func runtimeSessionMetadata(sessionID, resumeSessionID string) map[string]any {
	metadata := map[string]any{"session_id": sessionID}
	if resumeSessionID != "" {
		metadata["resume_session_id"] = resumeSessionID
	}
	return metadata
}

func latestRuntimeSessionID(ctx context.Context, q dbtx, subject domain.Subject) (string, error) {
	var runtimeID string
	err := q.QueryRowContext(ctx, `SELECT id FROM runtimes WHERE subject_id=? ORDER BY created_at DESC,id ASC LIMIT 1`, subject.ID).Scan(&runtimeID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.NewError("RUNTIME_SESSION_NOT_FOUND", "subject has no previous runtime session", http.StatusConflict)
	}
	if err != nil {
		return "", err
	}
	runtime, err := getRuntime(ctx, q, subject.ID, runtimeID, false)
	if err != nil {
		return "", err
	}
	for _, payload := range []json.RawMessage{runtime.ProcessJSON, runtime.TranscriptJSON} {
		var metadata struct {
			SessionID string `json:"session_id"`
		}
		if json.Unmarshal(payload, &metadata) == nil && metadata.SessionID != "" {
			return metadata.SessionID, nil
		}
	}
	if runtime.SessionMode == "new" {
		return runtime.ID, nil
	}
	if subject.SessionID != "" {
		return subject.SessionID, nil
	}
	return "", domain.NewError("RUNTIME_SESSION_NOT_FOUND", "latest runtime session identity is unavailable", http.StatusConflict)
}

func (s *Store) SubmitInstruction(ctx context.Context, subjectID, runtimeID, content string, profile domain.RuntimeProfile, resumeSession bool, now time.Time) (domain.Runtime, error) {
	var result domain.Runtime
	err := s.withWrite(ctx, func(q dbtx) error {
		subject, err := getSubject(ctx, q, subjectID)
		if err != nil {
			return err
		}
		if !subject.Profile.Equal(profile) {
			return domain.NewError("RUNTIME_PROFILE_MISMATCH", "subject runtime profile does not match server", http.StatusConflict)
		}
		if subject.WorkspaceState == domain.WorkspaceUnavailable {
			return domain.NewError("WORKSPACE_UNAVAILABLE", "subject workspace is unavailable", http.StatusConflict)
		}
		if resumeSession && subject.State != domain.SubjectIdle {
			return domain.NewError("SUBJECT_RUNTIME_ACTIVE", "subject already has an active runtime", http.StatusConflict)
		}
		if subject.State == domain.SubjectIdle {
			resumeSessionID := ""
			if resumeSession {
				resumeSessionID, err = latestRuntimeSessionID(ctx, q, subject)
				if err != nil {
					return err
				}
			}
			if _, err = q.ExecContext(ctx, `UPDATE subjects SET state='queuing',current_runtime_id=?,updated_at=? WHERE id=? AND state='idle'`, runtimeID, formatTime(now), subjectID); err != nil {
				return err
			}
			if err = insertQueuedRuntime(ctx, q, subjectID, runtimeID, content, resumeSessionID, now); err != nil {
				return err
			}
			result, err = getRuntime(ctx, q, subjectID, runtimeID, true)
			return err
		}
		if subject.CurrentRuntimeID == nil {
			return domain.NewError("INTERNAL_ERROR", "subject state is inconsistent", http.StatusInternalServerError)
		}
		result, err = getRuntime(ctx, q, subjectID, *subject.CurrentRuntimeID, true)
		if err != nil {
			return err
		}
		if result.Closing {
			return domain.NewError("RUNTIME_CLOSING", "runtime is closing", http.StatusConflict)
		}
		if result.State == domain.RuntimeError || result.State == domain.RuntimeCancelled {
			return domain.NewError("RUNTIME_STOPPING", "runtime is stopping", http.StatusConflict)
		}
		var pending, next int
		if err = q.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM runtime_instructions WHERE runtime_id=? AND dispatch_committed_at IS NULL),COALESCE(MAX(sequence),0)+1 FROM runtime_instructions WHERE runtime_id=?`, result.ID, result.ID).Scan(&pending, &next); err != nil {
			return err
		}
		if pending >= version.MaxPendingCount {
			e := domain.NewError("RUNTIME_INSTRUCTION_REJECTED", "runtime pending instruction limit reached", http.StatusUnprocessableEntity)
			e.Details = map[string]any{"reason": "pending_limit", "limit": version.MaxPendingCount}
			return e
		}
		_, err = q.ExecContext(ctx, `INSERT INTO runtime_instructions(runtime_id,sequence,content,byte_length,created_at) VALUES(?,?,?,?,?)`, result.ID, next, content, len([]byte(content)), formatTime(now))
		if err != nil {
			return err
		}
		result, err = getRuntime(ctx, q, subjectID, result.ID, true)
		return err
	})
	return result, err
}

func (s *Store) GetRuntime(ctx context.Context, subjectID, runtimeID string, pending bool) (domain.Runtime, error) {
	return getRuntime(ctx, s.db, subjectID, runtimeID, pending)
}

const runtimeColumns = `r.id,r.subject_id,r.state,r.queue_sequence,r.settings_slot,r.config_hash,r.session_mode,r.inflight_turns,r.closing,r.close_deadline,r.queued_at,r.started_at,r.ended_at,r.state_changed_at,r.end_reason,r.error_code,r.error_json,r.process_json,r.usage_json,r.resource_json,r.transcript_json,r.created_at`

type runtimeRow struct {
	value                                                                                                       domain.Runtime
	queue, slot                                                                                                 sql.NullInt64
	hash, deadline, queued, started, ended, reason, errCode, errJSON, processJSON, usageJSON, resourceJSON, log sql.NullString
	closing                                                                                                     int
	changed, created                                                                                            string
}

func (r *runtimeRow) destinations() []any {
	return []any{&r.value.ID, &r.value.SubjectID, &r.value.State, &r.queue, &r.slot, &r.hash, &r.value.SessionMode, &r.value.InflightTurns, &r.closing, &r.deadline, &r.queued, &r.started, &r.ended, &r.changed, &r.reason, &r.errCode, &r.errJSON, &r.processJSON, &r.usageJSON, &r.resourceJSON, &r.log, &r.created}
}

func (r *runtimeRow) runtime() domain.Runtime {
	out := r.value
	if r.queue.Valid {
		value := r.queue.Int64
		out.QueueSequence = &value
	}
	if r.slot.Valid {
		value := int(r.slot.Int64)
		out.SettingsSlot = &value
	}
	if r.hash.Valid {
		out.ConfigHash = &r.hash.String
	}
	out.Closing = r.closing != 0
	setTimePtr := func(value sql.NullString, dst **time.Time) {
		if value.Valid {
			parsed, _ := parseTime(value.String)
			*dst = &parsed
		}
	}
	setTimePtr(r.deadline, &out.CloseDeadline)
	setTimePtr(r.queued, &out.QueuedAt)
	setTimePtr(r.started, &out.StartedAt)
	setTimePtr(r.ended, &out.EndedAt)
	out.StateChangedAt, _ = parseTime(r.changed)
	out.CreatedAt, _ = parseTime(r.created)
	if r.reason.Valid {
		out.EndReason = &r.reason.String
	}
	if r.errCode.Valid {
		out.ErrorCode = &r.errCode.String
	}
	if r.errJSON.Valid {
		out.ErrorJSON = []byte(r.errJSON.String)
	}
	if r.processJSON.Valid {
		out.ProcessJSON = []byte(r.processJSON.String)
	}
	if r.usageJSON.Valid {
		out.UsageJSON = []byte(r.usageJSON.String)
	}
	if r.resourceJSON.Valid {
		out.ResourceJSON = []byte(r.resourceJSON.String)
	}
	if r.log.Valid {
		out.TranscriptJSON = []byte(r.log.String)
	}
	return out
}

func getRuntime(ctx context.Context, q dbtx, subjectID, runtimeID string, pending bool) (domain.Runtime, error) {
	var record runtimeRow
	row := q.QueryRowContext(ctx, `SELECT `+runtimeColumns+` FROM runtimes r WHERE r.subject_id=? AND r.id=?`, subjectID, runtimeID)
	if err := row.Scan(record.destinations()...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Runtime{}, domain.NewError("RUNTIME_NOT_FOUND", "runtime was not found", http.StatusNotFound)
		}
		return domain.Runtime{}, err
	}
	out := record.runtime()
	if pending {
		instructions, err := listPending(ctx, q, runtimeID)
		if err != nil {
			return out, err
		}
		out.Instructions = instructions
	}
	return out, nil
}

// GetCurrentRuntime returns the projection used by the HTTP current-runtime endpoint.
func (s *Store) GetCurrentRuntime(ctx context.Context, subjectID string) (*domain.Runtime, *int, error) {
	var runtimeID sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT current_runtime_id FROM subjects WHERE id=?`, subjectID).Scan(&runtimeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, domain.NewError("SUBJECT_NOT_FOUND", "subject was not found", http.StatusNotFound)
		}
		return nil, nil, err
	}
	if !runtimeID.Valid {
		return nil, nil, nil
	}
	var record runtimeRow
	var position sql.NullInt64
	var instructionsJSON string
	destinations := append(record.destinations(), &position, &instructionsJSON)
	query := `SELECT ` + runtimeColumns + `,
		CASE WHEN q.sequence IS NULL THEN NULL ELSE (SELECT COUNT(*) FROM runtime_queue before WHERE before.sequence<=q.sequence) END,
		COALESCE((SELECT json_group_array(json_object('sequence',i.sequence,'content',i.content,'created_at',i.created_at)) FROM (SELECT sequence,content,created_at FROM runtime_instructions WHERE runtime_id=r.id AND dispatch_committed_at IS NULL ORDER BY sequence) i),'[]')
		FROM runtimes r LEFT JOIN runtime_queue q ON q.runtime_id=r.id
		WHERE r.subject_id=? AND r.id=?`
	if err := s.db.QueryRowContext(ctx, query, subjectID, runtimeID.String).Scan(destinations...); err != nil {
		return nil, nil, err
	}
	runtime := record.runtime()
	if err := json.Unmarshal([]byte(instructionsJSON), &runtime.Instructions); err != nil {
		return nil, nil, err
	}
	var queuePosition *int
	if position.Valid {
		value := int(position.Int64)
		queuePosition = &value
	}
	return &runtime, queuePosition, nil
}

func listPending(ctx context.Context, q dbtx, runtimeID string) ([]domain.Instruction, error) {
	rows, err := q.QueryContext(ctx, `SELECT sequence,content,created_at FROM runtime_instructions WHERE runtime_id=? AND dispatch_committed_at IS NULL ORDER BY sequence`, runtimeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Instruction{}
	for rows.Next() {
		var item domain.Instruction
		var created string
		if err := rows.Scan(&item.Sequence, &item.Content, &created); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = parseTime(created)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ClaimNext(ctx context.Context, slot int, now time.Time) (domain.Subject, domain.Runtime, bool, error) {
	var subject domain.Subject
	var runtime domain.Runtime
	found := false
	err := s.withWrite(ctx, func(q dbtx) error {
		var subjectID, runtimeID string
		err := q.QueryRowContext(ctx, `SELECT subject_id,runtime_id FROM runtime_queue ORDER BY sequence LIMIT 1`).Scan(&subjectID, &runtimeID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		subject, err = getSubject(ctx, q, subjectID)
		if err != nil {
			return err
		}
		runtime, err = getRuntime(ctx, q, subjectID, runtimeID, true)
		if err != nil {
			return err
		}
		if subject.State != domain.SubjectQueuing || runtime.State != domain.RuntimeQueued || subject.CurrentRuntimeID == nil || *subject.CurrentRuntimeID != runtimeID {
			return domain.NewError("INTERNAL_ERROR", "stale runtime queue entry", http.StatusInternalServerError)
		}
		t := formatTime(now)
		if _, err = q.ExecContext(ctx, `DELETE FROM runtime_queue WHERE runtime_id=?`, runtimeID); err != nil {
			return err
		}
		if _, err = q.ExecContext(ctx, `UPDATE runtimes SET state='running',settings_slot=?,started_at=?,state_changed_at=? WHERE id=? AND state='queued'`, slot, t, t, runtimeID); err != nil {
			return err
		}
		if _, err = q.ExecContext(ctx, `UPDATE subjects SET state='using',updated_at=? WHERE id=? AND state='queuing'`, t, subjectID); err != nil {
			return err
		}
		if _, err = appendEvent(ctx, q, subjectID, &runtimeID, "runtime.started", "runtime", map[string]any{"slot": slot}, now, nil, nil); err != nil {
			return err
		}
		found = true
		runtime.State = domain.RuntimeRunning
		runtime.SettingsSlot = &slot
		runtime.StartedAt = &now
		runtime.StateChangedAt = now
		return nil
	})
	return subject, runtime, found, err
}

func (s *Store) DispatchPending(ctx context.Context, subjectID, runtimeID string, first bool, now time.Time) ([]domain.Instruction, int64, error) {
	var items []domain.Instruction
	var group int64
	err := s.withWrite(ctx, func(q dbtx) error {
		var err error
		items, err = listPending(ctx, q, runtimeID)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		if !first {
			items = items[:1]
		}
		if err = q.QueryRowContext(ctx, `SELECT COALESCE(MAX(dispatch_group),0)+1 FROM runtime_instructions WHERE runtime_id=?`, runtimeID).Scan(&group); err != nil {
			return err
		}
		for _, item := range items {
			if _, err = q.ExecContext(ctx, `UPDATE runtime_instructions SET dispatch_group=?,dispatch_committed_at=? WHERE runtime_id=? AND sequence=? AND dispatch_committed_at IS NULL`, group, formatTime(now), runtimeID, item.Sequence); err != nil {
				return err
			}
		}
		return nil
	})
	return items, group, err
}

func (s *Store) ConfirmDispatch(ctx context.Context, subjectID, runtimeID string, group int64, now time.Time) error {
	return s.withWrite(ctx, func(q dbtx) error {
		rows, err := q.QueryContext(ctx, `SELECT sequence,content FROM runtime_instructions WHERE runtime_id=? AND dispatch_group=? ORDER BY sequence`, runtimeID, group)
		if err != nil {
			return err
		}
		type dispatchedInstruction struct {
			sequence uint64
			content  string
		}
		var dispatched []dispatchedInstruction
		for rows.Next() {
			var item dispatchedInstruction
			if err = rows.Scan(&item.sequence, &item.content); err != nil {
				_ = rows.Close()
				return err
			}
			dispatched = append(dispatched, item)
		}
		if err = rows.Close(); err != nil {
			return err
		}
		result, err := q.ExecContext(ctx, `UPDATE runtime_instructions SET write_confirmed_at=? WHERE runtime_id=? AND dispatch_group=? AND write_confirmed_at IS NULL`, formatTime(now), runtimeID, group)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return domain.NewError("INTERNAL_ERROR", "dispatch group was not found", http.StatusInternalServerError)
		}
		if _, err = q.ExecContext(ctx, `UPDATE runtimes SET state='running',inflight_turns=inflight_turns+1,state_changed_at=? WHERE subject_id=? AND id=?`, formatTime(now), subjectID, runtimeID); err != nil {
			return err
		}
		for _, item := range dispatched {
			if _, err = appendEvent(ctx, q, subjectID, &runtimeID, "instruction.dispatched", "user", map[string]any{"dispatch_group": group, "sequence": item.sequence, "content": item.content}, now, nil, nil); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) MarkUncertainDeliveries(ctx context.Context, subjectID, runtimeID string, now time.Time) (int, error) {
	count := 0
	err := s.withWrite(ctx, func(q dbtx) error {
		rows, err := q.QueryContext(ctx, `SELECT sequence FROM runtime_instructions WHERE runtime_id=? AND dispatch_committed_at IS NOT NULL AND write_confirmed_at IS NULL ORDER BY sequence`, runtimeID)
		if err != nil {
			return err
		}
		var sequences []uint64
		for rows.Next() {
			var sequence uint64
			if err = rows.Scan(&sequence); err != nil {
				_ = rows.Close()
				return err
			}
			sequences = append(sequences, sequence)
		}
		if err = rows.Close(); err != nil {
			return err
		}
		if len(sequences) == 0 {
			return nil
		}
		detail := map[string]any{"code": "DELIVERY_UNCERTAIN", "detected_at": now.UTC()}
		if _, err = q.ExecContext(ctx, `UPDATE runtime_instructions SET delivery_error_json=? WHERE runtime_id=? AND dispatch_committed_at IS NOT NULL AND write_confirmed_at IS NULL`, payloadJSON(detail), runtimeID); err != nil {
			return err
		}
		if _, err = appendEvent(ctx, q, subjectID, &runtimeID, "diagnostic", "runtime", map[string]any{"code": "DELIVERY_UNCERTAIN", "instruction_sequences": sequences}, now, nil, nil); err != nil {
			return err
		}
		count = len(sequences)
		return nil
	})
	return count, err
}

func (s *Store) CompleteTurn(ctx context.Context, subjectID, runtimeID string, usage any, now time.Time) (domain.Runtime, error) {
	var out domain.Runtime
	err := s.withWrite(ctx, func(q dbtx) error {
		r, err := getRuntime(ctx, q, subjectID, runtimeID, false)
		if err != nil {
			return err
		}
		if r.State.Terminal() {
			out = r
			return nil
		}
		if r.InflightTurns < 1 {
			return domain.NewError("AGENT_PROTOCOL_ERROR", "received result without an in-flight turn", http.StatusInternalServerError)
		}
		state := domain.RuntimeRunning
		if r.InflightTurns == 1 {
			var pending int
			if err = q.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_instructions WHERE runtime_id=? AND dispatch_committed_at IS NULL`, runtimeID).Scan(&pending); err != nil {
				return err
			}
			if pending == 0 {
				state = domain.RuntimeWaiting
			}
		}
		_, err = q.ExecContext(ctx, `UPDATE runtimes SET state=?,inflight_turns=inflight_turns-1,state_changed_at=?,usage_json=? WHERE id=?`, state, formatTime(now), nullableJSON(usage), runtimeID)
		if err != nil {
			return err
		}
		out, err = getRuntime(ctx, q, subjectID, runtimeID, true)
		return err
	})
	return out, err
}

func (s *Store) SetRuntimeTerminal(ctx context.Context, subjectID, runtimeID string, state domain.RuntimeState, reason, errorCode string, now time.Time, clear bool) error {
	return s.SetRuntimeTerminalWithDetails(ctx, subjectID, runtimeID, state, reason, errorCode, nil, now, clear)
}

func (s *Store) SetRuntimeTerminalWithDetails(ctx context.Context, subjectID, runtimeID string, state domain.RuntimeState, reason, errorCode string, errorDetails any, now time.Time, clear bool) error {
	if !state.Terminal() {
		return domain.NewError("INTERNAL_ERROR", "terminal runtime state required", http.StatusInternalServerError)
	}
	if errorCode != "" && errorDetails == nil {
		errorDetails = map[string]any{"message": reason}
	}
	return s.withWrite(ctx, func(q dbtx) error {
		current, err := getRuntime(ctx, q, subjectID, runtimeID, false)
		if err != nil {
			return err
		}
		if current.State.Terminal() {
			if current.State == state {
				if len(current.ErrorJSON) == 0 && errorDetails != nil {
					_, err = q.ExecContext(ctx, `UPDATE runtimes SET error_json=? WHERE subject_id=? AND id=?`, nullableJSON(errorDetails), subjectID, runtimeID)
					return err
				}
				return nil
			}
			return domain.NewError("INTERNAL_ERROR", "terminal runtime state is immutable", http.StatusInternalServerError)
		}
		_, err = q.ExecContext(ctx, `UPDATE runtimes SET state=?,ended_at=COALESCE(ended_at,?),state_changed_at=?,end_reason=?,error_code=?,error_json=? WHERE subject_id=? AND id=?`, state, formatTime(now), formatTime(now), nullString(reason), nullString(errorCode), nullableJSON(errorDetails), subjectID, runtimeID)
		if err != nil {
			return err
		}
		if clear {
			_, err = q.ExecContext(ctx, `UPDATE subjects SET state='idle',current_runtime_id=NULL,updated_at=? WHERE id=? AND current_runtime_id=?`, formatTime(now), subjectID, runtimeID)
		}
		return err
	})
}

func (s *Store) FinalizeRuntime(ctx context.Context, subjectID, runtimeID string, now time.Time) error {
	return s.withWrite(ctx, func(q dbtx) error {
		r, err := getRuntime(ctx, q, subjectID, runtimeID, false)
		if err != nil {
			return err
		}
		if !r.State.Terminal() {
			return domain.NewError("INTERNAL_ERROR", "cannot finalize a non-terminal runtime", http.StatusInternalServerError)
		}
		result, err := q.ExecContext(ctx, `UPDATE subjects SET state='idle',current_runtime_id=NULL,updated_at=? WHERE id=? AND current_runtime_id=?`, formatTime(now), subjectID, runtimeID)
		if err != nil {
			return err
		}
		return requireAffected(result, "INTERNAL_ERROR", "runtime is no longer current", http.StatusInternalServerError)
	})
}

func (s *Store) RequestStop(ctx context.Context, subjectID, reason string, now time.Time) (domain.Runtime, bool, error) {
	var out domain.Runtime
	queued := false
	err := s.withWrite(ctx, func(q dbtx) error {
		subject, err := getSubject(ctx, q, subjectID)
		if err != nil {
			return err
		}
		if subject.CurrentRuntimeID == nil {
			return domain.NewError("RUNTIME_NOT_FOUND", "subject has no current runtime", http.StatusNotFound)
		}
		out, err = getRuntime(ctx, q, subjectID, *subject.CurrentRuntimeID, true)
		if err != nil {
			return err
		}
		if out.State == domain.RuntimeQueued {
			queued = true
			t := formatTime(now)
			if _, err = q.ExecContext(ctx, `DELETE FROM runtime_queue WHERE runtime_id=?`, out.ID); err != nil {
				return err
			}
			if _, err = q.ExecContext(ctx, `UPDATE runtimes SET state='cancelled',ended_at=?,state_changed_at=?,end_reason=? WHERE id=? AND state='queued'`, t, t, reason, out.ID); err != nil {
				return err
			}
			if _, err = q.ExecContext(ctx, `UPDATE subjects SET state='idle',current_runtime_id=NULL,updated_at=? WHERE id=?`, t, subjectID); err != nil {
				return err
			}
			out.State = domain.RuntimeCancelled
			out.EndedAt = &now
			_, err = appendEvent(ctx, q, subjectID, &out.ID, "runtime.cancelled", "runtime", map[string]any{"reason": reason, "queued": true}, now, nil, nil)
			return err
		}
		if out.State == domain.RuntimeCancelled {
			return nil
		}
		if out.State.Terminal() {
			return domain.NewError("RUNTIME_STOPPING", "runtime is already stopping", http.StatusConflict)
		}
		t := formatTime(now)
		if _, err = q.ExecContext(ctx, `UPDATE runtimes SET state='cancelled',ended_at=?,state_changed_at=?,end_reason=? WHERE id=?`, t, t, reason, out.ID); err != nil {
			return err
		}
		out.State = domain.RuntimeCancelled
		out.EndedAt = &now
		_, err = appendEvent(ctx, q, subjectID, &out.ID, "runtime.cancelled", "runtime", map[string]any{"reason": reason, "queued": false}, now, nil, nil)
		return err
	})
	return out, queued, err
}

func (s *Store) StopQueued(ctx context.Context, subjectID, runtimeID, reason string, now time.Time) error {
	return s.withWrite(ctx, func(q dbtx) error {
		r, err := getRuntime(ctx, q, subjectID, runtimeID, false)
		if err != nil {
			return err
		}
		if r.State != domain.RuntimeQueued {
			return domain.NewError("RUNTIME_STOPPING", "runtime is not queued", http.StatusConflict)
		}
		if _, err = q.ExecContext(ctx, `DELETE FROM runtime_queue WHERE runtime_id=?`, runtimeID); err != nil {
			return err
		}
		t := formatTime(now)
		if _, err = q.ExecContext(ctx, `UPDATE runtimes SET state='cancelled',ended_at=?,state_changed_at=?,end_reason=? WHERE id=?`, t, t, reason, runtimeID); err != nil {
			return err
		}
		_, err = q.ExecContext(ctx, `UPDATE subjects SET state='idle',current_runtime_id=NULL,updated_at=? WHERE id=?`, t, subjectID)
		return err
	})
}

func (s *Store) SetClosing(ctx context.Context, subjectID, runtimeID string, deadline, now time.Time) error {
	return s.withWrite(ctx, func(q dbtx) error {
		r, err := q.ExecContext(ctx, `UPDATE runtimes SET closing=1,close_deadline=?,state_changed_at=? WHERE subject_id=? AND id=? AND state IN ('running','waiting') AND closing=0`, formatTime(deadline), formatTime(now), subjectID, runtimeID)
		if err != nil {
			return err
		}
		return requireAffected(r, "RUNTIME_CLOSING", "runtime is already closing", http.StatusConflict)
	})
}

func (s *Store) UpdateRuntimeLaunch(ctx context.Context, subjectID, runtimeID, configHash string, processInfo any, now time.Time) error {
	return s.withWrite(ctx, func(q dbtx) error {
		_, err := q.ExecContext(ctx, `UPDATE runtimes SET config_hash=?,process_json=?,state_changed_at=? WHERE subject_id=? AND id=?`, nullString(configHash), nullableJSON(processInfo), formatTime(now), subjectID, runtimeID)
		return err
	})
}

func (s *Store) QueuePosition(ctx context.Context, runtimeID string) (*int, error) {
	var sequence int64
	if err := s.db.QueryRowContext(ctx, `SELECT sequence FROM runtime_queue WHERE runtime_id=?`, runtimeID).Scan(&sequence); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var position int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_queue WHERE sequence<=?`, sequence).Scan(&position); err != nil {
		return nil, err
	}
	return &position, nil
}

func (s *Store) Capacity(ctx context.Context) (using, queuing int, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM subjects WHERE state='using'),(SELECT COUNT(*) FROM runtime_queue)`).Scan(&using, &queuing)
	return
}

func appendEvent(ctx context.Context, q dbtx, subjectID string, runtimeID *string, eventType, source string, payload any, now time.Time, rawSeq, index *uint64) (uint64, error) {
	var seq uint64
	if err := q.QueryRowContext(ctx, `UPDATE subjects SET last_event_seq=last_event_seq+1 WHERE id=? RETURNING last_event_seq`, subjectID).Scan(&seq); err != nil {
		return 0, err
	}
	_, err := q.ExecContext(ctx, `INSERT INTO events(subject_id,seq,runtime_id,type,timestamp,source,payload_json,origin_raw_seq,origin_index) VALUES(?,?,?,?,?,?,?,?,?)`, subjectID, seq, runtimeID, eventType, formatTime(now), source, payloadJSON(payload), rawSeq, index)
	return seq, err
}

func (s *Store) AppendEvent(ctx context.Context, subjectID string, runtimeID *string, eventType, source string, payload any, now time.Time) (domain.Event, error) {
	var seq uint64
	err := s.withWrite(ctx, func(q dbtx) error {
		var err error
		seq, err = appendEvent(ctx, q, subjectID, runtimeID, eventType, source, payload, now, nil, nil)
		return err
	})
	b, _ := json.Marshal(payload)
	return domain.Event{Seq: seq, Type: eventType, Timestamp: now.UTC(), RuntimeID: runtimeID, Source: source, Payload: b}, err
}

func (s *Store) NextRawSeq(ctx context.Context, subjectID string) (uint64, error) {
	var seq uint64
	err := s.withWrite(ctx, func(q dbtx) error {
		return q.QueryRowContext(ctx, `UPDATE subjects SET last_raw_seq=last_raw_seq+1 WHERE id=? RETURNING last_raw_seq`, subjectID).Scan(&seq)
	})
	return seq, err
}

// ProjectedEvent is an Event derived deterministically from one raw record.
type ProjectedEvent struct {
	Type    string
	Source  string
	Payload any
}

// ProjectRaw writes all Events derived from a raw record and advances its
// checkpoint in one transaction. build receives the first allocated Event
// sequence so payloads may reference another Event from the same raw record.
func (s *Store) ProjectRaw(ctx context.Context, subjectID, runtimeID string, rawSeq uint64, eventCount int, now time.Time, build func(uint64) []ProjectedEvent) ([]domain.Event, bool, error) {
	var events []domain.Event
	var projected bool
	err := s.withWrite(ctx, func(q dbtx) error {
		var checkpoint, lastRaw uint64
		if err := q.QueryRowContext(ctx, `SELECT last_projected_raw_seq,last_raw_seq FROM subjects WHERE id=?`, subjectID).Scan(&checkpoint, &lastRaw); err != nil {
			return err
		}
		if rawSeq > lastRaw {
			return fmt.Errorf("raw sequence %d exceeds allocated sequence %d", rawSeq, lastRaw)
		}
		if rawSeq <= checkpoint {
			rows, err := q.QueryContext(ctx, `SELECT seq,type,timestamp,runtime_id,source,payload_json FROM events WHERE subject_id=? AND origin_raw_seq=? ORDER BY origin_index`, subjectID, rawSeq)
			if err != nil {
				return err
			}
			for rows.Next() {
				event, err := scanEvent(rows)
				if err != nil {
					_ = rows.Close()
					return err
				}
				events = append(events, event)
			}
			if err = rows.Err(); err != nil {
				_ = rows.Close()
				return err
			}
			if err = rows.Close(); err != nil {
				return err
			}
			if len(events) != eventCount {
				return fmt.Errorf("stored raw projection has %d events, expected %d", len(events), eventCount)
			}
			if eventCount == 0 {
				return nil
			}
			built := build(events[0].Seq)
			if len(built) != eventCount {
				return fmt.Errorf("raw projector produced %d events, expected %d", len(built), eventCount)
			}
			for index, projectedEvent := range built {
				if projectedEvent.Type != events[index].Type || projectedEvent.Source != events[index].Source {
					return fmt.Errorf("stored raw projection shape differs at index %d", index)
				}
				payload := payloadJSON(projectedEvent.Payload)
				if string(events[index].Payload) == payload {
					continue
				}
				if _, err = q.ExecContext(ctx, `UPDATE events SET payload_json=? WHERE subject_id=? AND origin_raw_seq=? AND origin_index=?`, payload, subjectID, rawSeq, index); err != nil {
					return err
				}
				events[index].Payload = json.RawMessage(payload)
				projected = true
			}
			return nil
		}

		var firstSeq uint64
		if eventCount > 0 {
			var lastSeq uint64
			if err := q.QueryRowContext(ctx, `UPDATE subjects SET last_event_seq=last_event_seq+? WHERE id=? RETURNING last_event_seq`, eventCount, subjectID).Scan(&lastSeq); err != nil {
				return err
			}
			firstSeq = lastSeq - uint64(eventCount) + 1
		}
		built := build(firstSeq)
		if len(built) != eventCount {
			return fmt.Errorf("raw projector produced %d events, expected %d", len(built), eventCount)
		}
		for index, projectedEvent := range built {
			seq := firstSeq + uint64(index)
			payload := payloadJSON(projectedEvent.Payload)
			if _, err := q.ExecContext(ctx, `INSERT INTO events(subject_id,seq,runtime_id,type,timestamp,source,payload_json,origin_raw_seq,origin_index) VALUES(?,?,?,?,?,?,?,?,?)`, subjectID, seq, runtimeID, projectedEvent.Type, formatTime(now), projectedEvent.Source, payload, rawSeq, index); err != nil {
				return err
			}
			rid := runtimeID
			events = append(events, domain.Event{Seq: seq, Type: projectedEvent.Type, Timestamp: now.UTC(), RuntimeID: &rid, Source: projectedEvent.Source, Payload: json.RawMessage(payload)})
		}
		result, err := q.ExecContext(ctx, `UPDATE subjects SET last_projected_raw_seq=? WHERE id=? AND last_projected_raw_seq<?`, rawSeq, subjectID, rawSeq)
		if err != nil {
			return err
		}
		if err = requireAffected(result, "RAW_PROJECTION_OUT_OF_ORDER", "raw projection checkpoint did not advance", http.StatusInternalServerError); err != nil {
			return err
		}
		projected = true
		return nil
	})
	return events, projected, err
}

type rowScanner interface {
	Scan(...any) error
}

func scanEvent(row rowScanner) (domain.Event, error) {
	var event domain.Event
	var timestamp, payload string
	var runtimeID sql.NullString
	if err := row.Scan(&event.Seq, &event.Type, &timestamp, &runtimeID, &event.Source, &payload); err != nil {
		return event, err
	}
	event.Timestamp, _ = parseTime(timestamp)
	event.Payload = json.RawMessage(payload)
	if runtimeID.Valid {
		event.RuntimeID = &runtimeID.String
	}
	return event, nil
}

func (s *Store) UpdateRuntimeResource(ctx context.Context, subjectID, runtimeID string, resource any) error {
	return s.withWrite(ctx, func(q dbtx) error {
		_, err := q.ExecContext(ctx, `UPDATE runtimes SET resource_json=? WHERE subject_id=? AND id=?`, payloadJSON(resource), subjectID, runtimeID)
		return err
	})
}

func (s *Store) UpdateRuntimeTranscript(ctx context.Context, subjectID, runtimeID string, transcript any) error {
	return s.withWrite(ctx, func(q dbtx) error {
		_, err := q.ExecContext(ctx, `UPDATE runtimes SET transcript_json=? WHERE subject_id=? AND id=?`, payloadJSON(transcript), subjectID, runtimeID)
		return err
	})
}

func (s *Store) UpdateRuntimeProcess(ctx context.Context, subjectID, runtimeID string, process any) error {
	return s.withWrite(ctx, func(q dbtx) error {
		_, err := q.ExecContext(ctx, `UPDATE runtimes SET process_json=? WHERE subject_id=? AND id=?`, payloadJSON(process), subjectID, runtimeID)
		return err
	})
}

func (s *Store) ListEvents(ctx context.Context, subjectID string, limit int, after, before *uint64) ([]domain.Event, error) {
	query := `SELECT seq,type,timestamp,runtime_id,source,payload_json FROM events WHERE subject_id=?`
	args := []any{subjectID}
	order := "DESC"
	if after != nil {
		query += ` AND seq>?`
		args = append(args, *after)
		order = "ASC"
	}
	if before != nil {
		query += ` AND seq<?`
		args = append(args, *before)
	}
	query += ` ORDER BY seq ` + order + ` LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Event{}
	for rows.Next() {
		var e domain.Event
		var ts, payload string
		var rid sql.NullString
		if err := rows.Scan(&e.Seq, &e.Type, &ts, &rid, &e.Source, &payload); err != nil {
			return nil, err
		}
		e.Timestamp, _ = parseTime(ts)
		e.Payload = []byte(payload)
		if rid.Valid {
			e.RuntimeID = &rid.String
		}
		result = append(result, e)
	}
	if after == nil {
		for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
			result[i], result[j] = result[j], result[i]
		}
	}
	return result, rows.Err()
}

func (s *Store) ListQueue(ctx context.Context, limit int, after int64) ([]domain.Runtime, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_queue`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT subject_id,runtime_id FROM runtime_queue WHERE sequence>? ORDER BY sequence LIMIT ?`, after, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := []domain.Runtime{}
	for rows.Next() {
		var sid, rid string
		if err := rows.Scan(&sid, &rid); err != nil {
			return nil, 0, err
		}
		r, err := getRuntime(ctx, s.db, sid, rid, true)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, r)
	}
	return result, total, rows.Err()
}

// ListQueueEntries returns the projection used by the HTTP queue list.
func (s *Store) ListQueueEntries(ctx context.Context, limit int, after int64) ([]QueueEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH ranked AS (
			SELECT sequence,runtime_id,subject_id,queued_at,
				ROW_NUMBER() OVER (ORDER BY sequence) AS position
			FROM runtime_queue
		)
		SELECT q.sequence,q.position,q.runtime_id,q.subject_id,s.name,
			(SELECT COUNT(*) FROM runtime_instructions i WHERE i.runtime_id=q.runtime_id AND i.dispatch_committed_at IS NULL),
			q.queued_at
		FROM ranked q
		JOIN subjects s ON s.id=q.subject_id
		WHERE q.sequence>?
		ORDER BY q.sequence
		LIMIT ?`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []QueueEntry{}
	for rows.Next() {
		var item QueueEntry
		var name sql.NullString
		var queued string
		if err := rows.Scan(&item.Sequence, &item.Position, &item.RuntimeID, &item.SubjectID, &name, &item.InstructionCount, &queued); err != nil {
			return nil, err
		}
		if name.Valid {
			item.SubjectName = &name.String
		}
		item.QueuedAt, _ = parseTime(queued)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ListRuntimes(ctx context.Context, subjectID string, limit int, afterTime, afterID string) ([]domain.Runtime, error) {
	query := `SELECT id FROM runtimes WHERE subject_id=?`
	args := []any{subjectID}
	if afterTime != "" {
		query += ` AND (created_at<? OR (created_at=? AND id>?))`
		args = append(args, afterTime, afterTime, afterID)
	}
	query += ` ORDER BY created_at DESC,id ASC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Runtime{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		r, err := getRuntime(ctx, s.db, subjectID, id, false)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ListRuntimeSummaries returns the projection used by the HTTP runtime history list.
func (s *Store) ListRuntimeSummaries(ctx context.Context, subjectID string, limit int, afterTime, afterID string) ([]RuntimeSummary, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM subjects WHERE id=?`, subjectID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewError("SUBJECT_NOT_FOUND", "subject was not found", http.StatusNotFound)
		}
		return nil, err
	}
	query := `SELECT r.id,r.state,CASE WHEN r.id=s.current_runtime_id THEN 1 ELSE 0 END,r.created_at,r.started_at,r.ended_at,(SELECT COUNT(*) FROM runtimes total WHERE total.subject_id=s.id) FROM subjects s JOIN runtimes r ON r.subject_id=s.id WHERE s.id=?`
	args := []any{subjectID}
	if afterTime != "" {
		query += ` AND (r.created_at<? OR (r.created_at=? AND r.id>?))`
		args = append(args, afterTime, afterTime, afterID)
	}
	query += ` ORDER BY r.created_at DESC,r.id ASC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RuntimeSummary{}
	for rows.Next() {
		var item RuntimeSummary
		var current int
		var created string
		var started, ended sql.NullString
		if err := rows.Scan(&item.ID, &item.State, &current, &created, &started, &ended, &item.TotalCount); err != nil {
			return nil, err
		}
		item.Current = current != 0
		item.CreatedAt, _ = parseTime(created)
		if started.Valid {
			value, _ := parseTime(started.String)
			item.StartedAt = &value
		}
		if ended.Valid {
			value, _ := parseTime(ended.String)
			item.EndedAt = &value
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CurrentRuntimes(ctx context.Context) ([]domain.Runtime, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+runtimeColumns+` FROM subjects s JOIN runtimes r ON r.subject_id=s.id AND r.id=s.current_runtime_id WHERE s.current_runtime_id IS NOT NULL ORDER BY s.created_at`)
	if err != nil {
		return nil, err
	}
	var out []domain.Runtime
	indices := map[string]int{}
	for rows.Next() {
		var record runtimeRow
		if err := rows.Scan(record.destinations()...); err != nil {
			_ = rows.Close()
			return nil, err
		}
		record.value.Instructions = []domain.Instruction{}
		indices[record.value.ID] = len(out)
		out = append(out, record.runtime())
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	instructions, err := s.db.QueryContext(ctx, `SELECT i.runtime_id,i.sequence,i.content,i.created_at
		FROM subjects s JOIN runtime_instructions i ON i.runtime_id=s.current_runtime_id
		WHERE s.current_runtime_id IS NOT NULL AND i.dispatch_committed_at IS NULL
		ORDER BY s.created_at,i.sequence`)
	if err != nil {
		return nil, err
	}
	defer instructions.Close()
	for instructions.Next() {
		var runtimeID, created string
		var item domain.Instruction
		if err := instructions.Scan(&runtimeID, &item.Sequence, &item.Content, &created); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = parseTime(created)
		if index, ok := indices[runtimeID]; ok {
			out[index].Instructions = append(out[index].Instructions, item)
		}
	}
	return out, instructions.Err()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func nullableJSON(v any) any {
	if v == nil {
		return nil
	}
	return payloadJSON(v)
}
func requireAffected(r sql.Result, code, message string, status int) error {
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.NewError(code, message, status)
	}
	return nil
}
func mapSQLError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint failed") {
		return domain.Wrap("INTERNAL_ERROR", "database constraint rejected the operation", http.StatusInternalServerError, err)
	}
	return err
}
