package sqlite

import (
	"context"
	"encoding/json"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"net/http"
)

func saveOverride(ctx context.Context, q dbtx, runtimeID string, values []json.RawMessage) error {
	raw := json.RawMessage(`{}`)
	if len(values) > 0 && len(values[0]) > 0 && string(values[0]) != "null" {
		raw = values[0]
	}
	_, err := q.ExecContext(ctx, `UPDATE runtimes SET config_override_json=? WHERE id=?`, string(raw), runtimeID)
	return err
}
func (s *Store) SaveRuntimeConfig(ctx context.Context, runtimeID string, profile domain.RuntimeProfile, effective any) error {
	return s.withWrite(ctx, func(q dbtx) error {
		result, err := q.ExecContext(ctx, `UPDATE runtimes SET profile_json=?,effective_config_json=? WHERE id=? AND profile_json IS NULL`, profileJSON(profile), payloadJSON(effective), runtimeID)
		if err != nil {
			return err
		}
		return requireAffected(result, "INVALID_STATE", "Runtime configuration is already frozen", http.StatusConflict)
	})
}
