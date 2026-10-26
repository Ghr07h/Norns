package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type SubjectState string

const (
	SubjectIdle            SubjectState = "idle"
	SubjectQueuing         SubjectState = "queuing"
	SubjectUsing           SubjectState = "using"
	WorkspacePlanned                    = "planned"
	WorkspaceMaterializing              = "materializing"
	WorkspaceMaterialized               = "materialized"
	WorkspaceUnavailable                = "unavailable"
)

type RuntimeState string

const (
	RuntimeQueued    RuntimeState = "queued"
	RuntimeRunning   RuntimeState = "running"
	RuntimeWaiting   RuntimeState = "waiting"
	RuntimeEnded     RuntimeState = "ended"
	RuntimeError     RuntimeState = "error"
	RuntimeCancelled RuntimeState = "cancelled"
)

func (s RuntimeState) Terminal() bool {
	return s == RuntimeEnded || s == RuntimeError || s == RuntimeCancelled
}

type RuntimeProfile struct {
	Agent        string   `json:"agent"`
	AgentVersion string   `json:"agent_version"`
	Capabilities []string `json:"capabilities"`
}

func (p RuntimeProfile) Has(capability string) bool {
	for _, current := range p.Capabilities {
		if current == capability {
			return true
		}
	}
	return false
}

func (p RuntimeProfile) Equal(other RuntimeProfile) bool {
	if p.Agent != other.Agent || p.AgentVersion != other.AgentVersion || len(p.Capabilities) != len(other.Capabilities) {
		return false
	}
	for i := range p.Capabilities {
		if p.Capabilities[i] != other.Capabilities[i] {
			return false
		}
	}
	return true
}

type Subject struct {
	ID             string
	Name           *string
	State          SubjectState
	DirectoryName  string
	WorkspaceState string
	WorkspaceError json.RawMessage
	Profile        RuntimeProfile
	// SessionID and SessionInitialized are retained for v1 workspace and
	// database compatibility. New Claude sessions are identified by Runtime ID.
	SessionID           string
	SessionInitialized  bool
	CurrentRuntimeID    *string
	LastEventSeq        uint64
	LastRawSeq          uint64
	LastProjectedRawSeq uint64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Instruction struct {
	Sequence  uint64    `json:"sequence"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Runtime struct {
	ID             string
	SubjectID      string
	State          RuntimeState
	QueueSequence  *int64
	SettingsSlot   *int
	ConfigHash     *string
	SessionMode    string
	InflightTurns  int
	Closing        bool
	CloseDeadline  *time.Time
	QueuedAt       *time.Time
	StartedAt      *time.Time
	EndedAt        *time.Time
	StateChangedAt time.Time
	EndReason      *string
	ErrorCode      *string
	ErrorJSON      json.RawMessage
	ProcessJSON    json.RawMessage
	UsageJSON      json.RawMessage
	ResourceJSON   json.RawMessage
	TranscriptJSON json.RawMessage
	CreatedAt      time.Time
	Instructions   []Instruction
}

type Event struct {
	Seq       uint64          `json:"seq"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	RuntimeID *string         `json:"runtime_id"`
	Source    string          `json:"source"`
	Payload   json.RawMessage `json:"payload"`
}

type Error struct {
	Code       string
	Message    string
	HTTPStatus int
	Details    map[string]any
	Cause      error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

func NewError(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: status, Details: map[string]any{}}
}

func Wrap(code, message string, status int, cause error) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: status, Details: map[string]any{}, Cause: cause}
}

func AsError(err error) *Error {
	var de *Error
	if errors.As(err, &de) {
		return de
	}
	return Wrap("INTERNAL_ERROR", "internal error", http.StatusInternalServerError, err)
}

func ValidateInstruction(content string, byteLimit int) error {
	if strings.TrimSpace(content) == "" {
		return NewError("INVALID_REQUEST", "instruction must contain non-whitespace characters", http.StatusBadRequest)
	}
	if len([]byte(content)) > byteLimit {
		return NewError("REQUEST_TOO_LARGE", "instruction exceeds the 1 MiB limit", http.StatusRequestEntityTooLarge)
	}
	return nil
}

func ValidateProfile(profile RuntimeProfile) error {
	if profile.Agent != "cc" || profile.AgentVersion == "" {
		return NewError("INVALID_REQUEST", "unsupported runtime profile", http.StatusBadRequest)
	}
	for i, capability := range profile.Capabilities {
		if capability == "" || (i > 0 && profile.Capabilities[i-1] >= capability) {
			return NewError("INVALID_REQUEST", "runtime profile capabilities must be sorted and unique", http.StatusBadRequest)
		}
	}
	return nil
}
