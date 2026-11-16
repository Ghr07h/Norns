package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/google/uuid"
)

func transactionStore(t *testing.T) *Store {
	t.Helper()
	store, _, err := Open(context.Background(), filepath.Join(t.TempDir(), "nornsd.sqlite"), uuid.NewString(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	store.db.SetMaxOpenConns(1)
	return store
}

func TestFailedBeginDiscardsPoisonedConnection(t *testing.T) {
	store := transactionStore(t)
	ctx := context.Background()
	conn, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, `CREATE TABLE uncommitted_marker(value TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	err = store.withWrite(ctx, func(dbtx) error { t.Error("write ran on an existing transaction"); return nil })
	if err == nil || domain.AsError(err).Details["sqlite_code"] == nil {
		t.Fatalf("expected SQLite transaction error with code, got %v", err)
	}
	if reason, ok := domain.AsError(err).Details["sqlite_error"].(string); !ok || !strings.Contains(reason, "within a transaction") {
		t.Fatalf("transaction response omitted the SQLite cause: %v", domain.AsError(err).Details)
	}
	if err = store.Health(ctx); err != nil {
		t.Fatalf("failed BEGIN left a poisoned connection in the pool: %v", err)
	}
	var count int
	if err = store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name='uncommitted_marker'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("discarded connection retained uncommitted data: count=%d err=%v", count, err)
	}
}

func connectionIdentity(t *testing.T, store *Store) any {
	t.Helper()
	conn, err := store.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var identity any
	if err = conn.Raw(func(raw any) error { identity = raw; return nil }); err != nil {
		t.Fatal(err)
	}
	return identity
}

func TestFailedRollbackDiscardsConnectionAndPreservesCause(t *testing.T) {
	store := transactionStore(t)
	before := connectionIdentity(t, store)
	failure := errors.New("write failed before rollback")
	err := store.withWrite(context.Background(), func(conn dbtx) error {
		if _, rollbackErr := conn.ExecContext(context.Background(), `ROLLBACK`); rollbackErr != nil {
			t.Fatal(rollbackErr)
		}
		return failure
	})
	if !errors.Is(err, failure) || domain.AsError(err).Code != "SERVICE_UNAVAILABLE" {
		t.Fatalf("rollback failure lost the original error: %v", err)
	}
	if before == connectionIdentity(t, store) {
		t.Fatal("failed rollback returned the connection to the pool")
	}
	if err = store.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCancelledWriteRollsBackWithIndependentContext(t *testing.T) {
	store := transactionStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := store.withWrite(ctx, func(conn dbtx) error {
		if _, createErr := conn.ExecContext(ctx, `CREATE TABLE cancelled_marker(value TEXT)`); createErr != nil {
			t.Fatal(createErr)
		}
		cancel()
		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if err = store.Health(context.Background()); err != nil {
		t.Fatalf("cancelled write poisoned the connection: %v", err)
	}
	var count int
	if err = store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='cancelled_marker'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cancelled transaction was not rolled back: count=%d err=%v", count, err)
	}
}

func TestHealthChecksWriteTransactionAvailability(t *testing.T) {
	store := transactionStore(t)
	store.db.SetMaxOpenConns(2)
	ctx := context.Background()
	blocker, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	probe, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = probe.ExecContext(ctx, `PRAGMA busy_timeout=20`); err != nil {
		t.Fatal(err)
	}
	_ = probe.Close()
	if _, err = blocker.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	defer blocker.ExecContext(ctx, `ROLLBACK`)
	if err = store.Health(ctx); err == nil || domain.AsError(err).Details["sqlite_code"] == nil {
		t.Fatalf("health ignored an unavailable write transaction: %v", err)
	}
	if reason, ok := domain.AsError(err).Details["sqlite_error"].(string); !ok || !strings.Contains(reason, "database is locked") {
		t.Fatalf("health response omitted the SQLite cause: %v", domain.AsError(err).Details)
	}
	if _, err = blocker.ExecContext(ctx, `ROLLBACK`); err != nil {
		t.Fatal(err)
	}
	if err = store.Health(ctx); err != nil {
		t.Fatalf("health did not recover after releasing the write lock: %v", err)
	}
}
