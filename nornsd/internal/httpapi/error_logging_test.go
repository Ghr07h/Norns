package httpapi

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
)

func TestServiceErrorsLogCauseWithoutExposingItInResponse(t *testing.T) {
	var logs bytes.Buffer
	server := New(Config{Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	cause := errors.New("database is locked (SQLITE_BUSY)")
	failure := domain.Wrap("SERVICE_UNAVAILABLE", "cannot begin database transaction", http.StatusServiceUnavailable, cause)
	failure.Details["sqlite_code"] = 5
	response := httptest.NewRecorder()
	server.writeError(response, "req_database_failure", failure)
	for _, expected := range []string{"database is locked", "SQLITE_BUSY", "req_database_failure", "SERVICE_UNAVAILABLE", `"sqlite_code":5`} {
		if !strings.Contains(logs.String(), expected) {
			t.Errorf("server logs omitted %q: %s", expected, logs.String())
		}
	}
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), cause.Error()) {
		t.Fatalf("unexpected public error response: %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"sqlite_code":5`) {
		t.Fatalf("response omitted SQLite error code: %s", response.Body.String())
	}
}

func TestServiceErrorResponseIncludesExplicitSQLiteReason(t *testing.T) {
	var logs bytes.Buffer
	server := New(Config{Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	failure := domain.Wrap("SERVICE_UNAVAILABLE", "cannot begin database transaction", http.StatusServiceUnavailable, errors.New("private internal context"))
	failure.Details["sqlite_code"] = 5
	failure.Details["sqlite_error"] = "database is locked (SQLITE_BUSY)"
	response := httptest.NewRecorder()
	server.writeError(response, "req_database_failure", failure)
	for _, expected := range []string{"database is locked (SQLITE_BUSY)", `"sqlite_code":5`, "req_database_failure"} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("response omitted diagnostic %q: %s", expected, response.Body.String())
		}
	}
	if strings.Contains(response.Body.String(), "private internal context") {
		t.Fatalf("response exposed an unstructured internal cause: %s", response.Body.String())
	}
}
