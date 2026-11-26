package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Ghr07h/norns/norns/internal/domain"
	storesqlite "github.com/Ghr07h/norns/norns/internal/store/sqlite"
)

const cognitionColumns = `id,title,content,task_id,origin,origin_task_id,origin_exploration_id,task_purpose_relevant,created_at,meta`
const explorationColumns = `id,title,content,task_id,retry_of,status,explorer_id,output_ref,assigned_at,outcome,outcome_reason,created_at,finished_at,sources,hints`

func (s *Service) GetCognition(ctx context.Context, id string) (Result, error) {
	if err := domain.ValidateID(id, "cognition"); err != nil {
		return Result{}, err
	}
	var cognition domain.Cognition
	err := s.shared(ctx, func(ctx context.Context) error {
		return s.Store.Read(ctx, func(tx *sql.Tx) error {
			var err error
			cognition, err = scanCognition(tx.QueryRowContext(ctx, `SELECT `+cognitionColumns+` FROM cognitions WHERE id=?`, id))
			if errors.Is(err, sql.ErrNoRows) {
				return domain.NewError("COGNITION_NOT_FOUND", "Cognition not found")
			}
			return err
		})
	})
	return Result{Data: cognition, Message: "Cognition retrieved"}, err
}

func (s *Service) GetExploration(ctx context.Context, id string) (Result, error) {
	if err := domain.ValidateID(id, "exploration"); err != nil {
		return Result{}, err
	}
	var exploration domain.Exploration
	err := s.shared(ctx, func(ctx context.Context) error {
		return s.Store.Read(ctx, func(tx *sql.Tx) error {
			var err error
			exploration, err = scanExploration(tx.QueryRowContext(ctx, `SELECT `+explorationColumns+` FROM explorations WHERE id=?`, id))
			if errors.Is(err, sql.ErrNoRows) {
				return domain.NewError("EXPLORATION_NOT_FOUND", "Exploration not found")
			}
			return err
		})
	})
	return Result{Data: exploration, Message: "Exploration retrieved"}, err
}

type entityScanner interface {
	Scan(...any) error
}

func scanCognition(row entityScanner) (domain.Cognition, error) {
	var c domain.Cognition
	var metaJSON string
	var task, originTask, originExp sql.NullString
	if err := row.Scan(&c.ID, &c.Title, &c.Content, &task, &c.Origin, &originTask, &originExp, &c.TaskPurposeRelevant, &c.CreatedAt, &metaJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c, err
		}
		return c, dbErr(err)
	}
	var err error
	c.Meta, err = domain.DecodeCognitionMeta(metaJSON)
	if err != nil {
		return c, domain.Wrap("WORKSPACE_STATE_UNTRUSTED", "Cognition meta is invalid", err)
	}
	c.TaskID = ptr(task)
	c.OriginTaskID = ptr(originTask)
	c.OriginExplorationID = ptr(originExp)
	c.OriginRef = c.OriginTaskID
	if c.OriginRef == nil {
		c.OriginRef = c.OriginExplorationID
	}
	return c, nil
}

func scanExploration(row entityScanner) (domain.Exploration, error) {
	var e domain.Exploration
	var sourcesJSON, hintsJSON string
	var retry, explorer, outputRef, assigned, outcome, reason, finished sql.NullString
	if err := row.Scan(&e.ID, &e.Title, &e.Content, &e.TaskID, &retry, &e.Status, &explorer, &outputRef, &assigned, &outcome, &reason, &e.CreatedAt, &finished, &sourcesJSON, &hintsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return e, err
		}
		return e, dbErr(err)
	}
	e.RetryOf = ptr(retry)
	e.ExplorerID = ptr(explorer)
	e.OutputRef = ptr(outputRef)
	e.AssignedAt = ptr(assigned)
	e.Outcome = ptr(outcome)
	e.OutcomeReason = ptr(reason)
	e.FinishedAt = ptr(finished)
	var err error
	e.Sources, err = storesqlite.DecodeCognitionIDs(sourcesJSON)
	if err != nil {
		return e, err
	}
	e.Hints, err = storesqlite.DecodeCognitionIDs(hintsJSON)
	return e, err
}
