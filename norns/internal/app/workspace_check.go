package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Ghr07h/norns/norns/internal/domain"
)

type WorkspaceSummary struct {
	TaskCount        int64 `json:"task_count"`
	CognitionCount   int64 `json:"cognition_count"`
	ExplorationCount int64 `json:"exploration_count"`
}

type WorkspaceCheck struct {
	Valid           bool             `json:"valid"`
	State           string           `json:"state"`
	RuntimeOccupied bool             `json:"runtime_occupied"`
	Task            *domain.Task     `json:"task,omitempty"`
	Tasks           *[]domain.Task   `json:"tasks,omitempty"`
	Summary         WorkspaceSummary `json:"summary"`
}

func (s *Service) CheckWorkspace(ctx context.Context) (Result, error) {
	check := WorkspaceCheck{Valid: true, State: "empty"}
	err := s.shared(ctx, func(ctx context.Context) error {
		return s.Store.Read(ctx, func(tx *sql.Tx) error {
			if err := tx.QueryRowContext(ctx, `SELECT runtime_occupied, (SELECT count(*) FROM tasks), (SELECT count(*) FROM cognitions), (SELECT count(*) FROM explorations) FROM workspace_meta WHERE singleton=1`).Scan(
				&check.RuntimeOccupied, &check.Summary.TaskCount, &check.Summary.CognitionCount, &check.Summary.ExplorationCount,
			); err != nil {
				return dbErr(err)
			}
			task, err := openTask(ctx, tx)
			if err == nil {
				check.State = "open_task"
				check.Task = &task
				return nil
			}
			var taskError *domain.Error
			if !errors.As(err, &taskError) || taskError.Code != "NO_OPEN_TASK" {
				return err
			}
			tasks := []domain.Task{}
			check.Tasks = &tasks
			if check.Summary.TaskCount == 0 {
				return nil
			}
			check.State = "no_open_task"
			rows, err := tx.QueryContext(ctx, `SELECT id,purpose,init_cognition_ref,max_concurrency,status,outcome,outcome_reason,created_at,closed_at FROM tasks ORDER BY substr(created_at,1,19) ASC,rtrim(substr(created_at,20),'Z') ASC,id ASC`)
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
				tasks = append(tasks, task)
			}
			if err := rows.Err(); err != nil {
				return dbErr(err)
			}
			return nil
		})
	})
	if err != nil {
		return Result{}, err
	}
	message := "Workspace checked"
	if check.State == "empty" {
		message = "Workspace is empty"
	}
	return Result{Data: check, Message: message}, nil
}
