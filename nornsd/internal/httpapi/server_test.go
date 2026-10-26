package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/app"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	runtimemanager "github.com/Ghr07h/norns/nornsd/internal/runtime"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

func TestAPIAuthenticationAndSubjectLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rootPath := t.TempDir()
	owner, err := root.Acquire(ctx, rootPath, "test", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}
	manager := runtimemanager.NewManager(ctx, runtimemanager.Options{Owner: owner, Profile: profile, Concurrency: 1, SlotConfigs: nil})
	manager.Start()
	defer func() { manager.Close(); _ = owner.Close() }()
	service := &app.Service{Owner: owner, Store: owner.Store, Manager: manager, Profile: profile}
	server := New(Config{Service: service, Token: "secret", Concurrency: 1, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	request := httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	var info struct {
		ServerID string `json:"server_id"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &info) != nil || info.ServerID != owner.Identity.RootID {
		t.Fatalf("service identity response=%d body=%s", response.Code, response.Body.String())
	}
	body := bytes.NewBufferString(`{"name":"subject"}`)
	request = httptest.NewRequest(http.MethodPost, "/api/v1/subjects", body)
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	var created map[string]string
	if json.Unmarshal(response.Body.Bytes(), &created) != nil || created["subject_id"] == "" {
		t.Fatalf("invalid create response: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/subjects/"+created["subject_id"], nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", response.Code, response.Body.String())
	}
	var detailFields map[string]json.RawMessage
	if err = json.Unmarshal(response.Body.Bytes(), &detailFields); err != nil {
		t.Fatal(err)
	}
	if _, ok := detailFields["runtime"]; ok {
		t.Fatalf("subject detail must not embed runtime: %s", response.Body.String())
	}
	var detail struct {
		Workspace struct {
			Materialized bool    `json:"materialized"`
			Path         *string `json:"path"`
		} `json:"workspace"`
	}
	if json.Unmarshal(response.Body.Bytes(), &detail) != nil || !detail.Workspace.Materialized || detail.Workspace.Path == nil || !filepath.IsAbs(*detail.Workspace.Path) {
		t.Fatalf("invalid workspace path: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/subjects", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	var page struct {
		Subjects []map[string]json.RawMessage `json:"subjects"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil || len(page.Subjects) != 1 {
		t.Fatalf("invalid subject list: %s", response.Body.String())
	}
	if _, ok := page.Subjects[0]["runtime"]; ok {
		t.Fatalf("subject list must not embed runtime: %s", response.Body.String())
	}
	var subjectPageFields map[string]json.RawMessage
	if json.Unmarshal(response.Body.Bytes(), &subjectPageFields) != nil {
		t.Fatalf("invalid subject page: %s", response.Body.String())
	}
	if _, ok := subjectPageFields["total"]; ok {
		t.Fatalf("subject list must not include total: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/subjects/"+created["subject_id"]+"/events", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	var eventPage struct {
		Events     []domain.Event `json:"events"`
		Cursor     string         `json:"cursor"`
		NextCursor *string        `json:"next_cursor"`
		HasMore    bool           `json:"has_more"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &eventPage) != nil || len(eventPage.Events) != 1 || eventPage.Cursor == "" || eventPage.NextCursor != nil || eventPage.HasMore {
		t.Fatalf("invalid initial event page: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/subjects/"+created["subject_id"]+"/events/updates?cursor="+eventPage.Cursor, nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "{\"has_updates\":false}\n" {
		t.Fatalf("invalid unchanged event response: %s", response.Body.String())
	}
	if _, err = owner.Store.AppendEvent(ctx, created["subject_id"], nil, "diagnostic", "test", map[string]any{"code": "NEW"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err = manager.RepairReadable(ctx, created["subject_id"]); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/subjects/"+created["subject_id"]+"/events/updates?cursor="+eventPage.Cursor, nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "{\"has_updates\":true}\n" {
		t.Fatalf("invalid updated event response: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/subjects/"+created["subject_id"]+"/events?after="+eventPage.Cursor, nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &eventPage) != nil || len(eventPage.Events) != 1 || eventPage.Events[0].Type != "diagnostic" || eventPage.Cursor == "" {
		t.Fatalf("invalid incremental event page: %s", response.Body.String())
	}
	runtime, _, err := service.Submit(ctx, created["subject_id"], "inspect artifacts", false)
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/subjects/"+created["subject_id"]+"/runtime", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	var current struct {
		Runtime *struct {
			ID           string               `json:"id"`
			Instructions []domain.Instruction `json:"instructions"`
		} `json:"runtime"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &current) != nil || current.Runtime == nil || current.Runtime.ID != runtime.ID || len(current.Runtime.Instructions) != 1 {
		t.Fatalf("invalid current runtime: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/subjects/"+created["subject_id"]+"/runtimes", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	var runtimePage struct {
		Runtimes   []map[string]json.RawMessage `json:"runtimes"`
		TotalCount int                          `json:"total_count"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &runtimePage) != nil || len(runtimePage.Runtimes) != 1 || runtimePage.TotalCount != 1 {
		t.Fatalf("invalid runtime page: %s", response.Body.String())
	}
	if _, ok := runtimePage.Runtimes[0]["started_at"]; !ok {
		t.Fatalf("runtime summary is missing started_at: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/queue", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	var queuePageFields map[string]json.RawMessage
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &queuePageFields) != nil {
		t.Fatalf("invalid queue page: %s", response.Body.String())
	}
	if _, ok := queuePageFields["total"]; ok {
		t.Fatalf("queue list must not include total: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/subjects/"+created["subject_id"]+"/runtimes/"+runtime.ID, nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	var runtimeDetail struct {
		SessionID string `json:"session_id"`
		Artifacts struct {
			Raw              *string `json:"raw_log_path"`
			Readable         *string `json:"readable_log_path"`
			MemorySamples    *string `json:"memory_samples_path"`
			Transcript       *string `json:"transcript_path"`
			TranscriptStatus string  `json:"transcript_status"`
		} `json:"artifacts"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &runtimeDetail) != nil || runtimeDetail.SessionID != runtime.ID || runtimeDetail.Artifacts.Raw == nil || runtimeDetail.Artifacts.Readable == nil || runtimeDetail.Artifacts.MemorySamples != nil || runtimeDetail.Artifacts.Transcript != nil || runtimeDetail.Artifacts.TranscriptStatus != "pending" {
		t.Fatalf("invalid initial artifacts: %s", response.Body.String())
	}
	if err = owner.Store.UpdateRuntimeTranscript(ctx, created["subject_id"], runtime.ID, map[string]any{"status": "missing", "session_id": runtime.ID, "copied": []string{}, "missing": []string{runtime.ID + ".jsonl", runtime.ID}}); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/subjects/"+created["subject_id"]+"/runtimes/"+runtime.ID, nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if json.Unmarshal(response.Body.Bytes(), &runtimeDetail) != nil || runtimeDetail.Artifacts.Transcript != nil || runtimeDetail.Artifacts.TranscriptStatus != "missing" {
		t.Fatalf("invalid missing transcript artifact: %s", response.Body.String())
	}
	transcriptPath := filepath.Join(*detail.Workspace.Path, ".nornsd", "runtimes", runtime.ID, runtime.ID+".jsonl")
	if err = os.MkdirAll(filepath.Dir(transcriptPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(transcriptPath, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	memorySamplesPath := filepath.Join(filepath.Dir(transcriptPath), "memory-samples.jsonl")
	if err = os.WriteFile(memorySamplesPath, []byte("{\"sampled_at\":\"2026-10-21T00:00:00Z\",\"memory_bytes\":1024}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = owner.Store.UpdateRuntimeTranscript(ctx, created["subject_id"], runtime.ID, map[string]any{"status": "complete", "session_id": runtime.ID, "copied": []string{runtime.ID + ".jsonl"}, "missing": []string{}}); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/subjects/"+created["subject_id"]+"/runtimes/"+runtime.ID, nil)
	request.Header.Set("Authorization", "Bearer secret")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if json.Unmarshal(response.Body.Bytes(), &runtimeDetail) != nil || runtimeDetail.Artifacts.MemorySamples == nil || *runtimeDetail.Artifacts.MemorySamples != memorySamplesPath || runtimeDetail.Artifacts.Transcript == nil || *runtimeDetail.Artifacts.Transcript != transcriptPath || runtimeDetail.Artifacts.TranscriptStatus != "available" {
		t.Fatalf("invalid transcript artifact: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/subjects", bytes.NewBufferString(`{"unknown":true}`))
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d", response.Code)
	}
	for _, tc := range []struct {
		method string
		path   string
		status int
		code   string
	}{
		{http.MethodGet, "/api/v1/subjects?unknown=true", http.StatusBadRequest, "INVALID_REQUEST"},
		{http.MethodGet, "/api/v1/queue?limit=1&limit=2", http.StatusBadRequest, "INVALID_REQUEST"},
		{http.MethodGet, "/api/v1/subjects/" + created["subject_id"] + "/norns/views/visual?lang=de", http.StatusBadRequest, "INVALID_REQUEST"},
		{http.MethodPost, "/api/v1/health", http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{http.MethodGet, "/api/v1/not-a-route", http.StatusNotFound, "NOT_FOUND"},
	} {
		request = httptest.NewRequest(tc.method, tc.path, nil)
		request.Header.Set("Authorization", "Bearer secret")
		response = httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("%s %s status=%d body=%s", tc.method, tc.path, response.Code, response.Body.String())
		}
		var envelope struct {
			Error struct {
				Code      string         `json:"code"`
				RequestID string         `json:"request_id"`
				Details   map[string]any `json:"details"`
			} `json:"error"`
		}
		if err = json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != tc.code || envelope.Error.RequestID == "" || envelope.Error.Details == nil {
			t.Fatalf("invalid error envelope for %s: %s", tc.path, response.Body.String())
		}
	}
}

func TestSubmitCanForkLatestRuntimeSession(t *testing.T) {
	ctx := context.Background()
	rootPath := t.TempDir()
	owner, err := root.Acquire(ctx, rootPath, "test", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	profile := domain.RuntimeProfile{Agent: "cc", AgentVersion: version.ClaudeCodeVersion}
	manager := runtimemanager.NewManager(ctx, runtimemanager.Options{Owner: owner, Profile: profile, Concurrency: 1})
	defer manager.Close()
	service := &app.Service{Owner: owner, Store: owner.Store, Manager: manager, Profile: profile}
	server := New(Config{Service: service, Token: "secret", Concurrency: 1, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	withoutHistory, err := service.CreateSubject(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/subjects/"+withoutHistory.ID+"/runtime", bytes.NewBufferString(`{"instruction":"continue","resume_session":true}`))
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte("RUNTIME_SESSION_NOT_FOUND")) {
		t.Fatalf("resume without history status=%d body=%s", response.Code, response.Body.String())
	}

	subject, err := service.CreateSubject(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	previousID := "00000000-0000-4000-8000-000000000001"
	if _, err = owner.Store.CreateDirectRuntime(ctx, subject.ID, previousID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err = owner.Store.SetRuntimeTerminal(ctx, subject.ID, previousID, domain.RuntimeEnded, "completed", "", time.Now().UTC(), true); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/subjects/"+subject.ID+"/runtime", bytes.NewBufferString(`{"instruction":"continue","resume_session":true}`))
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	var accepted struct {
		Runtime struct {
			ID string `json:"id"`
		} `json:"runtime"`
	}
	if response.Code != http.StatusAccepted || json.Unmarshal(response.Body.Bytes(), &accepted) != nil || accepted.Runtime.ID == "" {
		t.Fatalf("resume status=%d body=%s", response.Code, response.Body.String())
	}
	resumed, err := owner.Store.GetRuntime(ctx, subject.ID, accepted.Runtime.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.SessionMode != "resume" || !bytes.Contains(resumed.ProcessJSON, []byte(previousID)) {
		t.Fatalf("resumed Runtime mode=%q process=%s", resumed.SessionMode, resumed.ProcessJSON)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/subjects/"+subject.ID+"/runtime", bytes.NewBufferString(`{"instruction":"again","resume_session":true}`))
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte("SUBJECT_RUNTIME_ACTIVE")) {
		t.Fatalf("resume with active Runtime status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSetViewFilename(t *testing.T) {
	response := httptest.NewRecorder()
	setViewFilename(response, "status-20261022T080000.123456789Z-a1b2c3d4.json")
	if got := response.Header().Get("X-Norns-View-Filename"); got != "status-20261022T080000.123456789Z-a1b2c3d4.json" {
		t.Fatalf("view filename header = %q", got)
	}
	response = httptest.NewRecorder()
	setViewFilename(response, "")
	if got := response.Header().Get("X-Norns-View-Filename"); got != "" {
		t.Fatalf("empty view filename set header %q", got)
	}
}
