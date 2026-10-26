package app

import "testing"

func TestArchivedSessionIDSupportsLegacyTranscriptMetadata(t *testing.T) {
	if got := archivedSessionID([]string{"subject-session.jsonl", "subject-session/subagent.jsonl"}, nil); got != "subject-session" {
		t.Fatalf("archivedSessionID() = %q, want legacy Subject session", got)
	}
	if got := archivedSessionID(nil, []string{"runtime-session.jsonl", "runtime-session"}); got != "runtime-session" {
		t.Fatalf("archivedSessionID() = %q, want missing Runtime session", got)
	}
}
