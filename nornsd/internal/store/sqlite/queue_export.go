package sqlite

import (
	"context"
	"encoding/json"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"time"
)

type QueuedRecord struct {
	SubjectID     string                 `json:"subject_id"`
	SubjectName   *string                `json:"subject_name"`
	DirectoryName string                 `json:"directory_name"`
	RuntimeID     string                 `json:"runtime_id"`
	QueueSequence *int64                 `json:"queue_sequence"`
	QueuedAt      *time.Time             `json:"queued_at"`
	Profile       *domain.RuntimeProfile `json:"profile"`
	Config        json.RawMessage        `json:"config"`
	SessionMode   string                 `json:"session_mode"`
	Process       json.RawMessage        `json:"process"`
	Instructions  []domain.Instruction   `json:"instructions"`
}

// ExportQueued holds the write transaction across publication. A failed or
// interrupted publication never deletes queue or instruction records.
func (s *Store) ExportQueued(ctx context.Context, reason string, publish func([]QueuedRecord) error) error {
	return s.withWrite(ctx, func(q dbtx) error {
		rows, err := q.QueryContext(ctx, `SELECT subject_id,runtime_id FROM runtime_queue ORDER BY sequence`)
		if err != nil {
			return err
		}
		type pair struct{ subject, runtime string }
		ids := []pair{}
		for rows.Next() {
			var id pair
			if err = rows.Scan(&id.subject, &id.runtime); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if len(ids) == 0 {
			return nil
		}
		records := make([]QueuedRecord, 0, len(ids))
		for _, id := range ids {
			subject, err := getSubject(ctx, q, id.subject)
			if err != nil {
				return err
			}
			runtime, err := getRuntime(ctx, q, id.subject, id.runtime, true)
			if err != nil {
				return err
			}
			var profile *domain.RuntimeProfile
			if runtime.Profile.Agent != "" {
				p := runtime.Profile
				profile = &p
			}
			records = append(records, QueuedRecord{subject.ID, subject.Name, subject.DirectoryName, runtime.ID, runtime.QueueSequence, runtime.QueuedAt, profile, runtime.ConfigOverride, runtime.SessionMode, runtime.ProcessJSON, runtime.Instructions})
		}
		if err = publish(records); err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, id := range ids {
			if _, err = q.ExecContext(ctx, `DELETE FROM runtime_queue WHERE runtime_id=?`, id.runtime); err != nil {
				return err
			}
			if _, err = q.ExecContext(ctx, `UPDATE runtimes SET state='cancelled',ended_at=?,state_changed_at=?,end_reason=? WHERE id=? AND state='queued'`, formatTime(now), formatTime(now), reason, id.runtime); err != nil {
				return err
			}
			if _, err = q.ExecContext(ctx, `UPDATE subjects SET state='idle',current_runtime_id=NULL,updated_at=? WHERE id=?`, formatTime(now), id.subject); err != nil {
				return err
			}
			if _, err = appendEvent(ctx, q, id.subject, &id.runtime, "runtime.cancelled", "server", map[string]any{"reason": reason, "queued": true, "exported": true}, now, nil, nil); err != nil {
				return err
			}
		}
		return nil
	})
}
