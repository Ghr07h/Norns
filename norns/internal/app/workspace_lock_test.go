package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/domain"
)

func TestWorkspacePreflightRejectsMissingMarkersBeforeLocking(t *testing.T) {
	for _, missing := range []string{"directory", "database", "regular-database"} {
		t.Run(missing, func(t *testing.T) {
			service, _ := newTestService(t)
			if missing != "directory" {
				if err := os.Mkdir(service.Paths.Norns, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if missing == "regular-database" {
				if err := os.Mkdir(service.Paths.Database, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(service.Paths.Lock, 0o700); err != nil {
				t.Fatal(err)
			}
			if _, _, err := service.StatusView(context.Background(), nil, true); errorCode(err) != "WORKSPACE_NOT_INITIALIZED" {
				t.Fatalf("query accessed the unusable lock instead of checking markers: %v", err)
			}
			if _, err := service.Reset(context.Background()); errorCode(err) != "WORKSPACE_NOT_INITIALIZED" {
				t.Fatalf("reset accessed the unusable lock instead of checking markers: %v", err)
			}
		})
	}
}

func TestWorkspaceOperationsCleanLocksOnSuccessAndFailure(t *testing.T) {
	ctx := context.Background()
	for _, initialize := range []string{"init", "task-open"} {
		t.Run(initialize, func(t *testing.T) {
			service, _ := newTestService(t)
			var err error
			if initialize == "init" {
				_, err = service.Init(ctx)
			} else {
				info := "Subject"
				_, err = service.OpenTask(ctx, OpenTaskRequest{Purpose: "Work", Concurrency: 1, Info: &info})
			}
			if err != nil {
				t.Fatal(err)
			}
			resetCode, openCode := "", "INVALID_ARGUMENT"
			if initialize == "task-open" {
				resetCode, openCode = "RESET_OPEN_TASK", "OPEN_TASK_EXISTS"
			}
			operations := []struct {
				call func() error
				code string
			}{
				{func() error { _, _, err := service.StatusView(ctx, nil, true); return err }, ""},
				{func() error { _, err := service.Reset(ctx); return err }, resetCode},
				{func() error {
					_, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Missing info", Concurrency: 1})
					return err
				}, openCode},
			}
			for _, operation := range operations {
				if _, err := os.Stat(service.Paths.Lock); !os.IsNotExist(err) {
					t.Fatalf("previous operation left a lock file: %v", err)
				}
				if err := operation.call(); errorCode(err) != operation.code || err != nil && operation.code == "" {
					t.Fatalf("operation returned %v, want code %q", err, operation.code)
				}
			}
			if _, err := os.Stat(service.Paths.Lock); !os.IsNotExist(err) {
				t.Fatalf("last operation left a lock file: %v", err)
			}
		})
	}
}

func TestWorkspaceOperationReleasesLockAfterPanic(t *testing.T) {
	for _, shared := range []bool{true, false} {
		service, _ := newTestService(t)
		if _, err := service.Init(context.Background()); err != nil {
			t.Fatal(err)
		}
		operation := service.withExclusive
		if shared {
			operation = service.shared
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != "operation panic" {
					t.Fatalf("unexpected panic: %v", recovered)
				}
			}()
			_ = operation(context.Background(), func(context.Context) error {
				if _, err := os.Stat(service.Paths.Lock); err != nil {
					t.Fatalf("operation has no lifecycle lock: %v", err)
				}
				panic("operation panic")
			})
		}()
		if _, err := os.Stat(service.Paths.Lock); !os.IsNotExist(err) {
			t.Fatalf("panicking operation left a lock file: %v", err)
		}
	}
}

func TestWorkspaceCorruptDatabaseStillRequiresValidation(t *testing.T) {
	service, _ := newTestService(t)
	if err := os.Mkdir(service.Paths.Norns, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(service.Paths.Database, []byte("not a SQLite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.StatusView(context.Background(), nil, true); err == nil || errorCode(err) == "WORKSPACE_NOT_INITIALIZED" {
		t.Fatalf("existing markers bypassed database validation: %v", err)
	}
	if _, err := os.Stat(service.Paths.Lock); !os.IsNotExist(err) {
		t.Fatalf("failed database validation left a lock file: %v", err)
	}
}

func TestWorkspaceQueryRecoversResetWithoutCurrentDirectory(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	createClosedTask(t, service)
	journal, _ := stageReset(t, service, "prepared", "00000000000000e5")
	if err := os.Rename(service.Paths.Norns, filepath.Join(service.Paths.Root, journal.OldRoot)); err != nil {
		t.Fatal(err)
	}
	journal.Phase = "old_renamed"
	if err := service.writeResetJournal(journal); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.StatusView(ctx, nil, true); err != nil {
		t.Fatalf("preflight blocked reset recovery: %v", err)
	}
	for _, path := range []string{service.Paths.Lock, service.Paths.ResetLog} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("recovery left %s: %v", path, err)
		}
	}
}

func TestWorkspaceLockCleanupPreservesReplacementAndErrors(t *testing.T) {
	for _, shared := range []bool{true, false} {
		service, _ := newTestService(t)
		if _, err := service.Init(context.Background()); err != nil {
			t.Fatal(err)
		}
		operation := service.withExclusive
		if shared {
			operation = service.shared
		}
		err := operation(context.Background(), func(context.Context) error {
			if err := os.Rename(service.Paths.Lock, service.Paths.Lock+".old"); err != nil {
				return err
			}
			if err := os.Mkdir(service.Paths.Lock, 0o700); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cleanup should preserve a replacement path: %v", err)
		}
		if info, err := os.Stat(service.Paths.Lock); err != nil || !info.IsDir() {
			t.Fatalf("cleanup removed a replacement path: %v", err)
		}
	}
	primary := domain.NewError("INVALID_STATE", "original error")
	var err error = primary
	releaseOperationLock(&err, func() error { return nil })
	if err != primary {
		t.Fatal("successful cleanup changed the original error")
	}
	cleanup := errors.New("cleanup failed")
	releaseOperationLock(&err, func() error { return cleanup })
	if !errors.Is(err, primary) || !errors.Is(err, cleanup) {
		t.Fatalf("cleanup discarded an error: %v", err)
	}
}
