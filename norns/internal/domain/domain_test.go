package domain

import (
	"strings"
	"testing"
	"time"
)

func TestRandomTaskIDsAreUUIDv4(t *testing.T) {
	generator := RandomIDs{}
	values := []struct {
		entity string
		prefix string
		make   func() (string, error)
	}{
		{"task", "task", generator.TaskID},
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

func TestSequentialEntityIDValidationAndOrdering(t *testing.T) {
	for _, entity := range []string{"cognition", "exploration"} {
		prefix := "cog_"
		if entity == "exploration" {
			prefix = "exp_"
		}
		for _, suffix := range []string{"1", "9", "10", "9223372036854775807"} {
			if err := ValidateID(prefix+suffix, entity); err != nil {
				t.Fatal(err)
			}
		}
		for _, suffix := range []string{"0", "01", "0001", "-1", "+1", "1.0", "1e2", " 1", "1\n", "9223372036854775808", "550e8400-e29b-41d4-a716-446655440000"} {
			if err := ValidateID(prefix+suffix, entity); err == nil {
				t.Fatalf("invalid ID accepted: %s%s", prefix, suffix)
			}
		}
		if !IDLess(prefix+"2", prefix+"10") || !IDLess(prefix+"9223372036854775806", prefix+"9223372036854775807") || IDLess(prefix+"10", prefix+"2") {
			t.Fatal("numeric ID ordering failed")
		}
	}
	if ValidateID("cog_1", "exploration") == nil || ValidateID("exp_1", "cognition") == nil || ValidateID("task_1", "task") == nil {
		t.Fatal("wrong entity type accepted")
	}
}
