package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/Ghr07h/norns/norns/internal/domain"
	"github.com/Ghr07h/norns/norns/internal/version"
	"github.com/Ghr07h/norns/norns/internal/workspace"
)

//go:embed schema.sql
var schemaSQL string

type Store struct {
	Paths workspace.Paths
	IDs   domain.IDGenerator
	Clock domain.Clock
}

func New(paths workspace.Paths, ids domain.IDGenerator, clock domain.Clock) *Store {
	return &Store{Paths: paths, IDs: ids, Clock: clock}
}

func (s *Store) Initialized() bool {
	info, err := os.Stat(s.Paths.Database)
	return err == nil && info.Mode().IsRegular()
}

func (s *Store) Initialize(ctx context.Context, setup func(*sql.Tx) error) error {
	if s.Initialized() {
		return domain.NewError("INVALID_STATE", "Workspace is already initialized")
	}
	if err := os.MkdirAll(s.Paths.Norns, 0o700); err != nil {
		return domain.Wrap("IO_ERROR", "cannot create .norns directory", err)
	}
	token, err := s.IDs.Token()
	if err != nil {
		return domain.Wrap("INTERNAL_ERROR", "cannot generate staging token", err)
	}
	stage := filepath.Join(s.Paths.Norns, ".norns.sqlite.stage."+token)
	defer os.Remove(stage)
	defer os.Remove(stage + "-journal")

	db, err := sql.Open("sqlite", stage)
	if err != nil {
		return domain.Wrap("DATABASE_ERROR", "cannot open staging database", err)
	}
	defer db.Close()
	if err := configure(db, false); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		return domain.Wrap("DATABASE_ERROR", "cannot create database schema", err)
	}
	if err := os.Chmod(stage, 0o600); err != nil {
		return domain.Wrap("IO_ERROR", "cannot set staging database permissions", err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id = %d", version.SQLiteAppID)); err != nil {
		return domain.Wrap("DATABASE_ERROR", "cannot set database application ID", err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version.SchemaVersion)); err != nil {
		return domain.Wrap("DATABASE_ERROR", "cannot set schema version", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Wrap("DATABASE_ERROR", "cannot begin initialization transaction", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_meta(singleton, protocol_version, runtime_occupied, created_at) VALUES (1, ?, 0, ?)`, version.ProtocolVersion, domain.TimeText(s.Clock.Now())); err != nil {
		return domain.Wrap("DATABASE_ERROR", "cannot create Workspace metadata", err)
	}
	if setup != nil {
		if err := setup(tx); err != nil {
			return err
		}
	}
	if err := Validate(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return domain.Wrap("DATABASE_ERROR", "cannot commit initialization", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return domain.Wrap("DATABASE_ERROR", "cannot checkpoint staging database", err)
	}
	if err := db.Close(); err != nil {
		return domain.Wrap("DATABASE_ERROR", "cannot close staging database", err)
	}
	if err := workspace.SyncFile(stage); err != nil {
		return domain.Wrap("IO_ERROR", "cannot sync staging database", err)
	}
	if s.Initialized() {
		return domain.NewError("INVALID_STATE", "Workspace was initialized concurrently")
	}
	if err := os.Rename(stage, s.Paths.Database); err != nil {
		return domain.Wrap("IO_ERROR", "cannot publish Workspace database", err)
	}
	if err := workspace.SyncDir(s.Paths.Norns); err != nil {
		return domain.Wrap("IO_ERROR", "cannot sync .norns directory", err)
	}
	return nil
}

func configure(db *sql.DB, readOnly bool) error {
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return domain.Wrap("DATABASE_ERROR", "cannot enable foreign keys", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		return domain.Wrap("DATABASE_ERROR", "cannot set SQLite busy timeout", err)
	}
	if !readOnly {
		if _, err := db.Exec(`PRAGMA synchronous = FULL`); err != nil {
			return domain.Wrap("DATABASE_ERROR", "cannot set SQLite synchronous mode", err)
		}
	}
	return nil
}

func (s *Store) probe(ctx context.Context) error {
	if !s.Initialized() {
		return domain.NewError("WORKSPACE_NOT_INITIALIZED", "Workspace is not initialized; run norns workspace init")
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(s.Paths.Database)+"?mode=ro")
	if err != nil {
		return domain.Wrap("DATABASE_ERROR", "cannot open Workspace database", err)
	}
	defer db.Close()
	if err := configure(db, true); err != nil {
		return err
	}
	var appID, schemaVersion int
	if err := db.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&appID); err != nil {
		return domain.Wrap("INVALID_DATABASE", "cannot read database application ID; try norns workspace reset", err)
	}
	if appID != version.SQLiteAppID {
		return domain.NewError("INVALID_DATABASE", "database application ID is invalid; try norns workspace reset")
	}
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&schemaVersion); err != nil {
		return domain.Wrap("INVALID_DATABASE", "cannot read schema version", err)
	}
	if schemaVersion != version.SchemaVersion {
		return domain.Errorf("UNSUPPORTED_SCHEMA_VERSION", "unsupported schema version %d", schemaVersion)
	}
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name IN ('workspace_meta','tasks','cognitions','explorations','exploration_sources')`)
	if err != nil {
		return domain.Wrap("INVALID_DATABASE", "cannot inspect database schema", err)
	}
	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return domain.Wrap("INVALID_DATABASE", "cannot inspect database schema", err)
		}
		tables[name] = true
	}
	if err := rows.Close(); err != nil {
		return domain.Wrap("INVALID_DATABASE", "cannot inspect database schema", err)
	}
	for _, name := range []string{"workspace_meta", "tasks", "cognitions", "explorations", "exploration_sources"} {
		if !tables[name] {
			return domain.NewError("INVALID_DATABASE", "Workspace database is missing required tables; try norns workspace reset")
		}
	}
	var protocolVersion int
	if err := db.QueryRowContext(ctx, `SELECT protocol_version FROM workspace_meta WHERE singleton=1`).Scan(&protocolVersion); err != nil {
		return domain.Wrap("INVALID_DATABASE", "Workspace metadata is incomplete; try norns workspace reset", err)
	}
	if protocolVersion != version.ProtocolVersion {
		return domain.Errorf("UNSUPPORTED_PROTOCOL_VERSION", "unsupported protocol version %d", protocolVersion)
	}
	return nil
}

func (s *Store) OpenRead(ctx context.Context) (*sql.DB, error) {
	if err := s.probe(ctx); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(s.Paths.Database)+"?mode=ro")
	if err != nil {
		return nil, domain.Wrap("DATABASE_ERROR", "cannot open read-only database", err)
	}
	if err := configure(db, true); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s *Store) OpenWrite(ctx context.Context) (*sql.DB, error) {
	if err := s.probe(ctx); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(s.Paths.Database)+"?mode=rw")
	if err != nil {
		return nil, domain.Wrap("DATABASE_ERROR", "cannot open writable database", err)
	}
	if err := configure(db, false); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode = WAL`); err != nil {
		db.Close()
		return nil, domain.Wrap("DATABASE_ERROR", "cannot enable WAL mode", err)
	}
	return db, nil
}

func (s *Store) Read(ctx context.Context, fn func(*sql.Tx) error) error {
	db, err := s.OpenRead(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return mapDBError(err)
	}
	defer tx.Rollback()
	if err := Validate(ctx, tx); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return mapDBError(err)
	}
	return nil
}

// database/sql cannot attach a *sql.Tx to an already-issued BEGIN IMMEDIATE.
// withImmediate exposes the same query surface while keeping one connection.
type txFromConn struct{ conn *sql.Conn }

func (t txFromConn) ExecContext(ctx context.Context, q string, a ...any) (sql.Result, error) {
	return t.conn.ExecContext(ctx, q, a...)
}
func (t txFromConn) QueryContext(ctx context.Context, q string, a ...any) (*sql.Rows, error) {
	return t.conn.QueryContext(ctx, q, a...)
}
func (t txFromConn) QueryRowContext(ctx context.Context, q string, a ...any) *sql.Row {
	return t.conn.QueryRowContext(ctx, q, a...)
}

type Queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) WriteQ(ctx context.Context, fn func(Queryer) error) error {
	db, err := s.OpenWrite(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return mapDBError(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return mapDBError(err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	q := txFromConn{conn: conn}
	if err := Validate(ctx, q); err != nil {
		return err
	}
	if err := fn(q); err != nil {
		return err
	}
	if err := Validate(ctx, q); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return mapDBError(err)
	}
	committed = true
	return nil
}

func mapDBError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "locked") || strings.Contains(msg, "busy") {
		e := domain.Wrap("WORKSPACE_BUSY", "SQLite writer is busy", err)
		e.Details["resource"] = "sqlite_write"
		return e
	}
	if strings.Contains(msg, "constraint") {
		return domain.Wrap("INVALID_STATE", "operation violates Workspace constraints", err)
	}
	return domain.Wrap("DATABASE_ERROR", "database operation failed", err)
}

func Validate(ctx context.Context, q Queryer) error {
	rows, err := q.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return domain.Wrap("INVALID_DATABASE", "cannot validate foreign keys", err)
	}
	if rows.Next() {
		rows.Close()
		return domain.NewError("WORKSPACE_STATE_UNTRUSTED", "Workspace contains invalid foreign-key references")
	}
	if err := rows.Close(); err != nil {
		return domain.Wrap("INVALID_DATABASE", "cannot validate foreign keys", err)
	}
	var metaCount, runtime, openCount int
	if err := q.QueryRowContext(ctx, `SELECT count(*), COALESCE(max(runtime_occupied),0) FROM workspace_meta`).Scan(&metaCount, &runtime); err != nil {
		return domain.Wrap("INVALID_DATABASE", "Workspace metadata is incomplete", err)
	}
	if metaCount != 1 {
		return domain.NewError("WORKSPACE_STATE_UNTRUSTED", "Workspace metadata singleton is invalid")
	}
	if runtime != 0 && runtime != 1 {
		return domain.NewError("WORKSPACE_STATE_UNTRUSTED", "Runtime occupancy value is invalid")
	}
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM tasks WHERE status='open'`).Scan(&openCount); err != nil {
		return mapDBError(err)
	}
	if openCount > 1 {
		return domain.NewError("WORKSPACE_STATE_UNTRUSTED", "Workspace contains more than one open Task")
	}
	if (openCount == 1) != (runtime == 1) {
		return domain.NewError("WORKSPACE_STATE_UNTRUSTED", "Runtime occupancy does not match the open Task; manual Workspace inspection and handling is required")
	}
	checks := []struct {
		query   string
		message string
	}{
		{`SELECT count(*) FROM tasks WHERE length(trim(purpose))=0 OR max_concurrency<1 OR
			(status='open' AND (outcome IS NOT NULL OR outcome_reason IS NOT NULL OR closed_at IS NOT NULL)) OR
			(status='closed' AND (outcome NOT IN ('succeeded','unachievable','failed','cancelled') OR outcome_reason IS NULL OR length(trim(outcome_reason))=0 OR closed_at IS NULL)) OR
			status NOT IN ('open','closed')`, "Task fields violate the lifecycle state matrix"},
		{`SELECT count(*) FROM tasks t WHERE NOT EXISTS (
			SELECT 1 FROM cognitions c WHERE c.id=t.init_cognition_ref AND c.origin='task')`, "Task init does not refer to a Task-init Cognition"},
		{`SELECT count(*) FROM cognitions WHERE length(trim(title))=0 OR length(trim(content))=0 OR task_purpose_relevant NOT IN (0,1) OR
			(origin='task' AND (task_id IS NULL OR origin_exploration_id IS NOT NULL OR task_purpose_relevant<>0)) OR
			(origin='user' AND NOT ((task_id IS NULL AND origin_cognition_id IS NOT NULL AND origin_exploration_id IS NULL AND task_purpose_relevant=0) OR
			                         (task_id IS NOT NULL AND origin_cognition_id IS NULL AND origin_exploration_id IS NOT NULL))) OR
			(origin='exploration' AND (task_id IS NULL OR origin_cognition_id IS NOT NULL OR origin_exploration_id IS NULL)) OR
			origin NOT IN ('task','user','exploration')`, "Cognition fields violate the origin matrix"},
		{`SELECT count(*) FROM cognitions c WHERE c.origin='task' AND NOT EXISTS (
			SELECT 1 FROM tasks t WHERE t.id=c.task_id AND t.init_cognition_ref=c.id)`, "Task-init Cognition ownership is invalid"},
		{`SELECT count(*) FROM cognitions c JOIN explorations e ON e.id=c.origin_exploration_id
			WHERE c.origin='exploration' AND c.task_id<>e.task_id`, "Explorer Cognition belongs to a different Task"},
		{`SELECT count(*) FROM cognitions c WHERE c.origin='task' AND (
			(NOT EXISTS (SELECT 1 FROM cognitions earlier WHERE earlier.origin='task' AND earlier.rowid<c.rowid)
			 AND c.origin_cognition_id IS NOT NULL) OR
			(EXISTS (SELECT 1 FROM cognitions earlier WHERE earlier.origin='task' AND earlier.rowid<c.rowid)
			 AND (c.origin_cognition_id IS NULL OR c.origin_cognition_id<>(
				SELECT expected.id FROM cognitions expected
				WHERE expected.origin='task' AND expected.rowid<c.rowid
				ORDER BY expected.created_at DESC,expected.id ASC LIMIT 1))))`, "Task-init Cognition predecessor does not match the creation-time selection rule"},
		{`SELECT count(*) FROM explorations WHERE length(trim(title))=0 OR length(trim(content))=0 OR
			(status='todo' AND (executor_id IS NOT NULL OR output_ref IS NOT NULL OR assigned_at IS NOT NULL OR outcome IS NOT NULL OR outcome_reason IS NOT NULL OR finished_at IS NOT NULL)) OR
			(status='running' AND (executor_id IS NULL OR length(trim(executor_id))=0 OR output_ref IS NULL OR length(trim(output_ref))=0 OR assigned_at IS NULL OR outcome IS NOT NULL OR outcome_reason IS NOT NULL OR finished_at IS NOT NULL)) OR
			(status='finished' AND (outcome NOT IN ('succeeded','inconclusive','failed','cancelled') OR outcome_reason IS NULL OR length(trim(outcome_reason))=0 OR finished_at IS NULL OR
			 ((executor_id IS NULL OR output_ref IS NULL OR assigned_at IS NULL) AND NOT (executor_id IS NULL AND output_ref IS NULL AND assigned_at IS NULL AND outcome='cancelled')))) OR
			status NOT IN ('todo','running','finished')`, "Exploration fields violate the lifecycle state matrix"},
		{`SELECT count(*) FROM explorations e WHERE NOT EXISTS (SELECT 1 FROM exploration_sources s WHERE s.exploration_id=e.id)`, "Exploration has no source Cognition"},
		{`SELECT count(*) FROM explorations e WHERE EXISTS (SELECT 1 FROM exploration_sources s WHERE s.exploration_id=e.id)
			AND (SELECT min(ordinal) FROM exploration_sources s WHERE s.exploration_id=e.id)<>0`, "Exploration source ordinals do not start at zero"},
		{`SELECT count(*) FROM explorations e WHERE (SELECT count(*) FROM exploration_sources s WHERE s.exploration_id=e.id)<>
			(SELECT max(ordinal)+1 FROM exploration_sources s WHERE s.exploration_id=e.id)`, "Exploration source ordinals are not contiguous"},
		{`SELECT count(*) FROM explorations e JOIN explorations p ON p.id=e.retry_of
			WHERE p.status<>'finished' OR e.title<>p.title OR e.content<>p.content OR
			EXISTS (SELECT cognition_id,ordinal FROM exploration_sources WHERE exploration_id=e.id
			        EXCEPT SELECT cognition_id,ordinal FROM exploration_sources WHERE exploration_id=p.id) OR
			EXISTS (SELECT cognition_id,ordinal FROM exploration_sources WHERE exploration_id=p.id
			        EXCEPT SELECT cognition_id,ordinal FROM exploration_sources WHERE exploration_id=e.id)`, "retry Exploration does not preserve its finished source definition"},
		{`SELECT count(*) FROM tasks t WHERE t.status='closed' AND EXISTS (
			SELECT 1 FROM explorations e WHERE e.task_id=t.id AND e.status<>'finished')`, "closed Task contains unfinished Exploration"},
	}
	for _, check := range checks {
		if err := rejectCount(ctx, q, check.query, check.message); err != nil {
			return err
		}
	}
	if err := validateDAG(ctx, q); err != nil {
		return err
	}
	if err := validateIdentifiersAndTimes(ctx, q); err != nil {
		return err
	}
	return nil
}

func rejectCount(ctx context.Context, q Queryer, query, message string) error {
	var count int
	if err := q.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return mapDBError(err)
	}
	if count != 0 {
		return domain.NewError("WORKSPACE_STATE_UNTRUSTED", message)
	}
	return nil
}

func validateIdentifiersAndTimes(ctx context.Context, q Queryer) error {
	entities := []struct {
		query  string
		prefix string
	}{
		{`SELECT id FROM tasks`, "task"},
		{`SELECT id FROM cognitions`, "cognition"},
		{`SELECT id FROM explorations`, "exploration"},
	}
	for _, entity := range entities {
		rows, err := q.QueryContext(ctx, entity.query)
		if err != nil {
			return mapDBError(err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return mapDBError(err)
			}
			if err := domain.ValidateID(id, entity.prefix); err != nil {
				rows.Close()
				return domain.NewError("WORKSPACE_STATE_UNTRUSTED", "Workspace contains an invalid entity ID")
			}
		}
		if err := rows.Close(); err != nil {
			return mapDBError(err)
		}
	}
	timeQueries := []string{
		`SELECT created_at FROM workspace_meta`,
		`SELECT created_at FROM tasks UNION ALL SELECT closed_at FROM tasks WHERE closed_at IS NOT NULL`,
		`SELECT created_at FROM cognitions`,
		`SELECT created_at FROM explorations UNION ALL SELECT assigned_at FROM explorations WHERE assigned_at IS NOT NULL UNION ALL SELECT finished_at FROM explorations WHERE finished_at IS NOT NULL`,
	}
	for _, query := range timeQueries {
		rows, err := q.QueryContext(ctx, query)
		if err != nil {
			return mapDBError(err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				return mapDBError(err)
			}
			parsed, err := domain.ParseTime(value)
			if err != nil || domain.TimeText(parsed) != value {
				rows.Close()
				return domain.NewError("WORKSPACE_STATE_UNTRUSTED", "Workspace contains a non-canonical timestamp")
			}
		}
		if err := rows.Close(); err != nil {
			return mapDBError(err)
		}
	}
	return nil
}

type edge struct{ from, to string }

func validateDAG(ctx context.Context, q Queryer) error {
	edges := []edge{}
	queries := []string{
		`SELECT cognition_id, exploration_id FROM exploration_sources`,
		`SELECT origin_cognition_id, id FROM cognitions WHERE origin_cognition_id IS NOT NULL`,
		`SELECT origin_exploration_id, id FROM cognitions WHERE origin_exploration_id IS NOT NULL`,
	}
	for _, query := range queries {
		rows, err := q.QueryContext(ctx, query)
		if err != nil {
			return mapDBError(err)
		}
		for rows.Next() {
			var e edge
			if err := rows.Scan(&e.from, &e.to); err != nil {
				rows.Close()
				return mapDBError(err)
			}
			edges = append(edges, e)
		}
		if err := rows.Close(); err != nil {
			return mapDBError(err)
		}
	}
	if cyclic(edges) {
		return domain.NewError("WORKSPACE_STATE_UNTRUSTED", "Graph contains a cycle")
	}
	rows, err := q.QueryContext(ctx, `SELECT retry_of, id FROM explorations WHERE retry_of IS NOT NULL`)
	if err != nil {
		return mapDBError(err)
	}
	edges = edges[:0]
	for rows.Next() {
		var e edge
		if err := rows.Scan(&e.from, &e.to); err != nil {
			rows.Close()
			return mapDBError(err)
		}
		edges = append(edges, e)
	}
	if err := rows.Close(); err != nil {
		return mapDBError(err)
	}
	if cyclic(edges) {
		return domain.NewError("WORKSPACE_STATE_UNTRUSTED", "retry chain contains a cycle")
	}
	return nil
}

func cyclic(edges []edge) bool {
	adj := map[string][]string{}
	indegree := map[string]int{}
	for _, e := range edges {
		adj[e.from] = append(adj[e.from], e.to)
		indegree[e.to]++
		if _, ok := indegree[e.from]; !ok {
			indegree[e.from] = 0
		}
	}
	queue := []string{}
	for n, d := range indegree {
		if d == 0 {
			queue = append(queue, n)
		}
	}
	seen := 0
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		seen++
		for _, m := range adj[n] {
			indegree[m]--
			if indegree[m] == 0 {
				queue = append(queue, m)
			}
		}
	}
	return seen != len(indegree)
}

func IsNotFound(err error) bool { return errors.Is(err, sql.ErrNoRows) }
