package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Ghr07h/norns/norns/internal/domain"
	"github.com/Ghr07h/norns/norns/internal/i18n"
	storesqlite "github.com/Ghr07h/norns/norns/internal/store/sqlite"
)

type WorkspaceFree struct {
	Changed          bool    `json:"changed"`
	RuntimeOccupied  bool    `json:"runtime_occupied"`
	ClosedTaskID     *string `json:"closed_task_id,omitempty"`
	CancelledTodo    int64   `json:"cancelled_todo"`
	CancelledRunning int64   `json:"cancelled_running"`
}

func (s *Service) FreeWorkspace(ctx context.Context) (Result, error) {
	var freed WorkspaceFree
	err := s.exclusive(ctx, func(ctx context.Context) error {
		return s.Store.WriteQ(ctx, func(q storesqlite.Queryer) error {
			var taskID string
			if err := q.QueryRowContext(ctx, `SELECT id FROM tasks WHERE status='open'`).Scan(&taskID); errors.Is(err, sql.ErrNoRows) {
				return nil
			} else if err != nil {
				return dbErr(err)
			}
			now := domain.TimeText(s.Clock.Now())
			reason := i18n.Message("cancelled by norns workspace free")
			for _, status := range []string{"todo", "running"} {
				result, err := q.ExecContext(ctx, `UPDATE explorations SET status='finished',outcome='cancelled',outcome_reason=?,finished_at=? WHERE task_id=? AND status=?`, reason, now, taskID, status)
				if err != nil {
					return dbErr(err)
				}
				count, err := result.RowsAffected()
				if err != nil {
					return dbErr(err)
				}
				if status == "todo" {
					freed.CancelledTodo = count
				} else {
					freed.CancelledRunning = count
				}
			}
			if _, err := q.ExecContext(ctx, `UPDATE tasks SET status='closed',outcome='cancelled',outcome_reason=?,closed_at=? WHERE id=? AND status='open'`, reason, now, taskID); err != nil {
				return dbErr(err)
			}
			if _, err := q.ExecContext(ctx, `UPDATE workspace_meta SET runtime_occupied=0 WHERE singleton=1`); err != nil {
				return dbErr(err)
			}
			freed.Changed = true
			freed.ClosedTaskID = &taskID
			return nil
		})
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Data: freed, Message: "Workspace freed"}, nil
}
