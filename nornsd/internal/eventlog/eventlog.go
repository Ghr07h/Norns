package eventlog

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	storesqlite "github.com/Ghr07h/norns/nornsd/internal/store/sqlite"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

type Logger struct {
	store                                       *storesqlite.Store
	subjectID, runtimeID, rawPath, readablePath string
	mu                                          *sync.Mutex
	state                                       *logState
	tools                                       map[string]uint64
	closeTools                                  map[string]bool
}

type logState struct {
	mu          sync.Mutex
	readableSeq uint64
	initialized bool
}

var subjectLogs sync.Map

type rawEnvelope struct {
	RawSeq     uint64          `json:"raw_seq"`
	Timestamp  time.Time       `json:"timestamp"`
	RuntimeID  string          `json:"runtime_id"`
	Stream     string          `json:"stream"`
	Kind       string          `json:"kind"`
	Payload    json.RawMessage `json:"payload"`
	Projection *rawProjection  `json:"projection,omitempty"`
}

type rawProjection struct {
	Type            string `json:"type"`
	ResumeSessionID string `json:"resume_session_id,omitempty"`
}

const sessionResumedEventType = "runtime.session_resumed"

func New(store *storesqlite.Store, subjectPath, subjectID, runtimeID string) *Logger {
	logPath := filepath.Join(subjectPath, ".nornsd", "log")
	state, _ := subjectLogs.LoadOrStore(logPath, &logState{})
	log := state.(*logState)
	return &Logger{store: store, subjectID: subjectID, runtimeID: runtimeID, rawPath: filepath.Join(logPath, "raw.jsonl"), readablePath: filepath.Join(logPath, "readable.jsonl"), mu: &log.mu, state: log, tools: map[string]uint64{}, closeTools: map[string]bool{}}
}

func (l *Logger) RawHandler(ctx context.Context) func(string, string, json.RawMessage) error {
	return func(stream, kind string, payload json.RawMessage) error {
		_, err := l.Record(ctx, stream, kind, payload, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("event projector: %w", err)
		}
		return nil
	}
}

func (l *Logger) Record(ctx context.Context, stream, kind string, payload json.RawMessage, now time.Time) (MessageResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	seq, err := l.store.NextRawSeq(ctx, l.subjectID)
	if err != nil {
		return MessageResult{}, err
	}
	record := rawEnvelope{RawSeq: seq, Timestamp: now.UTC(), RuntimeID: l.runtimeID, Stream: stream, Kind: kind, Payload: payload}
	if err = appendJSONLine(l.rawPath, record); err != nil {
		return MessageResult{}, err
	}
	return l.projectRecordLocked(ctx, record)
}

// RecordSessionResumed preserves a replayed startup result in the raw log while
// projecting it as Runtime lifecycle metadata instead of a new turn.
func (l *Logger) RecordSessionResumed(ctx context.Context, payload json.RawMessage, resumeSessionID string, now time.Time) error {
	if _, err := sessionResumedPayload(payload, resumeSessionID, l.runtimeID); err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	rawSeq, err := l.store.NextRawSeq(ctx, l.subjectID)
	if err != nil {
		return err
	}
	record := rawEnvelope{
		RawSeq:    rawSeq,
		Timestamp: now.UTC(),
		RuntimeID: l.runtimeID,
		Stream:    "stdout",
		Kind:      "agent_message",
		Payload:   payload,
		Projection: &rawProjection{
			Type:            sessionResumedEventType,
			ResumeSessionID: resumeSessionID,
		},
	}
	if err = appendJSONLine(l.rawPath, record); err != nil {
		return err
	}
	_, err = l.projectRecordLocked(ctx, record)
	return err
}

func sessionResumedPayload(payload json.RawMessage, resumeSessionID, runtimeID string) (map[string]any, error) {
	var envelope struct {
		Subtype        string `json:"subtype"`
		SessionID      string `json:"session_id"`
		IsError        bool   `json:"is_error"`
		NumTurns       int    `json:"num_turns"`
		Usage          any    `json:"usage"`
		Result         any    `json:"result"`
		StopReason     string `json:"stop_reason"`
		TerminalReason string `json:"terminal_reason"`
		APIErrorStatus any    `json:"api_error_status"`
		Origin         struct {
			Kind string `json:"kind"`
		} `json:"origin"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, err
	}
	eventPayload := map[string]any{
		"resume_session_id": resumeSessionID,
		"session_id":        runtimeID,
		"subtype":           envelope.Subtype,
		"is_error":          envelope.IsError,
		"num_turns":         envelope.NumTurns,
		"usage":             envelope.Usage,
	}
	if envelope.SessionID != "" {
		eventPayload["agent_session_id"] = envelope.SessionID
	}
	if envelope.Result != nil {
		eventPayload["result"] = envelope.Result
	}
	if envelope.StopReason != "" {
		eventPayload["stop_reason"] = envelope.StopReason
	}
	if envelope.TerminalReason != "" {
		eventPayload["terminal_reason"] = envelope.TerminalReason
	}
	if envelope.APIErrorStatus != nil {
		eventPayload["api_error_status"] = envelope.APIErrorStatus
	}
	if envelope.Origin.Kind != "" {
		eventPayload["origin_kind"] = envelope.Origin.Kind
	}
	return eventPayload, nil
}

func (l *Logger) projectSessionResumedRecordLocked(ctx context.Context, record rawEnvelope) (MessageResult, error) {
	eventPayload, err := sessionResumedPayload(record.Payload, record.Projection.ResumeSessionID, record.RuntimeID)
	if err != nil {
		return MessageResult{}, err
	}
	_, projected, err := l.store.ProjectRaw(ctx, l.subjectID, record.RuntimeID, record.RawSeq, 1, record.Timestamp, func(uint64) []storesqlite.ProjectedEvent {
		return []storesqlite.ProjectedEvent{{Type: sessionResumedEventType, Source: "runtime", Payload: eventPayload}}
	})
	if err != nil {
		return MessageResult{}, err
	}
	result := MessageResult{projectionChanged: projected}
	if projected {
		if err = syncReadableLocked(ctx, l.store, l.readablePath, l.subjectID, l.state); err != nil {
			return MessageResult{}, err
		}
	}
	return result, nil
}

func (l *Logger) Event(ctx context.Context, eventType, source string, payload any, now time.Time) (domain.Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	event, err := l.store.AppendEvent(ctx, l.subjectID, &l.runtimeID, eventType, source, payload, now)
	if err != nil {
		return event, err
	}
	if err = syncReadableLocked(ctx, l.store, l.readablePath, l.subjectID, l.state); err != nil {
		return event, err
	}
	return event, nil
}

func appendJSONLine(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	if err = encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

type MessageResult struct {
	IsResult          bool
	IsError           bool
	Usage             any
	CloseSucceeded    bool
	projectionChanged bool
}

type eventDraft struct {
	typeName  string
	source    string
	payload   map[string]any
	linkIndex *int
	linkSeq   uint64
}

type toolReference struct {
	index *int
	seq   uint64
}

func (l *Logger) projectRecordLocked(ctx context.Context, record rawEnvelope) (MessageResult, error) {
	if record.Projection != nil {
		switch record.Projection.Type {
		case sessionResumedEventType:
			return l.projectSessionResumedRecordLocked(ctx, record)
		default:
			return MessageResult{}, fmt.Errorf("unsupported raw projection %q", record.Projection.Type)
		}
	}
	if record.Stream != "stdout" || record.Kind != "agent_message" {
		_, _, err := l.store.ProjectRaw(ctx, l.subjectID, record.RuntimeID, record.RawSeq, 0, record.Timestamp, func(uint64) []storesqlite.ProjectedEvent { return nil })
		return MessageResult{}, err
	}

	result, drafts, tools, closeTools, err := l.buildProjection(record.Payload)
	if err != nil {
		return MessageResult{}, err
	}
	events, projected, err := l.store.ProjectRaw(ctx, l.subjectID, record.RuntimeID, record.RawSeq, len(drafts), record.Timestamp, func(firstSeq uint64) []storesqlite.ProjectedEvent {
		out := make([]storesqlite.ProjectedEvent, len(drafts))
		for i, draft := range drafts {
			payload := make(map[string]any, len(draft.payload)+1)
			for key, value := range draft.payload {
				payload[key] = value
			}
			if draft.linkIndex != nil {
				payload["tool_call_seq"] = firstSeq + uint64(*draft.linkIndex)
			} else if draft.linkSeq != 0 {
				payload["tool_call_seq"] = draft.linkSeq
			}
			out[i] = storesqlite.ProjectedEvent{Type: draft.typeName, Source: draft.source, Payload: payload}
		}
		return out
	})
	if err != nil {
		return MessageResult{}, err
	}
	for id, ref := range tools {
		if ref.index != nil {
			if *ref.index >= len(events) {
				return MessageResult{}, fmt.Errorf("tool event index %d is unavailable", *ref.index)
			}
			ref.seq = events[*ref.index].Seq
			ref.index = nil
			tools[id] = ref
		}
	}
	l.tools = make(map[string]uint64, len(tools))
	for id, ref := range tools {
		l.tools[id] = ref.seq
	}
	l.closeTools = closeTools
	if projected {
		result.projectionChanged = true
		if err = syncReadableLocked(ctx, l.store, l.readablePath, l.subjectID, l.state); err != nil {
			return MessageResult{}, err
		}
	}
	return result, nil
}

func (l *Logger) buildProjection(raw json.RawMessage) (MessageResult, []eventDraft, map[string]toolReference, map[string]bool, error) {
	var envelope struct {
		Type             string `json:"type"`
		Subtype          string `json:"subtype"`
		SessionID        string `json:"session_id"`
		IsError          bool   `json:"is_error"`
		NumTurns         int    `json:"num_turns"`
		Usage            any    `json:"usage"`
		Result           any    `json:"result"`
		StopReason       string `json:"stop_reason"`
		TerminalReason   string `json:"terminal_reason"`
		APIErrorStatus   any    `json:"api_error_status"`
		AttributionAgent string `json:"attributionAgent"`
		SubagentType     string `json:"subagent_type"`
		Origin           struct {
			Kind string `json:"kind"`
		} `json:"origin"`
		Message struct {
			Content []json.RawMessage `json:"content"`
		} `json:"message"`
		Content []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return MessageResult{}, nil, nil, nil, err
	}
	result := MessageResult{}
	drafts := []eventDraft{}
	agent := projectedAgent(envelope.AttributionAgent, envelope.SubagentType)
	tools := make(map[string]toolReference, len(l.tools))
	for id, seq := range l.tools {
		tools[id] = toolReference{seq: seq}
	}
	closeTools := make(map[string]bool, len(l.closeTools))
	for id, closeTool := range l.closeTools {
		closeTools[id] = closeTool
	}
	blocks := envelope.Message.Content
	if len(blocks) == 0 {
		blocks = envelope.Content
	}
	for _, blockRaw := range blocks {
		var block map[string]any
		if json.Unmarshal(blockRaw, &block) != nil {
			continue
		}
		kind, _ := block["type"].(string)
		switch kind {
		case "text":
			if text, _ := block["text"].(string); text != "" {
				drafts = append(drafts, eventDraft{typeName: "agent.text", source: "agent", payload: map[string]any{"text": text, "agent": agent}})
			}
		case "tool_use":
			id, _ := block["id"].(string)
			name, _ := block["name"].(string)
			payload := map[string]any{"tool_call_id": id, "name": name, "input": block["input"], "agent": agent}
			index := len(drafts)
			drafts = append(drafts, eventDraft{typeName: "agent.tool_call", source: "agent", payload: payload})
			if id != "" {
				tools[id] = toolReference{index: &index}
				if name == "Bash" && isCloseCommand(toolCommand(block["input"])) {
					closeTools[id] = true
				}
			}
		case "tool_result":
			id, _ := block["tool_use_id"].(string)
			ref, found := tools[id]
			if !found {
				drafts = append(drafts, eventDraft{typeName: "diagnostic", source: "runtime", payload: map[string]any{"code": "UNMATCHED_TOOL_RESULT", "tool_call_id": id}})
			}
			draft := eventDraft{typeName: "agent.tool_result", source: "agent", payload: map[string]any{"tool_call_id": id, "content": block["content"], "is_error": block["is_error"], "agent": agent}}
			if found {
				draft.linkIndex = ref.index
				draft.linkSeq = ref.seq
			}
			drafts = append(drafts, draft)
			isError, _ := block["is_error"].(bool)
			result.CloseSucceeded = result.CloseSucceeded || (closeTools[id] && !isError && hasCloseSuccess(block["content"]))
			delete(tools, id)
			delete(closeTools, id)
		}
	}
	if envelope.Type == "result" {
		result.IsResult = true
		result.IsError = envelope.IsError
		result.Usage = envelope.Usage
		payload := map[string]any{"subtype": envelope.Subtype, "is_error": envelope.IsError, "num_turns": envelope.NumTurns, "usage": envelope.Usage, "agent": agent}
		if envelope.SessionID != "" {
			payload["session_id"] = envelope.SessionID
		}
		if envelope.Result != nil {
			payload["result"] = envelope.Result
		}
		if envelope.StopReason != "" {
			payload["stop_reason"] = envelope.StopReason
		}
		if envelope.TerminalReason != "" {
			payload["terminal_reason"] = envelope.TerminalReason
		}
		if envelope.APIErrorStatus != nil {
			payload["api_error_status"] = envelope.APIErrorStatus
		}
		if envelope.Origin.Kind != "" {
			payload["origin_kind"] = envelope.Origin.Kind
		}
		drafts = append(drafts, eventDraft{typeName: "agent.result", source: "agent", payload: payload})
	}
	return result, drafts, tools, closeTools, nil
}

func projectedAgent(attribution, subagentType string) map[string]any {
	source := strings.TrimSpace(subagentType)
	if source == "" {
		source = strings.TrimSpace(attribution)
	}
	if source == "" {
		return map[string]any{"role": "main", "name": "Coordinator"}
	}
	name := source
	if index := strings.LastIndex(name, ":"); index >= 0 {
		name = name[index+1:]
	}
	parts := strings.Split(name, "-")
	for index, part := range parts {
		if part == "" {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + part[1:]
	}
	return map[string]any{"role": "sub", "name": strings.Join(parts, "-"), "source": source}
}

func toolCommand(input any) string {
	m, _ := input.(map[string]any)
	command, _ := m["command"].(string)
	return command
}

var closeCommand = regexp.MustCompile(`(?:^|&&|\|\||;|\||\n)\s*(?:(?:exec|command)\s+)?(?:"[^"\n]*/norns"|'[^'\n]*/norns'|[^\s;&|()]*/norns|norns)\s+task\s+close(?:\s|$)`)

func isCloseCommand(command string) bool { return closeCommand.MatchString(command) }
func hasCloseSuccess(value any) bool {
	switch v := value.(type) {
	case string:
		for i, r := range v {
			if r != '{' && r != '[' {
				continue
			}
			decoder := json.NewDecoder(strings.NewReader(v[i:]))
			var item any
			if decoder.Decode(&item) == nil && hasCloseSuccess(item) {
				return true
			}
		}
		return false
	case []any:
		for _, item := range v {
			if hasCloseSuccess(item) {
				return true
			}
		}
	case map[string]any:
		ok, _ := v["ok"].(bool)
		data, _ := v["data"].(map[string]any)
		if ok && data != nil {
			if _, hasID := data["id"]; hasID {
				if _, hasOutcome := data["outcome"]; hasOutcome {
					return true
				}
			}
		}
		for _, item := range v {
			if hasCloseSuccess(item) {
				return true
			}
		}
	}
	return false
}

// ReplayRaw deterministically replays a Subject raw log. Records already at or
// before the SQLite checkpoint are read to reconstruct tool correlation state,
// while ProjectRaw keeps their stable Events idempotent.
func ReplayRaw(ctx context.Context, store *storesqlite.Store, subjectPath, subjectID string) error {
	path := filepath.Join(subjectPath, ".nornsd", "log", "raw.jsonl")
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	logger := New(store, subjectPath, subjectID, "")
	logger.mu.Lock()
	defer logger.mu.Unlock()
	reader := bufio.NewReaderSize(file, 64*1024)
	var offset int64
	var previous uint64
	projectionChanged := false
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > version.MaxAgentFrameBytes+1024*1024 {
			return fmt.Errorf("raw log record exceeds maximum size")
		}
		if len(line) > 0 {
			if line[len(line)-1] != '\n' {
				if errors.Is(readErr, io.EOF) {
					if err = file.Truncate(offset); err != nil {
						return err
					}
					break
				}
				return fmt.Errorf("raw log record is not newline terminated")
			}
			var record rawEnvelope
			if err = json.Unmarshal(bytesTrimNewline(line), &record); err != nil {
				return fmt.Errorf("invalid raw log record after byte %d: %w", offset, err)
			}
			if record.RawSeq == 0 || record.RawSeq <= previous || record.RuntimeID == "" || record.Stream == "" || record.Kind == "" || !json.Valid(record.Payload) {
				return fmt.Errorf("invalid raw log envelope after byte %d", offset)
			}
			if record.Projection == nil {
				record.Projection, err = legacyRawProjection(ctx, store, subjectID, record)
				if err != nil {
					return fmt.Errorf("inspect raw sequence %d projection: %w", record.RawSeq, err)
				}
			}
			projection, projectErr := logger.projectRecordLocked(ctx, record)
			if projectErr != nil {
				err = projectErr
				return fmt.Errorf("project raw sequence %d: %w", record.RawSeq, err)
			}
			projectionChanged = projectionChanged || projection.projectionChanged
			previous = record.RawSeq
			offset += int64(len(line))
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return readErr
		}
	}
	if projectionChanged {
		if err = rebuild(ctx, store, logger.readablePath, subjectID); err != nil {
			return err
		}
		subject, subjectErr := store.GetSubject(ctx, subjectID)
		if subjectErr != nil {
			return subjectErr
		}
		logger.state.readableSeq = subject.LastEventSeq
		logger.state.initialized = true
	}
	return nil
}

func legacyRawProjection(ctx context.Context, store *storesqlite.Store, subjectID string, record rawEnvelope) (*rawProjection, error) {
	if record.Stream != "stdout" || record.Kind != "agent_message" {
		return nil, nil
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(record.Payload, &envelope) != nil || envelope.Type != "result" {
		return nil, nil
	}
	events, err := store.ListRawProjectionEvents(ctx, subjectID, record.RawSeq)
	if err != nil {
		return nil, err
	}
	if len(events) != 1 || events[0].Type != sessionResumedEventType || events[0].Source != "runtime" {
		return nil, nil
	}
	var payload struct {
		ResumeSessionID string `json:"resume_session_id"`
	}
	if err = json.Unmarshal(events[0].Payload, &payload); err != nil {
		return nil, fmt.Errorf("decode legacy session resume projection: %w", err)
	}
	return &rawProjection{Type: sessionResumedEventType, ResumeSessionID: payload.ResumeSessionID}, nil
}

func bytesTrimNewline(line []byte) []byte {
	trimmed := strings.TrimSuffix(string(line), "\n")
	trimmed = strings.TrimSuffix(trimmed, "\r")
	return []byte(trimmed)
}

func RepairReadable(ctx context.Context, store *storesqlite.Store, subjectPath, subjectID string) error {
	path := filepath.Join(subjectPath, ".nornsd", "log", "readable.jsonl")
	value, _ := subjectLogs.LoadOrStore(filepath.Dir(path), &logState{})
	state := value.(*logState)
	state.mu.Lock()
	defer state.mu.Unlock()
	corrupt := false
	if file, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), version.MaxAgentFrameBytes+1024*1024)
		var last uint64
		for scanner.Scan() {
			var event domain.Event
			if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Seq != last+1 {
				corrupt = true
				break
			}
			last = event.Seq
		}
		_ = file.Close()
		if scanner.Err() != nil {
			corrupt = true
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if corrupt {
		backup := path + ".corrupt." + time.Now().UTC().Format("20060102T150405.000000000Z")
		if err := os.Rename(path, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := rebuild(ctx, store, path, subjectID); err != nil {
		state.initialized = false
		return err
	}
	subject, err := store.GetSubject(ctx, subjectID)
	if err != nil {
		state.initialized = false
		return err
	}
	state.readableSeq = subject.LastEventSeq
	state.initialized = true
	return nil
}

// SyncReadable appends committed Events that are not yet present in a healthy
// readable projection. It falls back to a full rebuild if the existing file is
// malformed or has a sequence gap.
func SyncReadable(ctx context.Context, store *storesqlite.Store, subjectPath, subjectID string) error {
	path := filepath.Join(subjectPath, ".nornsd", "log", "readable.jsonl")
	value, _ := subjectLogs.LoadOrStore(filepath.Dir(path), &logState{})
	state := value.(*logState)
	state.mu.Lock()
	defer state.mu.Unlock()
	return syncReadableLocked(ctx, store, path, subjectID, state)
}

func syncReadableLocked(ctx context.Context, store *storesqlite.Store, path, subjectID string, state *logState) error {
	if !state.initialized {
		var last uint64
		file, err := os.Open(path)
		if err == nil {
			scanner := bufio.NewScanner(file)
			scanner.Buffer(make([]byte, 64*1024), version.MaxAgentFrameBytes+1024*1024)
			for scanner.Scan() {
				var event domain.Event
				if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Seq != last+1 {
					_ = file.Close()
					if err = rebuild(ctx, store, path, subjectID); err != nil {
						return err
					}
					subject, getErr := store.GetSubject(ctx, subjectID)
					if getErr != nil {
						return getErr
					}
					state.readableSeq = subject.LastEventSeq
					state.initialized = true
					return nil
				}
				last = event.Seq
			}
			closeErr := file.Close()
			if scanner.Err() != nil || closeErr != nil {
				return errors.Join(scanner.Err(), closeErr)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		state.readableSeq = last
		state.initialized = true
	}
	for {
		events, listErr := store.ListEvents(ctx, subjectID, 1000, &state.readableSeq, nil)
		if listErr != nil {
			return listErr
		}
		for _, event := range events {
			if event.Seq != state.readableSeq+1 {
				state.initialized = false
				return fmt.Errorf("readable event sequence gap before %d", event.Seq)
			}
			if err := appendJSONLine(path, event); err != nil {
				return err
			}
			state.readableSeq = event.Seq
		}
		if len(events) < 1000 {
			return nil
		}
	}
}

func rebuild(ctx context.Context, store *storesqlite.Store, path, subjectID string) error {
	tmp := path + ".rebuild"
	_ = os.Remove(tmp)
	var after uint64
	for {
		events, err := store.ListEvents(ctx, subjectID, 1000, &after, nil)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			break
		}
		for _, event := range events {
			if err = appendJSONLine(tmp, event); err != nil {
				return err
			}
			after = event.Seq
		}
		if len(events) < 1000 {
			break
		}
	}
	if _, err := os.Stat(tmp); errors.Is(err, os.ErrNotExist) {
		if err = os.WriteFile(tmp, nil, 0600); err != nil {
			return err
		}
	}
	return os.Rename(tmp, path)
}

func (l *Logger) Diagnostic(ctx context.Context, code string, err error) {
	payload := map[string]any{"code": code}
	if err != nil {
		payload["message"] = fmt.Sprint(err)
	}
	_, _ = l.Event(ctx, "diagnostic", "runtime", payload, time.Now().UTC())
}
