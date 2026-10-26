package domain

import (
	"strings"
	"testing"
	"time"
)

func TestRandomEntityIDsAreUUIDv4WithExpectedPrefixes(t *testing.T) {
	generator := RandomIDs{}
	values := []struct {
		entity string
		prefix string
		make   func() (string, error)
	}{
		{"task", "task", generator.TaskID},
		{"cognition", "cog", generator.CognitionID},
		{"exploration", "exp", generator.ExplorationID},
	}
	seen := map[string]bool{}
	for _, value := range values {
		for range 32 {
			id, err := value.make()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(id, value.prefix+"_") {
				t.Fatalf("generated %s ID has wrong prefix: %q", value.entity, id)
			}
			if err := ValidateID(id, value.entity); err != nil {
				t.Fatalf("generated invalid %s ID %q: %v", value.entity, id, err)
			}
			if seen[id] {
				t.Fatalf("duplicate generated ID: %s", id)
			}
			seen[id] = true
		}
	}
}

func TestValidateIDRejectsWrongTypeAndNonV4UUID(t *testing.T) {
	for _, test := range []struct{ value, prefix string }{
		{"task_123e4567-e89b-42d3-a456-426614174000", "cognition"},
		{"cognition_123e4567-e89b-42d3-a456-426614174000", "cognition"},
		{"exploration_123e4567-e89b-42d3-a456-426614174000", "exploration"},
		{"task_123e4567-e89b-12d3-a456-426614174000", "task"},
		{"task_123E4567-E89B-42D3-A456-426614174000", "task"},
		{"task_bad", "task"},
	} {
		if err := ValidateID(test.value, test.prefix); err == nil {
			t.Fatalf("invalid ID accepted: %#v", test)
		}
	}
}

func TestTimeTextIsUTCAndRoundTrips(t *testing.T) {
	zone := time.FixedZone("test", 8*60*60)
	input := time.Date(2026, 10, 12, 16, 30, 0, 123456789, zone)
	text := TimeText(input)
	if text != "2026-10-12T08:30:00.123456789Z" {
		t.Fatalf("unexpected UTC time: %s", text)
	}
	parsed, err := ParseTime(text)
	if err != nil || !parsed.Equal(input) {
		t.Fatalf("time did not round-trip: %v %v", parsed, err)
	}
}

func TestNonEmptyTrimsAndRejectsBlankText(t *testing.T) {
	value, err := NonEmpty("  content  ", "value")
	if err != nil || value != "content" {
		t.Fatalf("unexpected normalized text: %q %v", value, err)
	}
	if _, err := NonEmpty(" \n\t", "value"); err == nil {
		t.Fatal("blank text was accepted")
	}
}
