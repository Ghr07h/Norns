package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Ghr07h/norns/norns/internal/domain"
	"github.com/Ghr07h/norns/norns/internal/version"
	"github.com/Ghr07h/norns/norns/internal/workspace"
)

type fixedClock struct{ value time.Time }

func (c fixedClock) Now() time.Time { return c.value }

func TestReadRejectsUntrustedRuntimeState(t *testing.T) {
	ctx := context.Background()
	paths, err := workspace.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := New(paths, domain.RandomIDs{}, fixedClock{time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)})
	if err := store.Initialize(ctx, nil); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(paths.Database)+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA ignore_check_constraints=ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE workspace_meta SET runtime_occupied=2`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	err = store.Read(ctx, func(*sql.Tx) error { return nil })
	var typed *domain.Error
	if !errors.As(err, &typed) || typed.Code != "WORKSPACE_STATE_UNTRUSTED" {
		t.Fatalf("expected WORKSPACE_STATE_UNTRUSTED, got %#v", err)
	}
}

func TestProbeRejectsDatabaseWithMissingTables(t *testing.T) {
	ctx := context.Background()
	paths, err := workspace.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := osMkdir(paths.Norns); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE workspace_meta(singleton INTEGER PRIMARY KEY, protocol_version INTEGER, runtime_occupied INTEGER, created_at TEXT)`,
		`INSERT INTO workspace_meta VALUES(1,1,0,'2026-10-12T00:00:00Z')`,
		`PRAGMA application_id=` + intText(version.SQLiteAppID),
		`PRAGMA user_version=` + intText(version.SchemaVersion),
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store := New(paths, domain.RandomIDs{}, fixedClock{})
	err = store.Read(ctx, func(*sql.Tx) error { return nil })
	var typed *domain.Error
	if !errors.As(err, &typed) || typed.Code != "INVALID_DATABASE" {
		t.Fatalf("expected INVALID_DATABASE, got %#v", err)
	}
}

func TestUnsupportedVersionsNeverEnterWriteCallback(t *testing.T) {
	for _, test := range []struct {
		name      string
		statement string
		code      string
	}{
		{"schema", `PRAGMA user_version=99`, "UNSUPPORTED_SCHEMA_VERSION"},
		{"protocol", `UPDATE workspace_meta SET protocol_version=99`, "UNSUPPORTED_PROTOCOL_VERSION"},
		{"application-id", `PRAGMA application_id=1`, "INVALID_DATABASE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			paths, err := workspace.Resolve(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			store := New(paths, domain.RandomIDs{}, fixedClock{time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)})
			if err := store.Initialize(ctx, nil); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", paths.Database)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(test.statement); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			called := false
			err = store.WriteQ(ctx, func(Queryer) error {
				called = true
				return nil
			})
			var typed *domain.Error
			if !errors.As(err, &typed) || typed.Code != test.code || called {
				t.Fatalf("code=%v called=%v err=%#v", test.code, called, err)
			}
		})
	}
}

func TestWriteQRollsBackCallbackFailure(t *testing.T) {
	ctx := context.Background()
	paths, err := workspace.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := New(paths, domain.RandomIDs{}, fixedClock{time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)})
	if err := store.Initialize(ctx, nil); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("stop")
	err = store.WriteQ(ctx, func(q Queryer) error {
		if _, err := q.ExecContext(ctx, `UPDATE workspace_meta SET runtime_occupied=1`); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("unexpected write error: %v", err)
	}
	db, err := sql.Open("sqlite", paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var occupied int
	if err := db.QueryRow(`SELECT runtime_occupied FROM workspace_meta`).Scan(&occupied); err != nil {
		t.Fatal(err)
	}
	if occupied != 0 {
		t.Fatalf("callback mutation was not rolled back: %d", occupied)
	}
}

func osMkdir(path string) error { return os.MkdirAll(path, 0o700) }

func intText(value int) string { return strconv.Itoa(value) }
