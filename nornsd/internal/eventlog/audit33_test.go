package eventlog

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"
)

// Sanitized frames captured from the real ECS Claude Code 2.1.199 audit.
func TestAuditedClaudeFramesProjectAndReplay(t *testing.T) {
	data, err := os.ReadFile("testdata/cc2199-frames.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string
		Frame  json.RawMessage
		Events []string
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			store, sid, rid := projectionStore(t)
			path := t.TempDir()
			logger := New(store, path, sid, rid)
			result, err := logger.Record(ctx, "stdout", "agent_message", tc.Frame, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if result.RawSeq == 0 {
				t.Fatal("missing durable frame identity")
			}
			events, err := store.ListEvents(ctx, sid, 100, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			types := []string{}
			for _, e := range events {
				types = append(types, e.Type)
			}
			if !reflect.DeepEqual(types, tc.Events) {
				t.Fatalf("types=%v want=%v", types, tc.Events)
			}
			var payload map[string]any
			if err = json.Unmarshal(events[0].Payload, &payload); err != nil {
				t.Fatal(err)
			}
			switch tc.Name {
			case "startup-error":
				if payload["errors"] == nil || !result.IsError {
					t.Fatalf("lost startup error: %s", events[0].Payload)
				}
			case "api-error-success-subtype":
				if payload["subtype"] != "success" || payload["api_error_status"] != float64(429) || !result.IsError {
					t.Fatalf("lost API error: %s", events[0].Payload)
				}
			case "synthetic-rate-limit":
				if payload["error"] != "rate_limit" || payload["model"] != "<synthetic>" {
					t.Fatalf("lost synthetic diagnostic: %s", events[0].Payload)
				}
			case "retry":
				if payload["error_status"] != float64(429) || payload["max_retries"] != float64(10) {
					t.Fatalf("lost retry diagnostic: %s", events[0].Payload)
				}
			}
			if err = ReplayRaw(ctx, store, path, sid); err != nil {
				t.Fatal(err)
			}
			replayed, err := store.ListEvents(ctx, sid, 100, nil, nil)
			if err != nil || !reflect.DeepEqual(events, replayed) {
				t.Fatalf("raw replay changed public events: %v", err)
			}
		})
	}
}
