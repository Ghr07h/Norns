package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Ghr07h/norns/norns/internal/domain"
	storesqlite "github.com/Ghr07h/norns/norns/internal/store/sqlite"
)

type resetJournal struct {
	Version     int    `json:"version"`
	Token       string `json:"token"`
	Phase       string `json:"phase"`
	OldRoot     string `json:"old_root"`
	NewRoot     string `json:"new_root"`
	ArchiveName string `json:"archive_name"`
	CreatedAt   string `json:"created_at"`
}

var (
	resetTokenPattern   = regexp.MustCompile(`^[0-9a-f]{16}$`)
	resetArchivePattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[0-9a-f]{8}$`)
)

func (s *Service) Reset(ctx context.Context) (Result, error) {
	var result Result
	err := s.exclusive(ctx, func(ctx context.Context) error {
		if !s.Store.Initialized() {
			return domain.NewError("WORKSPACE_NOT_INITIALIZED", "Workspace is not initialized")
		}
		if err := s.Store.Read(ctx, func(tx *sql.Tx) error {
			var count int
			if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM tasks WHERE status='open'`).Scan(&count); e != nil {
				return dbErr(e)
			}
			if count != 0 {
				return domain.NewError("RESET_OPEN_TASK", "close the open Task with outcome=cancelled before reset")
			}
			return nil
		}); err != nil {
			return err
		}
		db, err := s.Store.OpenWrite(ctx)
		if err != nil {
			return err
		}
		if _, err = db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
			db.Close()
			return domain.Wrap("DATABASE_ERROR", "cannot checkpoint Workspace before reset", err)
		}
		if err = db.Close(); err != nil {
			return domain.Wrap("DATABASE_ERROR", "cannot close Workspace before reset", err)
		}
		token, err := s.IDs.Token()
		if err != nil {
			return domain.Wrap("INTERNAL_ERROR", "cannot generate reset token", err)
		}
		j := resetJournal{Version: 1, Token: token, Phase: "prepared", OldRoot: ".norns.old." + token, NewRoot: ".norns.new." + token, ArchiveName: s.Clock.Now().UTC().Format("20060102T150405.000000000Z") + "-" + token[:8], CreatedAt: domain.TimeText(s.Clock.Now())}
		newRoot := filepath.Join(s.Paths.Root, j.NewRoot)
		newPaths := s.Paths
		newPaths.Norns = newRoot
		newPaths.Database = filepath.Join(newRoot, "norns.sqlite")
		newPaths.View = filepath.Join(newRoot, "view")
		newPaths.History = filepath.Join(newRoot, "history")
		newStore := storesqlite.New(newPaths, s.IDs, s.Clock)
		if err := newStore.Initialize(ctx, nil); err != nil {
			return err
		}
		if err := os.MkdirAll(newPaths.History, 0o700); err != nil {
			return domain.Wrap("RESET_FAILED", "cannot prepare new history directory", err)
		}
		if err := s.writeResetJournal(j); err != nil {
			return err
		}
		oldRoot := filepath.Join(s.Paths.Root, j.OldRoot)
		if err := s.Files.Rename(s.Paths.Norns, oldRoot); err != nil {
			return domain.Wrap("RESET_FAILED", "cannot move old Workspace root", err)
		}
		j.Phase = "old_renamed"
		if err := s.writeResetJournal(j); err != nil {
			return err
		}
		if err := s.Files.Rename(newRoot, s.Paths.Norns); err != nil {
			return domain.Wrap("RESET_FAILED", "cannot publish new Workspace root", err)
		}
		j.Phase = "new_published"
		if err := s.writeResetJournal(j); err != nil {
			return err
		}
		archive, err := s.finishReset(j)
		if err != nil {
			return err
		}
		j.Phase = "archived"
		if err := s.writeResetJournal(j); err != nil {
			return err
		}
		if err := os.Remove(s.Paths.ResetLog); err != nil {
			return domain.Wrap("RESET_FAILED", "cannot remove reset journal", err)
		}
		_ = s.Files.SyncDir(s.Paths.Root)
		result = Result{Data: map[string]any{"archive_path": filepath.ToSlash(archive), "task_open": false}, Message: "Workspace reset"}
		return nil
	})
	return result, err
}

func (s *Service) writeResetJournal(j resetJournal) error {
	data, err := json.Marshal(j)
	if err != nil {
		return domain.Wrap("INTERNAL_ERROR", "cannot encode reset journal", err)
	}
	temp := s.Paths.ResetLog + ".tmp"
	if err := os.WriteFile(temp, append(data, '\n'), 0o600); err != nil {
		return domain.Wrap("RESET_FAILED", "cannot write reset journal", err)
	}
	defer os.Remove(temp)
	if err := s.Files.SyncFile(temp); err != nil {
		return domain.Wrap("RESET_FAILED", "cannot sync reset journal", err)
	}
	if err := s.Files.Rename(temp, s.Paths.ResetLog); err != nil {
		return domain.Wrap("RESET_FAILED", "cannot publish reset journal", err)
	}
	if err := s.Files.SyncDir(s.Paths.Root); err != nil {
		return domain.Wrap("RESET_FAILED", "cannot sync Workspace root", err)
	}
	return nil
}

func (s *Service) readResetJournal() (resetJournal, error) {
	var j resetJournal
	data, err := os.ReadFile(s.Paths.ResetLog)
	if err != nil {
		return j, domain.Wrap("RESET_RECOVERY_FAILED", "cannot read reset journal", err)
	}
	if err := json.Unmarshal(data, &j); err != nil {
		return j, domain.Wrap("RESET_RECOVERY_FAILED", "invalid reset journal", err)
	}
	createdAt, timeErr := domain.ParseTime(j.CreatedAt)
	if j.Version != 1 || !resetTokenPattern.MatchString(j.Token) ||
		j.Phase != "prepared" && j.Phase != "old_renamed" && j.Phase != "new_published" && j.Phase != "archived" ||
		j.OldRoot != (".norns.old."+j.Token) || j.NewRoot != (".norns.new."+j.Token) ||
		!resetArchivePattern.MatchString(j.ArchiveName) || timeErr != nil || domain.TimeText(createdAt) != j.CreatedAt {
		return j, domain.NewError("RESET_RECOVERY_FAILED", "reset journal contains invalid paths")
	}
	return j, nil
}

func (s *Service) recoverResetLocked(ctx context.Context) error {
	j, err := s.readResetJournal()
	if err != nil {
		return err
	}
	oldRoot := filepath.Join(s.Paths.Root, j.OldRoot)
	newRoot := filepath.Join(s.Paths.Root, j.NewRoot)
	archive := filepath.Join(s.Paths.History, j.ArchiveName)
	current := exists(s.Paths.Norns)
	old := exists(oldRoot)
	newExists := exists(newRoot)
	archived := exists(archive)
	if archived && current && !old {
		if err := s.validateResetRoot(ctx, s.Paths.Norns, true); err != nil {
			return err
		}
		if err := s.validateResetRoot(ctx, archive, false); err != nil {
			return err
		}
		if newExists {
			if err := s.validateResetRoot(ctx, newRoot, true); err != nil {
				return err
			}
			if err := s.Files.RemoveAll(newRoot); err != nil {
				return domain.Wrap("RESET_RECOVERY_FAILED", "cannot remove verified reset staging directory", err)
			}
		}
		if err := os.Remove(s.Paths.ResetLog); err != nil {
			return domain.Wrap("RESET_RECOVERY_FAILED", "cannot complete reset recovery", err)
		}
		return nil
	}
	if !current && old {
		if newExists {
			if err := s.validateResetRoot(ctx, oldRoot, false); err != nil {
				return err
			}
			if err := s.validateResetRoot(ctx, newRoot, true); err == nil {
				if err := s.Files.Rename(newRoot, s.Paths.Norns); err != nil {
					return domain.Wrap("RESET_RECOVERY_FAILED", "cannot publish staged Workspace", err)
				}
				current = true
			} else {
				if err := s.Files.Rename(oldRoot, s.Paths.Norns); err != nil {
					return domain.Wrap("RESET_RECOVERY_FAILED", "cannot restore old Workspace after invalid staging state", err)
				}
				if err := os.Remove(s.Paths.ResetLog); err != nil {
					return domain.Wrap("RESET_RECOVERY_FAILED", "cannot clear reset journal", err)
				}
				return nil
			}
		} else {
			if err := s.validateResetRoot(ctx, oldRoot, false); err != nil {
				return err
			}
			if err := s.Files.Rename(oldRoot, s.Paths.Norns); err != nil {
				return domain.Wrap("RESET_RECOVERY_FAILED", "cannot restore old Workspace", err)
			}
			if err := os.Remove(s.Paths.ResetLog); err != nil {
				return domain.Wrap("RESET_RECOVERY_FAILED", "cannot clear reset journal", err)
			}
			return nil
		}
	}
	if current && old {
		if err := s.validateResetRoot(ctx, s.Paths.Norns, true); err != nil {
			return err
		}
		if err := s.validateResetRoot(ctx, oldRoot, false); err != nil {
			return err
		}
		if _, err := s.finishReset(j); err != nil {
			return err
		}
		if err := os.Remove(s.Paths.ResetLog); err != nil {
			return domain.Wrap("RESET_RECOVERY_FAILED", "cannot clear reset journal", err)
		}
		return nil
	}
	if current && !old && newExists && j.Phase == "prepared" {
		if err := s.validateResetRoot(ctx, s.Paths.Norns, false); err != nil {
			return err
		}
		if err := s.validateResetRoot(ctx, newRoot, true); err != nil {
			return err
		}
		if err := s.Files.RemoveAll(newRoot); err != nil {
			return domain.Wrap("RESET_RECOVERY_FAILED", "cannot discard staged Workspace", err)
		}
		if err := os.Remove(s.Paths.ResetLog); err != nil {
			return domain.Wrap("RESET_RECOVERY_FAILED", "cannot clear reset journal", err)
		}
		return nil
	}
	return domain.NewError("RESET_RECOVERY_FAILED", "reset state cannot be recovered deterministically")
}

func (s *Service) validateResetRoot(ctx context.Context, nornsRoot string, requireEmpty bool) error {
	paths := s.Paths
	paths.Norns = nornsRoot
	paths.Database = filepath.Join(nornsRoot, "norns.sqlite")
	paths.View = filepath.Join(nornsRoot, "view")
	paths.History = filepath.Join(nornsRoot, "history")
	store := storesqlite.New(paths, s.IDs, s.Clock)
	if !store.Initialized() {
		return domain.NewError("RESET_RECOVERY_FAILED", "reset Workspace root has no database")
	}
	err := store.Read(ctx, func(tx *sql.Tx) error {
		if !requireEmpty {
			return nil
		}
		var tasks, cognitions, explorations, runtime int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM tasks`).Scan(&tasks); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM cognitions`).Scan(&cognitions); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM explorations`).Scan(&explorations); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT runtime_occupied FROM workspace_meta WHERE singleton=1`).Scan(&runtime); err != nil {
			return err
		}
		if tasks != 0 || cognitions != 0 || explorations != 0 || runtime != 0 {
			return domain.NewError("RESET_RECOVERY_FAILED", "published reset Workspace is not empty")
		}
		return nil
	})
	if err == nil {
		return nil
	}
	var typed *domain.Error
	if errors.As(err, &typed) && typed.Code == "RESET_RECOVERY_FAILED" {
		return err
	}
	return domain.Wrap("RESET_RECOVERY_FAILED", "reset Workspace root failed validation", err)
}

func (s *Service) finishReset(j resetJournal) (string, error) {
	oldRoot := filepath.Join(s.Paths.Root, j.OldRoot)
	oldHistory := filepath.Join(oldRoot, "history")
	if err := os.MkdirAll(s.Paths.History, 0o700); err != nil {
		return "", domain.Wrap("RESET_FAILED", "cannot create new history directory", err)
	}
	entries, err := os.ReadDir(oldHistory)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", domain.Wrap("RESET_FAILED", "cannot read old history", err)
	}
	for _, entry := range entries {
		from := filepath.Join(oldHistory, entry.Name())
		to := filepath.Join(s.Paths.History, entry.Name())
		if exists(to) {
			return "", domain.NewError("RESET_RECOVERY_FAILED", "history entry collision during reset")
		}
		if err := s.Files.Rename(from, to); err != nil {
			return "", domain.Wrap("RESET_FAILED", "cannot transfer history entry", err)
		}
	}
	_ = os.Remove(oldHistory)
	archive := filepath.Join(s.Paths.History, j.ArchiveName)
	if exists(oldRoot) && !exists(archive) {
		if err := s.Files.Rename(oldRoot, archive); err != nil {
			return "", domain.Wrap("RESET_FAILED", "cannot archive old Workspace", err)
		}
	}
	if !exists(archive) {
		return "", domain.NewError("RESET_RECOVERY_FAILED", "reset archive is missing")
	}
	if err := s.Files.SyncDir(s.Paths.History); err != nil {
		return "", domain.Wrap("RESET_FAILED", "cannot sync history directory", err)
	}
	return filepath.Rel(s.Paths.Root, archive)
}
func exists(path string) bool { _, err := os.Stat(path); return err == nil }
