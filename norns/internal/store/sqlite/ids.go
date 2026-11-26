package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/Ghr07h/norns/norns/internal/domain"
)

// The caller must hold the same write transaction used to insert the entity.
// Rolling it back also rolls back the allocation.
func NextCognitionID(ctx context.Context, q Queryer) (string, error) {
	return nextEntityID(ctx, q, "last_cognition_number", "cog")
}

func NextExplorationID(ctx context.Context, q Queryer) (string, error) {
	return nextEntityID(ctx, q, "last_exploration_number", "exp")
}

func nextEntityID(ctx context.Context, q Queryer, column, prefix string) (string, error) {
	var number int64
	// column is selected only by the two helpers above, never by user input.
	err := q.QueryRowContext(ctx, `UPDATE workspace_meta SET `+column+`=`+column+`+1 WHERE singleton=1 AND `+column+`<? RETURNING `+column, int64(math.MaxInt64)).Scan(&number)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.NewError("INVALID_STATE", "entity ID sequence is exhausted")
	}
	if err != nil {
		return "", mapDBError(err)
	}
	return fmt.Sprintf("%s_%d", prefix, number), nil
}
