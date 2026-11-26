package sqlite

import "context"

func (s *Store) AllSubjectsIdle(ctx context.Context) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subjects WHERE state <> 'idle' OR current_runtime_id IS NOT NULL OR workspace_state IN ('planned','materializing')`).Scan(&count)
	return count == 0, err
}
