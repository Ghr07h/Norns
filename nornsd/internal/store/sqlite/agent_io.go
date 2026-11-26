package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
)

// AcceptInput records Claude's replay, not merely a successful OS pipe write.
// Unknown UUIDs (for example historical session messages) never consume input.
func (s *Store) AcceptInput(ctx context.Context, subjectID, runtimeID, id string, now time.Time) error {
	return s.withWrite(ctx, func(q dbtx) error {
		rows, err := q.QueryContext(ctx, `SELECT sequence FROM runtime_instructions WHERE runtime_id=? AND input_uuid=? AND write_confirmed_at IS NOT NULL AND accepted_at IS NULL ORDER BY sequence`, runtimeID, id)
		if err != nil {
			return err
		}
		var sequences []uint64
		for rows.Next() {
			var seq uint64
			if err = rows.Scan(&seq); err != nil {
				_ = rows.Close()
				return err
			}
			sequences = append(sequences, seq)
		}
		if err = rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err = rows.Close(); err != nil {
			return err
		}
		if len(sequences) == 0 {
			return nil
		}
		if _, err = q.ExecContext(ctx, `UPDATE runtime_instructions SET accepted_at=? WHERE runtime_id=? AND input_uuid=? AND accepted_at IS NULL`, formatTime(now), runtimeID, id); err != nil {
			return err
		}
		_, err = appendEvent(ctx, q, subjectID, &runtimeID, "instruction.accepted", "agent", map[string]any{"input_uuid": id, "instruction_sequences": sequences}, now, nil, nil)
		return err
	})
}

// MarkAgentProcessing tracks root work, including handling a background task
// notification. Merely receiving subagent progress does not make the root busy.
func (s *Store) MarkAgentProcessing(ctx context.Context, subjectID, runtimeID string, now time.Time) error {
	return s.withWrite(ctx, func(q dbtx) error {
		_, err := q.ExecContext(ctx, `UPDATE runtimes SET state='running',state_changed_at=? WHERE subject_id=? AND id=? AND state='waiting'`, formatTime(now), subjectID, runtimeID)
		return err
	})
}

type runtimeUsage struct {
	ResultCount      int            `json:"result_count"`
	ErrorResultCount int            `json:"error_result_count"`
	Totals           map[string]any `json:"totals"`
	Latest           any            `json:"latest"`
	LatestResult     map[string]any `json:"latest_result"`
	ModelUsage       any            `json:"model_usage,omitempty"`
	TotalCostUSD     any            `json:"total_cost_usd,omitempty"`
}

// CompleteResult settles ALL inputs echoed since the preceding result. A
// notification result can consume user inputs; a pure notification consumes none.
// Inputs written but not echoed remain outstanding and are never auto-requeued.
// UUID deduplication also prevents repeated frames from charging usage twice.
func (s *Store) CompleteResult(ctx context.Context, subjectID, runtimeID string, raw json.RawMessage, rawSeq uint64, now time.Time) (domain.Runtime, error) {
	var frame map[string]any
	if err := json.Unmarshal(raw, &frame); err != nil {
		return domain.Runtime{}, err
	}
	id, _ := frame["uuid"].(string)
	if id == "" {
		id = fmt.Sprintf("raw:%d", rawSeq)
	}
	var out domain.Runtime
	err := s.withWrite(ctx, func(q dbtx) error {
		r, err := getRuntime(ctx, q, subjectID, runtimeID, false)
		if err != nil {
			return err
		}
		insert, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO runtime_results(runtime_id,result_key,raw_seq,payload_json,created_at) VALUES (?,?,?,?,?)`, runtimeID, id, rawSeq, string(raw), formatTime(now))
		if err != nil {
			return err
		}
		if n, _ := insert.RowsAffected(); n == 0 {
			out = r
			return nil
		}
		rows, err := q.QueryContext(ctx, `SELECT sequence FROM runtime_instructions WHERE runtime_id=? AND accepted_at IS NOT NULL AND settled_by IS NULL ORDER BY sequence`, runtimeID)
		if err != nil {
			return err
		}
		var sequences []uint64
		for rows.Next() {
			var seq uint64
			if err = rows.Scan(&seq); err != nil {
				_ = rows.Close()
				return err
			}
			sequences = append(sequences, seq)
		}
		if err = rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err = rows.Close(); err != nil {
			return err
		}
		if _, err = q.ExecContext(ctx, `UPDATE runtime_instructions SET settled_by=? WHERE runtime_id=? AND accepted_at IS NOT NULL AND settled_by IS NULL`, id, runtimeID); err != nil {
			return err
		}
		var outstanding, pending int
		if err = q.QueryRowContext(ctx, `SELECT COUNT(DISTINCT input_uuid) FROM runtime_instructions WHERE runtime_id=? AND write_confirmed_at IS NOT NULL AND settled_by IS NULL`, runtimeID).Scan(&outstanding); err != nil {
			return err
		}
		if err = q.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_instructions WHERE runtime_id=? AND dispatch_committed_at IS NULL`, runtimeID).Scan(&pending); err != nil {
			return err
		}
		state := r.State
		if !state.Terminal() {
			state = domain.RuntimeRunning
			if outstanding == 0 && pending == 0 {
				state = domain.RuntimeWaiting
			}
		}
		usage := runtimeUsage{Totals: map[string]any{}}
		if len(r.UsageJSON) > 0 {
			if err = json.Unmarshal(r.UsageJSON, &usage); err != nil {
				return err
			}
		}
		if usage.Totals == nil {
			usage.Totals = map[string]any{}
		}
		usage.ResultCount++
		if isError, _ := frame["is_error"].(bool); isError {
			usage.ErrorResultCount++
		}
		usage.Latest = frame["usage"]
		if values, ok := frame["usage"].(map[string]any); ok {
			addUsage(usage.Totals, values)
		}
		usage.LatestResult = map[string]any{"id": id}
		for _, key := range []string{"subtype", "is_error", "terminal_reason", "api_error_status", "errors", "origin"} {
			if value, ok := frame[key]; ok {
				usage.LatestResult[key] = value
			}
		}
		// These are process-cumulative snapshots, including work that is not
		// necessarily reported in a root result. Never sum these snapshots.
		if value, ok := frame["modelUsage"]; ok {
			usage.ModelUsage = value
		}
		if value, ok := frame["total_cost_usd"]; ok {
			usage.TotalCostUSD = value
		}
		changed := r.StateChangedAt
		if state != r.State {
			changed = now
		}
		if _, err = q.ExecContext(ctx, `UPDATE runtimes SET state=?,inflight_inputs=?,state_changed_at=?,usage_json=? WHERE id=?`, state, outstanding, formatTime(changed), payloadJSON(usage), runtimeID); err != nil {
			return err
		}
		if len(sequences) > 0 {
			if _, err = appendEvent(ctx, q, subjectID, &runtimeID, "instruction.settled", "agent", map[string]any{"result_id": id, "instruction_sequences": sequences, "is_error": frame["is_error"], "origin": frame["origin"]}, now, nil, nil); err != nil {
				return err
			}
		}
		out, err = getRuntime(ctx, q, subjectID, runtimeID, true)
		return err
	})
	return out, err
}

// Only numeric counters are additive; metadata such as service_tier, speed,
// inference_geo and arrays of iterations are not token counters.
func addUsage(total, current map[string]any) {
	for key, value := range current {
		switch v := value.(type) {
		case float64:
			previous, _ := total[key].(float64)
			total[key] = previous + v
		case map[string]any:
			nested, _ := total[key].(map[string]any)
			if nested == nil {
				nested = map[string]any{}
			}
			addUsage(nested, v)
			total[key] = nested
		}
	}
}
