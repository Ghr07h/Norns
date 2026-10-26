package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Error struct {
	Code    string
	Message string
	Details map[string]any
	Cause   error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }

func NewError(code, message string) *Error {
	return &Error{Code: code, Message: message, Details: map[string]any{}}
}

func Errorf(code, format string, args ...any) *Error {
	return NewError(code, fmt.Sprintf(format, args...))
}

func Wrap(code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Details: map[string]any{}, Cause: cause}
}

type Clock interface{ Now() time.Time }
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

func TimeText(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func ParseTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }

type IDGenerator interface {
	TaskID() (string, error)
	CognitionID() (string, error)
	ExplorationID() (string, error)
	Token() (string, error)
}

type RandomIDs struct{}

func (RandomIDs) TaskID() (string, error)        { return entityID("task") }
func (RandomIDs) CognitionID() (string, error)   { return entityID("cog") }
func (RandomIDs) ExplorationID() (string, error) { return entityID("exp") }
func (RandomIDs) Token() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func entityID(prefix string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s_%08x-%04x-%04x-%04x-%012x", prefix,
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

var idPattern = regexp.MustCompile(`^(task|cog|exp)_([0-9a-f]{8})-([0-9a-f]{4})-4([0-9a-f]{3})-([89ab][0-9a-f]{3})-([0-9a-f]{12})$`)

var entityIDPrefixes = map[string]string{
	"task":        "task",
	"cognition":   "cog",
	"exploration": "exp",
}

func ValidateID(value, entity string) error {
	m := idPattern.FindStringSubmatch(value)
	if m == nil || m[1] != entityIDPrefixes[entity] {
		return Errorf("INVALID_ARGUMENT", "invalid %s ID: %s", entity, value)
	}
	return nil
}

func NonEmpty(value, name string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", Errorf("INVALID_ARGUMENT", "%s must not be empty", name)
	}
	return value, nil
}

var TaskOutcomes = map[string]bool{"succeeded": true, "unachievable": true, "failed": true, "cancelled": true}
var ExplorationOutcomes = map[string]bool{"succeeded": true, "inconclusive": true, "failed": true, "cancelled": true}

type Task struct {
	ID               string  `json:"id" yaml:"id"`
	Purpose          string  `json:"purpose" yaml:"purpose"`
	InitCognitionRef string  `json:"init_cognition_ref" yaml:"init_cognition_ref"`
	MaxConcurrency   int     `json:"max_concurrency" yaml:"max_concurrency"`
	Status           string  `json:"status" yaml:"status"`
	Outcome          *string `json:"outcome,omitempty" yaml:"outcome,omitempty"`
	OutcomeReason    *string `json:"outcome_reason,omitempty" yaml:"outcome_reason,omitempty"`
	CreatedAt        string  `json:"created_at" yaml:"created_at"`
	ClosedAt         *string `json:"closed_at,omitempty" yaml:"closed_at,omitempty"`
}

type Cognition struct {
	ID                  string  `json:"id" yaml:"id"`
	Title               string  `json:"title" yaml:"title"`
	Content             string  `json:"content" yaml:"content"`
	TaskID              *string `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	Origin              string  `json:"origin" yaml:"origin"`
	OriginRef           *string `json:"origin_ref,omitempty" yaml:"origin_ref,omitempty"`
	OriginCognitionID   *string `json:"-" yaml:"-"`
	OriginExplorationID *string `json:"-" yaml:"-"`
	TaskPurposeRelevant bool    `json:"task_purpose_relevant" yaml:"task_purpose_relevant"`
	CreatedAt           string  `json:"created_at" yaml:"created_at"`
}

type Exploration struct {
	ID            string   `json:"id" yaml:"id"`
	Title         string   `json:"title" yaml:"title"`
	Content       string   `json:"content" yaml:"content"`
	TaskID        string   `json:"task_id" yaml:"task_id"`
	Sources       []string `json:"source_cognitions" yaml:"source_cognitions"`
	RetryOf       *string  `json:"retry_of,omitempty" yaml:"retry_of,omitempty"`
	Status        string   `json:"status" yaml:"status"`
	ExecutorID    *string  `json:"executor_id,omitempty" yaml:"executor_id,omitempty"`
	OutputRef     *string  `json:"output_ref,omitempty" yaml:"output_ref,omitempty"`
	AssignedAt    *string  `json:"assigned_at,omitempty" yaml:"assigned_at,omitempty"`
	Outcome       *string  `json:"outcome,omitempty" yaml:"outcome,omitempty"`
	OutcomeReason *string  `json:"outcome_reason,omitempty" yaml:"outcome_reason,omitempty"`
	CreatedAt     string   `json:"created_at" yaml:"created_at"`
	FinishedAt    *string  `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
}
