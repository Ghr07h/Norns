package httpapi

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/app"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	nornsadapter "github.com/Ghr07h/norns/nornsd/internal/norns"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	runtimemanager "github.com/Ghr07h/norns/nornsd/internal/runtime"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

func newArchiveTestServer(t *testing.T, norns *nornsadapter.Adapter) (*Server, domain.Subject, string) {
	t.Helper()
	ctx := context.Background()
	owner, err := root.Acquire(ctx, t.TempDir(), "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	manager := runtimemanager.NewManager(ctx, runtimemanager.Options{Owner: owner, Concurrency: 1})
	t.Cleanup(func() { manager.Close(); _ = owner.Close() })
	service := &app.Service{Owner: owner, Store: owner.Store, Manager: manager, Norns: norns}
	subject, err := service.CreateSubject(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, path, err := service.SubjectPath(ctx, subject.ID)
	if err != nil {
		t.Fatal(err)
	}
	if norns != nil {
		if err = os.MkdirAll(filepath.Join(path, ".norns"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if norns != nil {
		if err := os.WriteFile(filepath.Join(path, ".norns", "norns.sqlite"), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return New(Config{Service: service, Token: "secret", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}), subject, path
}

func authenticatedRequest(t *testing.T, server *Server, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	r.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, r)
	return w
}

func TestNornsHTTPViewFormatsAndEntityDetails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	executable := filepath.Join(t.TempDir(), "norns")
	script := `#!/bin/sh
if [ "$1" = version ]; then printf '{"protocol_version":%d}\n'; exit 0; fi
printf '%s\n' "$@" > last-command
if [ "$1" = view ]; then
  kind=$2
  format=yaml
  for arg in "$@"; do
    case "$arg" in --json) format=json;; --md) format=md;; esac
  done
  mkdir -p .norns/view
  case "$format" in
    yaml) printf 'task:\n  id: task_test\ngraph:\n  cognitions: []\n  explorations: []\n' > ".norns/view/$kind.yaml";;
    json) printf '{"task":{"id":"task_test"},"graph":{"cognitions":[],"explorations":[]}}\n' > ".norns/view/$kind.json";;
    md) printf '# %s View\n\n## Graph\n\nMarkdown body\n' "$kind" > ".norns/view/$kind.md";;
  esac
  printf '{"ok":true,"data":{"path":".norns/view/%s.%s","format":"%s","note":"file reference"}}\n' "$kind" "$format" "$format"
elif [ "$1" = cognition ] || [ "$1" = exploration ]; then
  case "$4" in
    missing) printf '{"ok":false,"error":{"code":"COGNITION_NOT_FOUND","message":"not found"}}\n' >&2; exit 1;;
    invalid) printf '{"ok":false,"error":{"code":"INVALID_ARGUMENT","message":"invalid ID"}}\n' >&2; exit 1;;
  esac
  if [ "$1" = cognition ]; then
    printf '{"ok":true,"data":{"id":"cog_1","content":"finding","meta":{"type":"theorem"}}}\n'
  else
    printf '{"ok":true,"data":{"id":"exp_1","sources":["cog_1"],"hints":[],"explorer_id":"agent-42"}}\n'
  fi
else
  exit 1
fi
`
	if err := os.WriteFile(executable, []byte(strings.Replace(script, "%d", strconv.Itoa(version.NornsProtocol), 1)), 0700); err != nil {
		t.Fatal(err)
	}
	adapter := &nornsadapter.Adapter{Executable: executable}
	server, subject, path := newArchiveTestServer(t, adapter)
	base := "/api/subjects/" + subject.ID + "/norns/"
	for _, kind := range []string{"status", "trace"} {
		for _, format := range []string{"", "yaml", "json", "md"} {
			t.Run(kind+"/"+format, func(t *testing.T) {
				query := "?format=" + format
				if kind == "trace" {
					query += "&exploration_id=exp_1"
				} else {
					query += "&task_id=task_test"
				}
				response := authenticatedRequest(t, server, http.MethodGet, base+"views/"+kind+query)
				f, _ := nornsadapter.ParseViewFormat(format)
				if response.Code != http.StatusOK || response.Header().Get("Content-Type") != f.ContentType() || response.Header().Get("X-Norns-View-Filename") != kind+"."+string(f) {
					t.Fatalf("view response=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
				}
				published, err := os.ReadFile(filepath.Join(path, ".norns", "view", kind+"."+string(f)))
				if err != nil || !bytes.Equal(response.Body.Bytes(), published) {
					t.Fatalf("view changed: %v", err)
				}
				args, err := os.ReadFile(filepath.Join(path, "last-command"))
				if err != nil || !bytes.Contains(args, []byte("--output-reference")) {
					t.Fatalf("missing reference flag: %s, %v", args, err)
				}
				if kind == "trace" && !bytes.Contains(args, []byte("--exploration\nexp_1")) {
					t.Fatalf("lost exploration selection: %s", args)
				}
			})
		}
	}
	if open, err := adapter.HasOpenTask(context.Background(), path); err != nil || !open {
		t.Fatalf("HasOpenTask: %v, %v", open, err)
	}
	for route, field := range map[string]string{"cognitions/cog_1": "meta", "explorations/exp_1": "explorer_id"} {
		response := authenticatedRequest(t, server, http.MethodGet, base+route)
		var entity map[string]json.RawMessage
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &entity) != nil || entity[field] == nil || entity["ok"] != nil {
			t.Fatalf("entity response=%d %s", response.Code, response.Body.String())
		}
	}
	for route, code := range map[string]int{
		"views/status?format=xml":            400,
		"views/status?format=json&format=md": 400,
		"views/trace?format=md":              400,
		"cognitions/missing":                 404,
		"cognitions/invalid":                 400,
	} {
		response := authenticatedRequest(t, server, http.MethodGet, base+route)
		if response.Code != code || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatalf("%s: response=%d %s", route, response.Code, response.Body.String())
		}
	}
}

func TestArchiveHTTPDownloadAndIdleRequirements(t *testing.T) {
	server, subject, path := newArchiveTestServer(t, nil)
	if err := os.WriteFile(filepath.Join(path, "result.txt"), []byte("result"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/api/archive?scope=subject&subject_id=" + subject.ID, "/api/archive?scope=subject"} {
		response := authenticatedRequest(t, server, http.MethodGet, route)
		_, params, err := mime.ParseMediaType(response.Header().Get("Content-Disposition"))
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/gzip" || err != nil || !strings.HasSuffix(params["filename"], ".tar.gz") || response.Header().Get("Content-Length") != strconv.Itoa(response.Body.Len()) {
			t.Fatalf("archive response=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
		}
		gz, err := gzip.NewReader(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		tr := tar.NewReader(gz)
		foundResult, foundDatabase := false, false
		for {
			header, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasSuffix(header.Name, "/result.txt") {
				foundResult = true
			}
			if strings.HasSuffix(header.Name, "/.nornsd/service/nornsd.sqlite") {
				foundDatabase = true
			}
		}
		_ = gz.Close()
		if !foundResult || (route == "/api/archive?scope=subject" && !foundDatabase) {
			t.Fatalf("archive missing files: result=%v database=%v", foundResult, foundDatabase)
		}
	}
	if _, _, err := server.service.Submit(context.Background(), subject.ID, "queued work", false); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/api/archive?scope=subject&subject_id=" + subject.ID, "/api/archive?scope=subject"} {
		response := authenticatedRequest(t, server, http.MethodGet, route)
		if response.Code != http.StatusConflict {
			t.Fatalf("busy archive=%d %s", response.Code, response.Body.String())
		}
	}
	response := authenticatedRequest(t, server, http.MethodGet, "/api/subjects/missing/archive")
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing subject=%d %s", response.Code, response.Body.String())
	}
}

func TestArchiveRequestsUseIndependentTimeout(t *testing.T) {
	server := New(Config{Token: "secret", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	server.mux = http.NewServeMux()
	for _, route := range []string{"/api/archive", "/api/health"} {
		route := route
		server.mux.HandleFunc("GET "+route, func(w http.ResponseWriter, r *http.Request) {
			deadline, ok := r.Context().Deadline()
			expected := 10 * time.Minute
			if route == "/api/health" {
				expected = version.RequestTimeout
			}
			if remaining := time.Until(deadline); !ok || remaining > expected || remaining < expected-time.Second {
				t.Errorf("%s deadline: %s, expected %s", route, remaining, expected)
			}
			w.WriteHeader(http.StatusOK)
		})
		requestPath := strings.ReplaceAll(route, "{subject_id}", "test-subject")
		response := authenticatedRequest(t, server, http.MethodGet, requestPath)
		if response.Code != http.StatusOK {
			t.Fatal(response.Code)
		}
	}
	server.archiveTimeout = time.Millisecond
	server.mux = http.NewServeMux()
	server.mux.HandleFunc("GET /api/archive", func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		fail(server, w, r, r.Context().Err())
	})
	response := authenticatedRequest(t, server, http.MethodGet, "/api/archive?scope=subject")
	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("archive timeout: %d %s", response.Code, response.Body.String())
	}
}

// A ResponseWriter with a blocked download models a slow peer. It implements
// write deadlines so cancellation must unblock the actual response write.
type slowArchiveWriter struct {
	header  http.Header
	status  int
	expired chan struct{}
	once    sync.Once
	mu      sync.Mutex
	timer   *time.Timer
	onStart func()
}

func (w *slowArchiveWriter) Header() http.Header { return w.header }
func (w *slowArchiveWriter) WriteHeader(status int) {
	w.status = status
	if w.onStart != nil {
		w.onStart()
	}
}
func (w *slowArchiveWriter) Write(body []byte) (int, error) {
	<-w.expired
	return 0, os.ErrDeadlineExceeded
}
func (w *slowArchiveWriter) SetWriteDeadline(deadline time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	if !deadline.IsZero() {
		w.timer = time.AfterFunc(time.Until(deadline), func() { w.once.Do(func() { close(w.expired) }) })
	}
	return nil
}

func TestArchiveSlowDownloadReleasesAdmissionAndCleansTemporaryFile(t *testing.T) {
	server, _, _ := newArchiveTestServer(t, nil)
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	t.Setenv("TMP", temp)
	t.Setenv("TEMP", temp)
	server.archiveTimeout = 300 * time.Millisecond
	var mutationErr error
	w := &slowArchiveWriter{header: make(http.Header), expired: make(chan struct{}), onStart: func() {
		// Root admission must be released before any download bytes are sent.
		_, mutationErr = server.service.CreateSubject(context.Background(), nil, nil)
	}}
	r := httptest.NewRequest(http.MethodGet, "/api/archive?scope=subject", nil)
	r.Header.Set("Authorization", "Bearer secret")
	finished := make(chan struct{})
	go func() { server.ServeHTTP(w, r); close(finished) }()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		w.once.Do(func() { close(w.expired) })
		t.Fatal("download write did not observe the archive deadline")
	}
	if w.status != http.StatusOK || mutationErr != nil {
		t.Fatalf("download=%d, mutation after compression=%v", w.status, mutationErr)
	}
	entries, err := os.ReadDir(temp)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary archive not cleaned: %v, %v", entries, err)
	}
}

func TestIntegrationNornsHTTPWorkflow(t *testing.T) {
	if os.Getenv("NORNS_RUN_BINARY_INTEGRATION") != "1" {
		t.Skip("set NORNS_RUN_BINARY_INTEGRATION=1 to build and exercise the real norns CLI")
	}
	module, err := filepath.Abs("../../../norns")
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "norns")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	build := exec.Command("go", "build", "-o", executable, "./cmd/norns")
	build.Dir = module
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build norns: %v\n%s", err, output)
	}
	server, subject, path := newArchiveTestServer(t, &nornsadapter.Adapter{Executable: executable})
	run := func(args ...string) map[string]json.RawMessage {
		t.Helper()
		command := exec.Command(executable, args...)
		command.Dir = path
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("norns %v: %v\n%s", args, err, output)
		}
		var response struct {
			OK   bool                       `json:"ok"`
			Data map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(output, &response); err != nil || !response.OK {
			t.Fatalf("norns response %s: %v", output, err)
		}
		return response.Data
	}
	run("workspace", "init")
	run("task", "open", "--purpose", "Verify HTTP integrations", "--info", "Subject information", "--concurrency", "2")
	run("cognition", "add", "--cognition-title", "User input", "--cognition-content", "A global hint", "--origin", "user", "--meta", `{"scope":"test"}`)
	run("exploration", "add", "--exploration-title", "Inspect", "--exploration-content", "Verify a finding", "--source-cognitions", "cog_1")
	run("exploration", "assign", "--exploration", "exp_1", "--explorer", "test-explorer", "--output", "output.txt")
	run("cognition", "add", "--cognition-title", "User exploration input", "--cognition-content", "An exploration hint", "--origin", "user", "--exploration", "exp_1")
	run("cognition", "add", "--cognition-title", "Finding", "--cognition-content", strings.Repeat("Verified finding. ", 500), "--origin", "exploration", "--exploration", "exp_1", "--meta", `{"type":"theorem"}`)
	base := "/api/subjects/" + subject.ID + "/norns/"
	for _, kind := range []string{"status", "trace"} {
		for _, format := range []string{"", "yaml", "json", "md"} {
			query := "?format=" + format
			if kind == "trace" {
				query += "&exploration_id=exp_1"
			}
			response := authenticatedRequest(t, server, http.MethodGet, base+"views/"+kind+query)
			f, _ := nornsadapter.ParseViewFormat(format)
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != f.ContentType() || !bytes.Contains(response.Body.Bytes(), []byte("cog_4")) {
				t.Fatalf("%s %s: %d %s", kind, format, response.Code, response.Body.String())
			}
			if response.Body.Len() <= 6144 {
				t.Fatalf("large View fixture is too small: %d", response.Body.Len())
			}
			if !strings.HasSuffix(response.Header().Get("X-Norns-View-Filename"), "."+string(f)) {
				t.Fatal(response.Header())
			}
			if f == nornsadapter.Markdown && !bytes.Contains(response.Body.Bytes(), []byte("## Graph")) {
				t.Fatal("Markdown graph missing")
			}
		}
	}
	for route, marker := range map[string]string{"cognitions/cog_4": "theorem", "explorations/exp_1": "test-explorer"} {
		response := authenticatedRequest(t, server, http.MethodGet, base+route)
		if response.Code != http.StatusOK || !json.Valid(response.Body.Bytes()) || !bytes.Contains(response.Body.Bytes(), []byte(marker)) {
			t.Fatalf("entity: %d %s", response.Code, response.Body.String())
		}
	}
	for route, code := range map[string]int{"cognitions/cog_999": 404, "explorations/exp_999": 404, "cognitions/cog_01": 400} {
		response := authenticatedRequest(t, server, http.MethodGet, base+route)
		if response.Code != code {
			t.Fatalf("%s: %d %s", route, response.Code, response.Body.String())
		}
	}
	// A standalone open Norns Task does not make the nornsd Subject non-idle.
	for _, route := range []string{"/api/archive?scope=subject&subject_id=" + subject.ID, "/api/archive?scope=subject"} {
		response := authenticatedRequest(t, server, http.MethodGet, route)
		if response.Code != http.StatusOK {
			t.Fatalf("archive: %d %s", response.Code, response.Body.String())
		}
		gz, err := gzip.NewReader(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		tr := tar.NewReader(gz)
		found := false
		for {
			header, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasSuffix(header.Name, "/.norns/norns.sqlite") {
				found = true
			}
		}
		_ = gz.Close()
		if !found {
			t.Fatal("archive missing Norns database")
		}
	}
}

func TestNornsRequestsCheckWorkspaceBeforeServiceSupport(t *testing.T) {
	server, subject, path := newArchiveTestServer(t, nil)
	routes := []string{"views/status?format=md", "views/trace?exploration_id=exp_1&format=md", "views/visual?lang=en", "cognitions/cog_1", "explorations/exp_1"}
	check := func(code string, status int) {
		t.Helper()
		for _, route := range routes {
			response := authenticatedRequest(t, server, http.MethodGet, "/api/subjects/"+subject.ID+"/norns/"+route)
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != status || body.Error.Code != code {
				t.Fatalf("%s: got %d %s, expected %s", route, response.Code, response.Body.String(), code)
			}
		}
	}
	check("WORKSPACE_NOT_INITIALIZED", http.StatusConflict)
	if err := os.MkdirAll(filepath.Join(path, ".norns"), 0700); err != nil {
		t.Fatal(err)
	}
	check("WORKSPACE_NOT_INITIALIZED", http.StatusConflict)
	// A directory named norns.sqlite is not a database file.
	database := filepath.Join(path, ".norns", "norns.sqlite")
	if err := os.Mkdir(database, 0700); err != nil {
		t.Fatal(err)
	}
	check("WORKSPACE_NOT_INITIALIZED", http.StatusConflict)
	if err := os.Remove(database); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(database, nil, 0600); err != nil {
		t.Fatal(err)
	}
	check("NORNS_UNAVAILABLE", http.StatusServiceUnavailable)
	server.service.Norns = &nornsadapter.Adapter{Executable: filepath.Join(t.TempDir(), "missing-norns")}
	check("NORNS_UNAVAILABLE", http.StatusServiceUnavailable)
}
