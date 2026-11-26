package app

import (
	"context"
	"database/sql"

	"github.com/Ghr07h/norns/norns/internal/domain"
)

type TaskList struct {
	Tasks []domain.Task `json:"tasks"`
}

func (s *Service) ListTasks(ctx context.Context, status string) (Result, error) {
	if status != "" && status != "open" && status != "closed" {
		return Result{}, domain.NewError("INVALID_ARGUMENT", "status must be open or closed")
	}
	list := TaskList{Tasks: []domain.Task{}}
	err := s.shared(ctx, func(ctx context.Context) error {
		return s.Store.Read(ctx, func(tx *sql.Tx) error {
			query := `SELECT id,purpose,init_cognition_ref,max_concurrency,status,outcome,outcome_reason,created_at,closed_at FROM tasks`
			var args []any
			if status != "" {
				query += ` WHERE status=?`
				args = append(args, status)
			}
			query += ` ORDER BY substr(created_at,1,19) ASC,rtrim(substr(created_at,20),'Z') ASC,id ASC`
			rows, err := tx.QueryContext(ctx, query, args...)
			if err != nil {
				return dbErr(err)
			}
			defer rows.Close()
			for rows.Next() {
				var task domain.Task
				var outcome, reason, closed sql.NullString
				if err := rows.Scan(&task.ID, &task.Purpose, &task.InitCognitionRef, &task.MaxConcurrency, &task.Status, &outcome, &reason, &task.CreatedAt, &closed); err != nil {
					return dbErr(err)
				}
				task.Outcome, task.OutcomeReason, task.ClosedAt = ptr(outcome), ptr(reason), ptr(closed)
				list.Tasks = append(list.Tasks, task)
			}
			return dbErr(rows.Err())
		})
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Data: list, Message: "Tasks listed"}, nil
}
