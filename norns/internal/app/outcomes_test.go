package app

import (
	"context"
	"database/sql"
	"testing"
)

func TestEveryTaskOutcomePersistsAndCanReopen(t *testing.T) {
	for _, outcome := range []string{"succeeded", "unachievable", "failed", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			service, _ := newTestService(t)
			info := "Subject"
			opened, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Outcome", Concurrency: 1, Info: &info})
			if err != nil {
				t.Fatal(err)
			}
			taskID := resultID(t, opened)
			reason := "Task outcome: " + outcome
			if _, err := service.CloseTask(context.Background(), outcome, reason); err != nil {
				t.Fatal(err)
			}

			db, err := sql.Open("sqlite", service.Paths.Database)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var status, storedOutcome, storedReason string
			if err := db.QueryRow(`SELECT status,outcome,outcome_reason FROM tasks WHERE id=?`, taskID).Scan(&status, &storedOutcome, &storedReason); err != nil {
				t.Fatal(err)
			}
			if status != "closed" || storedOutcome != outcome || storedReason != reason {
				t.Fatalf("Task outcome was not persisted: status=%s outcome=%s reason=%s", status, storedOutcome, storedReason)
			}
			if _, err := service.ReopenTask(context.Background(), &taskID); err != nil {
				t.Fatalf("cannot reopen Task with outcome %s: %v", outcome, err)
			}
			var reopenedStatus string
			var reopenedOutcome, reopenedReason, closedAt sql.NullString
			if err := db.QueryRow(`SELECT status,outcome,outcome_reason,closed_at FROM tasks WHERE id=?`, taskID).Scan(&reopenedStatus, &reopenedOutcome, &reopenedReason, &closedAt); err != nil {
				t.Fatal(err)
			}
			if reopenedStatus != "open" || reopenedOutcome.Valid || reopenedReason.Valid || closedAt.Valid {
				t.Fatalf("reopen did not clear completion fields for outcome %s", outcome)
			}
		})
	}
}

func TestEveryExplorationOutcomePersistsAndCanRetry(t *testing.T) {
	for _, outcome := range []string{"succeeded", "inconclusive", "failed", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			service, explorationID := serviceWithExploration(t)
			if _, err := service.AssignExploration(context.Background(), explorationID, "executor", "output://result"); err != nil {
				t.Fatal(err)
			}
			finding, err := service.AddCognition(context.Background(), AddCognitionRequest{Title: "Finding", Content: "Preserved result", Origin: "exploration", ExplorationID: &explorationID})
			if err != nil {
				t.Fatal(err)
			}
			findingID := resultID(t, finding)
			reason := "Exploration outcome: " + outcome
			if _, err := service.FinishExploration(context.Background(), explorationID, outcome, reason); err != nil {
				t.Fatal(err)
			}

			db, err := sql.Open("sqlite", service.Paths.Database)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var status, storedOutcome, storedReason, finishedAt string
			if err := db.QueryRow(`SELECT status,outcome,outcome_reason,finished_at FROM explorations WHERE id=?`, explorationID).Scan(&status, &storedOutcome, &storedReason, &finishedAt); err != nil {
				t.Fatal(err)
			}
			if status != "finished" || storedOutcome != outcome || storedReason != reason || finishedAt == "" {
				t.Fatalf("Exploration outcome was not persisted: status=%s outcome=%s reason=%s finished=%s", status, storedOutcome, storedReason, finishedAt)
			}
			retry, err := service.RetryExploration(context.Background(), explorationID)
			if err != nil {
				t.Fatalf("cannot retry Exploration with outcome %s: %v", outcome, err)
			}
			retryID := resultID(t, retry)
			var retryOf, retryTitle, retryContent, retryStatus string
			var executor, outputRef, assignedAt, retryOutcome, retryReason, retryFinishedAt sql.NullString
			if err := db.QueryRow(`SELECT retry_of,title,content,status,executor_id,output_ref,assigned_at,outcome,outcome_reason,finished_at FROM explorations WHERE id=?`, retryID).Scan(&retryOf, &retryTitle, &retryContent, &retryStatus, &executor, &outputRef, &assignedAt, &retryOutcome, &retryReason, &retryFinishedAt); err != nil {
				t.Fatal(err)
			}
			if retryID == explorationID || retryOf != explorationID || retryTitle != "View publication" || retryContent != "View publication content" || retryStatus != "todo" || executor.Valid || outputRef.Valid || assignedAt.Valid || retryOutcome.Valid || retryReason.Valid || retryFinishedAt.Valid {
				t.Fatalf("invalid retry for outcome %s: id=%s retry_of=%s title=%s content=%s status=%s", outcome, retryID, retryOf, retryTitle, retryContent, retryStatus)
			}
			var copiedSource, preservedOrigin, preservedOutcome, preservedOutput string
			if err := db.QueryRow(`SELECT cognition_id FROM exploration_sources WHERE exploration_id=? ORDER BY ordinal`, retryID).Scan(&copiedSource); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT origin_exploration_id FROM cognitions WHERE id=?`, findingID).Scan(&preservedOrigin); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT outcome,output_ref FROM explorations WHERE id=?`, explorationID).Scan(&preservedOutcome, &preservedOutput); err != nil {
				t.Fatal(err)
			}
			if copiedSource == "" || preservedOrigin != explorationID || preservedOutcome != outcome || preservedOutput != "output://result" {
				t.Fatalf("retry changed source state for outcome %s: source=%s origin=%s outcome=%s output=%s", outcome, copiedSource, preservedOrigin, preservedOutcome, preservedOutput)
			}
		})
	}
}
