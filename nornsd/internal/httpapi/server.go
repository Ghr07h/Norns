package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/Ghr07h/norns/nornsd/internal/app"
	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/eventlog"
	"github.com/Ghr07h/norns/nornsd/internal/version"
)

type Config struct {
	Service     *app.Service
	Token       string
	Concurrency int
	Logger      *slog.Logger
}
type Server struct {
	service     *app.Service
	tokenHash   [32]byte
	concurrency int
	logger      *slog.Logger
	mux         *http.ServeMux
	draining    atomic.Bool
}

func New(cfg Config) *Server {
	s := &Server{service: cfg.Service, tokenHash: sha256.Sum256([]byte(cfg.Token)), concurrency: cfg.Concurrency, logger: cfg.Logger, mux: http.NewServeMux()}
	if s.logger == nil {
		s.logger = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	type route struct {
		method, path string
		query        map[string]bool
		handler      http.HandlerFunc
	}
	routes := []route{
		{"GET", "/api/v1", nil, s.serviceInfo},
		{"GET", "/api/v1/health", nil, s.health},
		{"POST", "/api/v1/subjects", nil, s.createSubject},
		{"GET", "/api/v1/subjects", map[string]bool{"limit": false, "cursor": false, "state": true}, s.listSubjects},
		{"GET", "/api/v1/subjects/{subject_id}", nil, s.getSubject},
		{"POST", "/api/v1/subjects/{subject_id}/runtime", nil, s.submit},
		{"GET", "/api/v1/subjects/{subject_id}/runtime", nil, s.getCurrentRuntime},
		{"POST", "/api/v1/subjects/{subject_id}/runtime/stop", nil, s.stop},
		{"GET", "/api/v1/queue", map[string]bool{"limit": false, "cursor": false}, s.listQueue},
		{"GET", "/api/v1/subjects/{subject_id}/events", map[string]bool{"limit": false, "cursor": false, "after": false}, s.listEvents},
		{"GET", "/api/v1/subjects/{subject_id}/events/updates", map[string]bool{"cursor": false}, s.eventUpdates},
		{"GET", "/api/v1/subjects/{subject_id}/runtimes", map[string]bool{"limit": false, "cursor": false}, s.listRuntimes},
		{"GET", "/api/v1/subjects/{subject_id}/runtimes/{runtime_id}", nil, s.getRuntime},
		{"GET", "/api/v1/subjects/{subject_id}/norns/views/status", map[string]bool{"task_id": false}, s.statusView},
		{"GET", "/api/v1/subjects/{subject_id}/norns/views/trace", map[string]bool{"exploration_id": false}, s.traceView},
		{"GET", "/api/v1/subjects/{subject_id}/norns/views/visual", map[string]bool{"lang": false}, s.visualView},
	}
	methods := map[string][]string{}
	for _, item := range routes {
		item := item
		methods[item.path] = append(methods[item.path], item.method)
		s.mux.HandleFunc(item.method+" "+item.path, func(w http.ResponseWriter, r *http.Request) {
			if err := validateQuery(r, item.query); err != nil {
				fail(s, w, r, err)
				return
			}
			item.handler(w, r)
		})
	}
	for path, allowed := range methods {
		allowed := strings.Join(allowed, ", ")
		s.mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Allow", allowed)
			fail(s, w, r, domain.NewError("METHOD_NOT_ALLOWED", "method is not allowed", http.StatusMethodNotAllowed))
		})
	}
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fail(s, w, r, domain.NewError("NOT_FOUND", "route was not found", http.StatusNotFound))
	})
}

func validateQuery(r *http.Request, allowed map[string]bool) error {
	for key, values := range r.URL.Query() {
		repeatable, ok := allowed[key]
		if !ok {
			return domain.NewError("INVALID_REQUEST", "unknown query parameter: "+key, http.StatusBadRequest)
		}
		if !repeatable && len(values) != 1 {
			return domain.NewError("INVALID_REQUEST", "query parameter must be specified once: "+key, http.StatusBadRequest)
		}
	}
	return nil
}

type statusWriter struct {
	http.ResponseWriter
	status, bytes int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	requestID := "req_" + uuid.NewString()
	w.Header().Set("X-Request-ID", requestID)
	sw := &statusWriter{ResponseWriter: w}
	defer func() {
		if recovered := recover(); recovered != nil {
			s.writeError(sw, requestID, domain.NewError("INTERNAL_ERROR", "internal error", http.StatusInternalServerError))
		}
		s.logger.Info("http request", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "status", sw.status, "duration_ms", time.Since(started).Milliseconds(), "response_bytes", sw.bytes)
	}()
	if !s.authorized(r) {
		s.writeError(sw, requestID, domain.NewError("UNAUTHORIZED", "authentication required", http.StatusUnauthorized))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), version.RequestTimeout)
	defer cancel()
	s.mux.ServeHTTP(sw, r.WithContext(context.WithValue(ctx, requestIDKey{}, requestID)))
}

type requestIDKey struct{}

func (s *Server) authorized(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || strings.Contains(strings.TrimPrefix(header, "Bearer "), " ") {
		return false
	}
	candidate := sha256.Sum256([]byte(strings.TrimPrefix(header, "Bearer ")))
	return subtle.ConstantTimeCompare(candidate[:], s.tokenHash[:]) == 1
}
func (s *Server) SetDraining() { s.draining.Store(true) }

func (s *Server) writeError(w http.ResponseWriter, requestID string, err error) {
	de := domain.AsError(err)
	if errors.Is(err, context.DeadlineExceeded) {
		de = domain.NewError("DEADLINE_EXCEEDED", "request deadline exceeded", http.StatusGatewayTimeout)
	}
	details := de.Details
	if details == nil {
		details = map[string]any{}
	}
	writeJSON(w, de.HTTPStatus, map[string]any{"error": map[string]any{"code": de.Code, "message": de.Message, "request_id": requestID, "details": details}})
}
func fail(s *Server, w http.ResponseWriter, r *http.Request, err error) {
	requestID, _ := r.Context().Value(requestIDKey{}).(string)
	s.writeError(w, requestID, err)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) error {
	if media := r.Header.Get("Content-Type"); !strings.HasPrefix(strings.ToLower(media), "application/json") {
		return domain.NewError("INVALID_REQUEST", "Content-Type must be application/json", http.StatusBadRequest)
	}
	r.Body = http.MaxBytesReader(w, r.Body, version.MaxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return domain.NewError("REQUEST_TOO_LARGE", "request body exceeds 2 MiB", http.StatusRequestEntityTooLarge)
		}
		return domain.Wrap("INVALID_REQUEST", "invalid JSON request", http.StatusBadRequest, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return domain.NewError("INVALID_REQUEST", "request body must contain one JSON value", http.StatusBadRequest)
	}
	return nil
}
func parseLimit(r *http.Request, max, def int) (int, error) {
	text := r.URL.Query().Get("limit")
	if text == "" {
		return def, nil
	}
	value, err := strconv.Atoi(text)
	if err != nil || value < 1 || value > max {
		return 0, domain.NewError("INVALID_REQUEST", fmt.Sprintf("limit must be between 1 and %d", max), http.StatusBadRequest)
	}
	return value, nil
}

func (s *Server) serviceInfo(w http.ResponseWriter, r *http.Request) {
	using, queuing, err := s.service.Capacity(r.Context())
	if err != nil {
		fail(s, w, r, err)
		return
	}
	caps := []string{"subjects", "runtime_submission", "runtime_queue", "readable_log_polling", "runtime_stop", "runtime_history"}
	var nornsVersion any
	if s.service.Profile.Has("norns") {
		caps = append(caps, "norns_views")
		nornsVersion = version.NornsProtocol
	}
	writeJSON(w, http.StatusOK, map[string]any{"service": "nornsd", "server_id": s.service.RootID(), "version": version.ProductVersion, "api_version": version.APIVersion, "runtime_profile": s.service.Profile, "protocol_versions": map[string]any{"norns": nornsVersion, "agent_config": version.AgentConfigVersion}, "capacity": map[string]any{"concurrency": s.concurrency, "using": using, "queuing": queuing}, "capabilities": caps})
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if s.draining.Load() {
		fail(s, w, r, domain.NewError("SERVICE_UNAVAILABLE", "server is draining", http.StatusServiceUnavailable))
		return
	}
	if err := s.service.Health(r.Context()); err != nil {
		fail(s, w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) createSubject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    *string `json:"name"`
		Runtime *struct {
			Instruction string `json:"instruction"`
		} `json:"runtime"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		fail(s, w, r, err)
		return
	}
	var instruction *string
	if body.Runtime != nil {
		instruction = &body.Runtime.Instruction
	}
	subject, err := s.service.CreateSubject(r.Context(), body.Name, instruction)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/subjects/"+subject.ID)
	writeJSON(w, http.StatusCreated, map[string]string{"subject_id": subject.ID})
}

func (s *Server) getSubject(w http.ResponseWriter, r *http.Request) {
	subject, err := s.service.GetSubjectBase(r.Context(), r.PathValue("subject_id"))
	if err != nil {
		fail(s, w, r, err)
		return
	}
	workspacePath, err := s.service.WorkspacePath(subject)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, subjectDTO(subject, workspacePath))
}

func (s *Server) listSubjects(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r, version.MaxListLimit, version.DefaultListLimit)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	rawStates := r.URL.Query()["state"]
	states := make([]domain.SubjectState, 0, len(rawStates))
	for _, value := range rawStates {
		state := domain.SubjectState(value)
		if state != domain.SubjectIdle && state != domain.SubjectQueuing && state != domain.SubjectUsing {
			fail(s, w, r, domain.NewError("INVALID_REQUEST", "invalid subject state", http.StatusBadRequest))
			return
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool { return states[i] < states[j] })
	filters := strings.Join(subjectStates(states), ",")
	afterTime, afterID := "", ""
	if token := r.URL.Query().Get("cursor"); token != "" {
		sortValues, e := decodeCursor(s.service.Store.CursorKey(), token, "subjects", filters)
		if e != nil || len(sortValues) != 2 {
			fail(s, w, r, domain.NewError("INVALID_CURSOR", "invalid cursor", http.StatusBadRequest))
			return
		}
		afterTime, afterID = sortValues[0], sortValues[1]
	}
	subjects, err := s.service.ListSubjectSummaries(r.Context(), states, limit+1, afterTime, afterID)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	var next any
	if len(subjects) > limit {
		last := subjects[limit-1]
		next, _ = encodeCursor(s.service.Store.CursorKey(), "subjects", filters, []string{last.CreatedAt.Format(time.RFC3339Nano), last.ID})
		subjects = subjects[:limit]
	}
	items := make([]any, 0, len(subjects))
	for _, subject := range subjects {
		items = append(items, map[string]any{"id": subject.ID, "name": subject.Name, "state": subject.State, "created_at": subject.CreatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"subjects": items, "next_cursor": next})
}

func subjectStates(states []domain.SubjectState) []string {
	out := make([]string, len(states))
	for i, v := range states {
		out[i] = string(v)
	}
	return out
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Instruction   string `json:"instruction"`
		ResumeSession bool   `json:"resume_session"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		fail(s, w, r, err)
		return
	}
	runtime, position, err := s.service.Submit(r.Context(), r.PathValue("subject_id"), body.Instruction, body.ResumeSession)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "runtime": map[string]any{"id": runtime.ID, "state": runtime.State, "queue_position": position, "instruction_count": len(runtime.Instructions)}})
}
func (s *Server) getCurrentRuntime(w http.ResponseWriter, r *http.Request) {
	current, err := s.service.GetCurrentRuntime(r.Context(), r.PathValue("subject_id"))
	if err != nil {
		fail(s, w, r, err)
		return
	}
	if current.Runtime == nil {
		writeJSON(w, http.StatusOK, map[string]any{"runtime": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runtime": runtimeDTO(*current.Runtime, current.QueuePosition)})
}
func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		fail(s, w, r, err)
		return
	}
	runtime, err := s.service.Stop(r.Context(), r.PathValue("subject_id"), body.Mode)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/subjects/"+runtime.SubjectID+"/runtimes/"+runtime.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "subject_id": runtime.SubjectID, "runtime_id": runtime.ID, "runtime_state": domain.RuntimeCancelled})
}

func (s *Server) listQueue(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r, version.MaxListLimit, version.DefaultListLimit)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	after := int64(0)
	if token := r.URL.Query().Get("cursor"); token != "" {
		values, e := decodeCursor(s.service.Store.CursorKey(), token, "queue", "")
		if e != nil || len(values) != 1 {
			fail(s, w, r, domain.NewError("INVALID_CURSOR", "invalid cursor", http.StatusBadRequest))
			return
		}
		after, e = strconv.ParseInt(values[0], 10, 64)
		if e != nil {
			fail(s, w, r, domain.NewError("INVALID_CURSOR", "invalid cursor", http.StatusBadRequest))
			return
		}
	}
	entries, err := s.service.ListQueueEntries(r.Context(), limit+1, after)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	var next any
	if len(entries) > limit {
		last := entries[limit-1]
		next, _ = encodeCursor(s.service.Store.CursorKey(), "queue", "", []string{strconv.FormatInt(last.Sequence, 10)})
		entries = entries[:limit]
	}
	items := make([]any, 0, len(entries))
	for _, entry := range entries {
		items = append(items, map[string]any{"position": entry.Position, "runtime_id": entry.RuntimeID, "subject_id": entry.SubjectID, "subject_name": entry.SubjectName, "instruction_count": entry.InstructionCount, "queued_at": entry.QueuedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"runtimes": items, "next_cursor": next})
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r, version.MaxEventLimit, version.DefaultEventLimit)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	query := r.URL.Query()
	if query.Has("after") && query.Has("cursor") {
		fail(s, w, r, domain.NewError("INVALID_REQUEST", "after and cursor are mutually exclusive", http.StatusBadRequest))
		return
	}
	subjectID := r.PathValue("subject_id")
	mode := "latest"
	var cursor eventlog.ReadableCursor
	if query.Has("cursor") {
		mode = "before"
		cursor, err = s.decodeEventCursor(query.Get("cursor"), subjectID, mode)
	} else if query.Has("after") {
		mode = "after"
		cursor, err = s.decodeEventCursor(query.Get("after"), subjectID, mode)
	}
	if err != nil {
		fail(s, w, r, domain.NewError("INVALID_CURSOR", "invalid event cursor", http.StatusBadRequest))
		return
	}
	page, err := s.service.ReadableEvents(r.Context(), subjectID, mode, cursor, limit)
	if err != nil {
		if errors.Is(err, eventlog.ErrInvalidReadableCursor) {
			err = domain.NewError("INVALID_CURSOR", "event cursor no longer matches the readable log", http.StatusBadRequest)
		}
		fail(s, w, r, err)
		return
	}
	var syncCursor any
	if mode != "before" {
		syncCursor, err = s.encodeEventCursor(subjectID, "after", page.Cursor)
		if err != nil {
			fail(s, w, r, err)
			return
		}
	}
	var next any
	if page.Older != nil {
		next, err = s.encodeEventCursor(subjectID, "before", *page.Older)
		if err != nil {
			fail(s, w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": page.Events, "cursor": syncCursor, "next_cursor": next, "has_more": page.HasMore})
}

func (s *Server) eventUpdates(w http.ResponseWriter, r *http.Request) {
	subjectID := r.PathValue("subject_id")
	cursor, err := s.decodeEventCursor(r.URL.Query().Get("cursor"), subjectID, "after")
	if err != nil {
		fail(s, w, r, domain.NewError("INVALID_CURSOR", "invalid event cursor", http.StatusBadRequest))
		return
	}
	updated, err := s.service.ReadableEventsUpdated(r.Context(), subjectID, cursor)
	if err != nil {
		if errors.Is(err, eventlog.ErrInvalidReadableCursor) {
			err = domain.NewError("INVALID_CURSOR", "event cursor no longer matches the readable log", http.StatusBadRequest)
		}
		fail(s, w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"has_updates": updated})
}

func (s *Server) encodeEventCursor(subjectID, mode string, cursor eventlog.ReadableCursor) (string, error) {
	return encodeCursor(s.service.Store.CursorKey(), "events", subjectID, []string{mode, strconv.FormatInt(cursor.Offset, 10), strconv.FormatUint(cursor.Seq, 10), cursor.Hash})
}

func (s *Server) decodeEventCursor(token, subjectID, mode string) (eventlog.ReadableCursor, error) {
	values, err := decodeCursor(s.service.Store.CursorKey(), token, "events", subjectID)
	if err != nil || len(values) != 4 || values[0] != mode {
		return eventlog.ReadableCursor{}, errors.New("invalid event cursor")
	}
	offset, err := strconv.ParseInt(values[1], 10, 64)
	if err != nil || offset < 0 {
		return eventlog.ReadableCursor{}, errors.New("invalid event cursor")
	}
	seq, err := strconv.ParseUint(values[2], 10, 64)
	if err != nil {
		return eventlog.ReadableCursor{}, errors.New("invalid event cursor")
	}
	if seq == 0 && (offset != 0 || values[3] != "") || seq != 0 && (offset == 0 || !strings.HasPrefix(values[3], "sha256:")) {
		return eventlog.ReadableCursor{}, errors.New("invalid event cursor")
	}
	return eventlog.ReadableCursor{Offset: offset, Seq: seq, Hash: values[3]}, nil
}

func (s *Server) listRuntimes(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r, version.MaxListLimit, version.DefaultListLimit)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	route := "subject-runtimes:" + r.PathValue("subject_id")
	afterTime, afterID := "", ""
	if token := r.URL.Query().Get("cursor"); token != "" {
		values, e := decodeCursor(s.service.Store.CursorKey(), token, route, "")
		if e != nil || len(values) != 2 {
			fail(s, w, r, domain.NewError("INVALID_CURSOR", "invalid cursor", http.StatusBadRequest))
			return
		}
		afterTime, afterID = values[0], values[1]
	}
	runtimes, err := s.service.ListRuntimeSummaries(r.Context(), r.PathValue("subject_id"), limit+1, afterTime, afterID)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	var next any
	if len(runtimes) > limit {
		last := runtimes[limit-1]
		next, _ = encodeCursor(s.service.Store.CursorKey(), route, "", []string{last.CreatedAt.Format(time.RFC3339Nano), last.ID})
		runtimes = runtimes[:limit]
	}
	items := make([]any, 0, len(runtimes))
	totalCount := 0
	for _, runtime := range runtimes {
		totalCount = runtime.TotalCount
		items = append(items, map[string]any{"id": runtime.ID, "state": runtime.State, "current": runtime.Current, "created_at": runtime.CreatedAt, "started_at": runtime.StartedAt, "ended_at": runtime.EndedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"runtimes": items, "next_cursor": next, "total_count": totalCount})
}
func (s *Server) getRuntime(w http.ResponseWriter, r *http.Request) {
	runtime, err := s.service.GetRuntime(r.Context(), r.PathValue("subject_id"), r.PathValue("runtime_id"))
	if err != nil {
		fail(s, w, r, err)
		return
	}
	subject, err := s.service.GetSubjectBase(r.Context(), runtime.SubjectID)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	artifacts, err := s.service.RuntimeArtifacts(subject, runtime)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, runtimeDetail(runtime, subject, artifacts))
}

func (s *Server) statusView(w http.ResponseWriter, r *http.Request) {
	var taskID *string
	if value := r.URL.Query().Get("task_id"); value != "" {
		taskID = &value
	}
	b, filename, err := s.service.StatusView(r.Context(), r.PathValue("subject_id"), taskID)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	setViewFilename(w, filename)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
func (s *Server) traceView(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("exploration_id")
	if id == "" {
		fail(s, w, r, domain.NewError("INVALID_REQUEST", "exploration_id is required", http.StatusBadRequest))
		return
	}
	b, filename, err := s.service.TraceView(r.Context(), r.PathValue("subject_id"), id)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	setViewFilename(w, filename)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
func (s *Server) visualView(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	if lang != "" && lang != "zh" && lang != "en" {
		fail(s, w, r, domain.NewError("INVALID_REQUEST", "lang must be zh or en", http.StatusBadRequest))
		return
	}
	b, filename, err := s.service.VisualView(r.Context(), r.PathValue("subject_id"), lang)
	if err != nil {
		fail(s, w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	setViewFilename(w, filename)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func setViewFilename(w http.ResponseWriter, filename string) {
	if filename != "" {
		w.Header().Set("X-Norns-View-Filename", filename)
	}
}

func subjectDTO(subject domain.Subject, workspacePath *string) map[string]any {
	return map[string]any{"id": subject.ID, "name": subject.Name, "state": subject.State, "runtime_profile": subject.Profile, "workspace": map[string]any{"materialized": subject.WorkspaceState == domain.WorkspaceMaterialized, "directory_name": subject.DirectoryName, "path": workspacePath}, "last_event_seq": subject.LastEventSeq, "created_at": subject.CreatedAt, "updated_at": subject.UpdatedAt}
}
func runtimeDTO(runtime domain.Runtime, position *int) map[string]any {
	return map[string]any{"id": runtime.ID, "subject_id": runtime.SubjectID, "state": runtime.State, "queue_position": position, "instructions": runtime.Instructions, "created_at": runtime.CreatedAt, "queued_at": runtime.QueuedAt, "started_at": runtime.StartedAt, "ended_at": runtime.EndedAt, "resources": rawValue(runtime.ResourceJSON), "closing": runtime.Closing, "close_deadline": runtime.CloseDeadline, "state_changed_at": runtime.StateChangedAt}
}
func runtimeDetail(runtime domain.Runtime, subject domain.Subject, artifacts app.RuntimeArtifacts) map[string]any {
	var runtimeErr any
	if runtime.ErrorCode != nil {
		runtimeErr = map[string]any{"code": runtime.ErrorCode, "details": rawValue(runtime.ErrorJSON)}
	}
	return map[string]any{"id": runtime.ID, "subject_id": runtime.SubjectID, "state": runtime.State, "agent": subject.Profile.Agent, "session_id": artifacts.SessionID, "queued_at": runtime.QueuedAt, "started_at": runtime.StartedAt, "ended_at": runtime.EndedAt, "end_reason": runtime.EndReason, "settings_slot": runtime.SettingsSlot, "config_hash": runtime.ConfigHash, "process": rawValue(runtime.ProcessJSON), "usage": rawValue(runtime.UsageJSON), "resources": rawValue(runtime.ResourceJSON), "error": runtimeErr, "artifacts": artifacts}
}
func rawValue(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return value
}
