package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Ghr07h/norns/norns/internal/domain"
	"github.com/Ghr07h/norns/norns/internal/i18n"
	"github.com/Ghr07h/norns/norns/internal/output"
)

type taskProjection struct {
	ID            string  `json:"id" yaml:"id"`
	Purpose       string  `json:"purpose" yaml:"purpose"`
	Outcome       *string `json:"outcome,omitempty" yaml:"outcome,omitempty"`
	OutcomeReason *string `json:"outcome_reason,omitempty" yaml:"outcome_reason,omitempty"`
	ClosedAt      *string `json:"closed_at,omitempty" yaml:"closed_at,omitempty"`
}
type cognitionProjection struct {
	ID                  string  `json:"id" yaml:"id"`
	Content             string  `json:"content" yaml:"content"`
	CreatedAt           string  `json:"created_at" yaml:"created_at"`
	OriginRef           *string `json:"origin_ref,omitempty" yaml:"origin_ref,omitempty"`
	TaskPurposeRelevant *bool   `json:"task_purpose_relevant,omitempty" yaml:"task_purpose_relevant,omitempty"`
}
type globalProjection struct {
	ID        string `json:"id" yaml:"id"`
	Content   string `json:"content" yaml:"content"`
	CreatedAt string `json:"created_at" yaml:"created_at"`
}
type explorationProjection struct {
	ID            string   `json:"id" yaml:"id"`
	Content       string   `json:"content" yaml:"content"`
	RetryOf       *string  `json:"retry_of,omitempty" yaml:"retry_of,omitempty"`
	Status        string   `json:"status" yaml:"status"`
	Outcome       *string  `json:"outcome,omitempty" yaml:"outcome,omitempty"`
	OutcomeReason *string  `json:"outcome_reason,omitempty" yaml:"outcome_reason,omitempty"`
	Sources       []string `json:"source_cognitions" yaml:"source_cognitions"`
	FinishedAt    *string  `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
}
type graphProjection struct {
	Cognitions   []cognitionProjection   `json:"cognitions" yaml:"cognitions"`
	Explorations []explorationProjection `json:"explorations" yaml:"explorations"`
}
type todoProjection struct {
	ID    string `json:"id" yaml:"id"`
	Title string `json:"title" yaml:"title"`
}
type runningProjection struct {
	ID              string `json:"id" yaml:"id"`
	Title           string `json:"title" yaml:"title"`
	DurationSeconds int64  `json:"duration_seconds" yaml:"duration_seconds"`
}
type statusView struct {
	Task             *taskProjection     `json:"task,omitempty" yaml:"task,omitempty"`
	Tasks            *[]taskProjection   `json:"tasks,omitempty" yaml:"tasks,omitempty"`
	Graph            graphProjection     `json:"graph" yaml:"graph"`
	GlobalCognitions []globalProjection  `json:"global_cognitions" yaml:"global_cognitions"`
	Todo             []todoProjection    `json:"todo_explorations" yaml:"todo_explorations"`
	Running          []runningProjection `json:"running_explorations" yaml:"running_explorations"`
	GeneratedAt      string              `json:"generated_at" yaml:"generated_at"`
}

type loaded struct {
	Tasks        []domain.Task
	Cognitions   []domain.Cognition
	Explorations []domain.Exploration
}

func (s *Service) StatusView(ctx context.Context, taskID *string, asJSON bool) ([]byte, string, error) {
	if taskID != nil {
		if err := domain.ValidateID(*taskID, "task"); err != nil {
			return nil, "", err
		}
	}
	var bytes []byte
	var path string
	err := s.shared(ctx, func(ctx context.Context) error {
		var all loaded
		if err := s.Store.Read(ctx, func(tx *sql.Tx) error { var e error; all, e = loadAll(ctx, tx); return e }); err != nil {
			return err
		}
		generated := s.Clock.Now()
		view, err := buildStatus(all, taskID, generated)
		if err != nil {
			return err
		}
		bytes, err = encodeView(view, asJSON)
		if err != nil {
			return err
		}
		path, err = s.publishView(bytes, "status", extension(asJSON))
		return err
	})
	return bytes, path, err
}

func buildStatus(all loaded, requested *string, generated time.Time) (statusView, error) {
	view := statusView{Graph: graphProjection{Cognitions: []cognitionProjection{}, Explorations: []explorationProjection{}}, GlobalCognitions: []globalProjection{}, Todo: []todoProjection{}, Running: []runningProjection{}, GeneratedAt: domain.TimeText(generated)}
	var selected *domain.Task
	if requested != nil {
		for i := range all.Tasks {
			if all.Tasks[i].ID == *requested {
				selected = &all.Tasks[i]
				break
			}
		}
		if selected == nil {
			return view, domain.NewError("TASK_NOT_FOUND", "Task not found")
		}
	} else {
		for i := range all.Tasks {
			if all.Tasks[i].Status == "open" {
				selected = &all.Tasks[i]
				break
			}
		}
	}
	if selected != nil {
		p := projectTask(*selected)
		view.Task = &p
	} else {
		tasks := []taskProjection{}
		for _, t := range all.Tasks {
			tasks = append(tasks, projectTask(t))
		}
		view.Tasks = &tasks
	}
	for _, c := range all.Cognitions {
		if c.TaskID == nil {
			view.GlobalCognitions = append(view.GlobalCognitions, globalProjection{c.ID, c.Content, c.CreatedAt})
			continue
		}
		p := projectCognition(c)
		if c.TaskPurposeRelevant && (selected == nil || *c.TaskID == selected.ID) {
			v := true
			p.TaskPurposeRelevant = &v
		}
		view.Graph.Cognitions = append(view.Graph.Cognitions, p)
	}
	for _, e := range all.Explorations {
		view.Graph.Explorations = append(view.Graph.Explorations, projectExploration(e))
		if selected != nil && e.TaskID == selected.ID {
			if e.Status == "todo" {
				view.Todo = append(view.Todo, todoProjection{e.ID, e.Title})
			}
			if e.Status == "running" {
				assigned, _ := time.Parse(time.RFC3339Nano, value(e.AssignedAt))
				d := int64(generated.Sub(assigned).Seconds())
				if d < 0 {
					d = 0
				}
				view.Running = append(view.Running, runningProjection{e.ID, e.Title, d})
			}
		}
	}
	return view, nil
}

type currentExploration struct {
	ID      string `json:"id" yaml:"id"`
	Title   string `json:"title" yaml:"title"`
	Content string `json:"content" yaml:"content"`
}
type originTask struct {
	Purpose string `json:"purpose" yaml:"purpose"`
	Note    string `json:"note" yaml:"note"`
}
type traceView struct {
	Current          currentExploration `json:"current_exploration" yaml:"current_exploration"`
	OriginTask       originTask         `json:"origin_task" yaml:"origin_task"`
	Related          graphProjection    `json:"related_subgraph" yaml:"related_subgraph"`
	GlobalCognitions []globalProjection `json:"global_cognitions" yaml:"global_cognitions"`
	GeneratedAt      string             `json:"generated_at" yaml:"generated_at"`
}

func (s *Service) TraceView(ctx context.Context, id string, asJSON bool) ([]byte, string, error) {
	if err := domain.ValidateID(id, "exploration"); err != nil {
		return nil, "", err
	}
	var bytes []byte
	var path string
	err := s.shared(ctx, func(ctx context.Context) error {
		var all loaded
		if err := s.Store.Read(ctx, func(tx *sql.Tx) error { var e error; all, e = loadAll(ctx, tx); return e }); err != nil {
			return err
		}
		view, err := buildTrace(all, id, s.Clock.Now())
		if err != nil {
			return err
		}
		bytes, err = encodeView(view, asJSON)
		if err != nil {
			return err
		}
		path, err = s.publishView(bytes, "trace", extension(asJSON))
		return err
	})
	return bytes, path, err
}

func buildTrace(all loaded, id string, generated time.Time) (traceView, error) {
	tasks := map[string]domain.Task{}
	cogs := map[string]domain.Cognition{}
	exps := map[string]domain.Exploration{}
	for _, v := range all.Tasks {
		tasks[v.ID] = v
	}
	for _, v := range all.Cognitions {
		cogs[v.ID] = v
	}
	for _, v := range all.Explorations {
		exps[v.ID] = v
	}
	current, ok := exps[id]
	if !ok {
		return traceView{}, domain.NewError("EXPLORATION_NOT_FOUND", "Exploration not found")
	}
	task, ok := tasks[current.TaskID]
	if !ok {
		return traceView{}, domain.NewError("WORKSPACE_STATE_UNTRUSTED", "Exploration refers to a missing Task")
	}
	includeC := map[string]bool{}
	includeE := map[string]bool{}
	includeOutputs := map[string]bool{}
	visitingC := map[string]bool{}
	var addC func(string) error
	var addE func(string, bool) error
	addC = func(cid string) error {
		if includeC[cid] {
			return nil
		}
		if visitingC[cid] {
			return domain.NewError("WORKSPACE_STATE_UNTRUSTED", "Graph contains a cycle")
		}
		c, ok := cogs[cid]
		if !ok {
			return domain.NewError("WORKSPACE_STATE_UNTRUSTED", "Graph references a missing Cognition")
		}
		visitingC[cid] = true
		includeC[cid] = true
		if c.OriginCognitionID != nil {
			if err := addC(*c.OriginCognitionID); err != nil {
				return err
			}
		}
		if c.OriginExplorationID != nil {
			if err := addE(*c.OriginExplorationID, false); err != nil {
				return err
			}
		}
		delete(visitingC, cid)
		return nil
	}
	addE = func(eid string, outputs bool) error {
		e, ok := exps[eid]
		if !ok {
			return domain.NewError("WORKSPACE_STATE_UNTRUSTED", "Graph references a missing Exploration")
		}
		if !includeE[eid] {
			includeE[eid] = true
			for _, cid := range e.Sources {
				if err := addC(cid); err != nil {
					return err
				}
			}
		}
		if outputs && !includeOutputs[eid] {
			includeOutputs[eid] = true
			for _, c := range all.Cognitions {
				if c.OriginExplorationID != nil && *c.OriginExplorationID == eid {
					if err := addC(c.ID); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := addE(id, true); err != nil {
		return traceView{}, err
	}
	for _, c := range all.Cognitions {
		if c.TaskID != nil && *c.TaskID == task.ID && c.TaskPurposeRelevant {
			if err := addC(c.ID); err != nil {
				return traceView{}, err
			}
		}
	}
	for cursor := current.RetryOf; cursor != nil; {
		e, ok := exps[*cursor]
		if !ok {
			return traceView{}, domain.NewError("WORKSPACE_STATE_UNTRUSTED", "retry chain refers to a missing Exploration")
		}
		if err := addE(e.ID, true); err != nil {
			return traceView{}, err
		}
		cursor = e.RetryOf
	}
	view := traceView{Current: currentExploration{current.ID, current.Title, current.Content}, OriginTask: originTask{task.Purpose, traceNote()}, Related: graphProjection{Cognitions: []cognitionProjection{}, Explorations: []explorationProjection{}}, GlobalCognitions: []globalProjection{}, GeneratedAt: domain.TimeText(generated)}
	for _, c := range all.Cognitions {
		if c.TaskID == nil {
			view.GlobalCognitions = append(view.GlobalCognitions, globalProjection{c.ID, c.Content, c.CreatedAt})
		} else if includeC[c.ID] {
			p := projectCognition(c)
			p.TaskPurposeRelevant = nil
			view.Related.Cognitions = append(view.Related.Cognitions, p)
		}
	}
	for _, e := range all.Explorations {
		if includeE[e.ID] {
			view.Related.Explorations = append(view.Related.Explorations, projectExploration(e))
		}
	}
	return view, nil
}

func traceNote() string {
	if i18n.Chinese() {
		return "只需落实 current_exploration，不需要完成整个 origin Task。"
	}
	return "Work only on current_exploration; do not attempt to complete the entire origin Task."
}

func (s *Service) VisualView(ctx context.Context, open bool, language string) (Result, error) {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		language = "en"
		if i18n.Chinese() {
			language = "zh"
		}
	}
	if language != "zh" && language != "en" {
		return Result{}, domain.NewError("INVALID_ARGUMENT", "lang must be zh or en")
	}
	var result Result
	err := s.shared(ctx, func(ctx context.Context) error {
		var all loaded
		var runtimeOccupied bool
		if err := s.Store.Read(ctx, func(tx *sql.Tx) error {
			var e error
			all, e = loadAll(ctx, tx)
			if e != nil {
				return e
			}
			return tx.QueryRowContext(ctx, `SELECT runtime_occupied FROM workspace_meta WHERE singleton=1`).Scan(&runtimeOccupied)
		}); err != nil {
			return err
		}
		generated := s.Clock.Now()
		zoneName, zoneOffset := generated.In(time.Local).Zone()
		renderedContent, err := renderVisualMarkdown(all)
		if err != nil {
			return domain.Wrap("INTERNAL_ERROR", "cannot render Visual Markdown", err)
		}
		payload := visualModel{
			Language:        language,
			Tasks:           all.Tasks,
			Cognitions:      all.Cognitions,
			Explorations:    all.Explorations,
			Edges:           visualEdges(all),
			RenderedContent: renderedContent,
			RuntimeOccupied: runtimeOccupied,
			GeneratedAt:     domain.TimeText(generated),
			DisplayTimezone: fmt.Sprintf("%s (%s)", zoneName, formatOffset(zoneOffset)),
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return domain.Wrap("INTERNAL_ERROR", "cannot encode Visual data", err)
		}
		html, err := visualHTML(data, language)
		if err != nil {
			return err
		}
		path, err := s.publishView(html, "visual", "html")
		if err != nil {
			return err
		}
		result = Result{Data: map[string]any{"path": path, "generated_at": payload.GeneratedAt, "opened": false}, Message: "Visual View generated"}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	if open {
		absolute := filepath.Join(s.Paths.Root, result.Data.(map[string]any)["path"].(string))
		if err := openBrowser(absolute); err != nil {
			result.Warnings = []output.Warning{output.OpenWarning(err)}
		} else {
			result.Data.(map[string]any)["opened"] = true
		}
	}
	return result, nil
}

type visualEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type visualModel struct {
	Language        string               `json:"language"`
	Tasks           []domain.Task        `json:"tasks"`
	Cognitions      []domain.Cognition   `json:"cognitions"`
	Explorations    []domain.Exploration `json:"explorations"`
	Edges           []visualEdge         `json:"edges"`
	RenderedContent map[string]string    `json:"rendered_content"`
	RuntimeOccupied bool                 `json:"runtime_occupied"`
	GeneratedAt     string               `json:"generated_at"`
	DisplayTimezone string               `json:"display_timezone"`
}

func visualEdges(all loaded) []visualEdge {
	edges := []visualEdge{}
	for _, exploration := range all.Explorations {
		for _, source := range exploration.Sources {
			edges = append(edges, visualEdge{From: source, To: exploration.ID, Kind: "source"})
		}
		if exploration.RetryOf != nil {
			edges = append(edges, visualEdge{From: *exploration.RetryOf, To: exploration.ID, Kind: "retry"})
		}
	}
	for _, cognition := range all.Cognitions {
		if cognition.OriginCognitionID != nil {
			edges = append(edges, visualEdge{From: *cognition.OriginCognitionID, To: cognition.ID, Kind: "extends"})
		}
		if cognition.OriginExplorationID != nil {
			edges = append(edges, visualEdge{From: *cognition.OriginExplorationID, To: cognition.ID, Kind: "output"})
		}
	}
	return edges
}

func formatOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	return fmt.Sprintf("%s%02d:%02d", sign, seconds/3600, seconds%3600/60)
}

func loadAll(ctx context.Context, tx *sql.Tx) (loaded, error) {
	var out loaded
	out.Tasks = []domain.Task{}
	out.Cognitions = []domain.Cognition{}
	out.Explorations = []domain.Exploration{}
	rows, e := tx.QueryContext(ctx, `SELECT id,purpose,init_cognition_ref,max_concurrency,status,outcome,outcome_reason,created_at,closed_at FROM tasks ORDER BY created_at ASC,id ASC`)
	if e != nil {
		return out, dbErr(e)
	}
	for rows.Next() {
		var t domain.Task
		var outcome, reason, closed sql.NullString
		if e := rows.Scan(&t.ID, &t.Purpose, &t.InitCognitionRef, &t.MaxConcurrency, &t.Status, &outcome, &reason, &t.CreatedAt, &closed); e != nil {
			rows.Close()
			return out, dbErr(e)
		}
		t.Outcome = ptr(outcome)
		t.OutcomeReason = ptr(reason)
		t.ClosedAt = ptr(closed)
		out.Tasks = append(out.Tasks, t)
	}
	rows.Close()
	rows, e = tx.QueryContext(ctx, `SELECT id,title,content,task_id,origin,origin_cognition_id,origin_exploration_id,task_purpose_relevant,created_at FROM cognitions ORDER BY created_at ASC,id ASC`)
	if e != nil {
		return out, dbErr(e)
	}
	for rows.Next() {
		var c domain.Cognition
		var task, oc, oe sql.NullString
		if e := rows.Scan(&c.ID, &c.Title, &c.Content, &task, &c.Origin, &oc, &oe, &c.TaskPurposeRelevant, &c.CreatedAt); e != nil {
			rows.Close()
			return out, dbErr(e)
		}
		c.TaskID = ptr(task)
		c.OriginCognitionID = ptr(oc)
		c.OriginExplorationID = ptr(oe)
		if c.OriginCognitionID != nil {
			c.OriginRef = c.OriginCognitionID
		} else {
			c.OriginRef = c.OriginExplorationID
		}
		out.Cognitions = append(out.Cognitions, c)
	}
	rows.Close()
	rows, e = tx.QueryContext(ctx, `SELECT id,title,content,task_id,retry_of,status,executor_id,output_ref,assigned_at,outcome,outcome_reason,created_at,finished_at FROM explorations ORDER BY created_at ASC,id ASC`)
	if e != nil {
		return out, dbErr(e)
	}
	for rows.Next() {
		var v domain.Exploration
		var retry, executor, outputRef, assigned, outcome, reason, finished sql.NullString
		if e := rows.Scan(&v.ID, &v.Title, &v.Content, &v.TaskID, &retry, &v.Status, &executor, &outputRef, &assigned, &outcome, &reason, &v.CreatedAt, &finished); e != nil {
			rows.Close()
			return out, dbErr(e)
		}
		v.RetryOf = ptr(retry)
		v.ExecutorID = ptr(executor)
		v.OutputRef = ptr(outputRef)
		v.AssignedAt = ptr(assigned)
		v.Outcome = ptr(outcome)
		v.OutcomeReason = ptr(reason)
		v.FinishedAt = ptr(finished)
		v.Sources, e = sourceIDs(ctx, tx, v.ID)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Explorations = append(out.Explorations, v)
	}
	rows.Close()
	return out, nil
}

func projectTask(t domain.Task) taskProjection {
	return taskProjection{t.ID, t.Purpose, t.Outcome, t.OutcomeReason, t.ClosedAt}
}
func projectCognition(c domain.Cognition) cognitionProjection {
	return cognitionProjection{ID: c.ID, Content: c.Content, CreatedAt: c.CreatedAt, OriginRef: c.OriginRef}
}
func projectExploration(e domain.Exploration) explorationProjection {
	return explorationProjection{ID: e.ID, Content: e.Content, RetryOf: e.RetryOf, Status: e.Status, Outcome: e.Outcome, OutcomeReason: e.OutcomeReason, Sources: e.Sources, FinishedAt: e.FinishedAt}
}
func ptr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	x := v.String
	return &x
}
func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func extension(json bool) string {
	if json {
		return "json"
	}
	return "yaml"
}
func encodeView(v any, asJSON bool) ([]byte, error) {
	if asJSON {
		b, e := json.MarshalIndent(v, "", "  ")
		if e != nil {
			return nil, domain.Wrap("INTERNAL_ERROR", "cannot encode JSON View", e)
		}
		return append(b, '\n'), nil
	}
	b, e := yaml.Marshal(v)
	if e != nil {
		return nil, domain.Wrap("INTERNAL_ERROR", "cannot encode YAML View", e)
	}
	return b, nil
}

func (s *Service) publishView(content []byte, kind, ext string) (string, error) {
	if err := os.MkdirAll(s.Paths.View, 0o700); err != nil {
		return "", domain.Wrap("IO_ERROR", "cannot create View directory", err)
	}
	token, err := s.IDs.Token()
	if err != nil {
		return "", domain.Wrap("INTERNAL_ERROR", "cannot generate View token", err)
	}
	stamp := s.Clock.Now().UTC().Format("20060102T150405.000000000Z")
	name := fmt.Sprintf("%s-%s-%s.%s", kind, stamp, token[:8], ext)
	final := filepath.Join(s.Paths.View, name)
	temp := final + ".tmp"
	if err := os.WriteFile(temp, content, 0o600); err != nil {
		return "", domain.Wrap("IO_ERROR", "cannot write View", err)
	}
	defer os.Remove(temp)
	if err := s.Files.SyncFile(temp); err != nil {
		return "", domain.Wrap("IO_ERROR", "cannot sync View", err)
	}
	if err := s.Files.Rename(temp, final); err != nil {
		return "", domain.Wrap("IO_ERROR", "cannot publish View", err)
	}
	if err := s.Files.SyncDir(s.Paths.View); err != nil {
		return "", domain.Wrap("IO_ERROR", "cannot sync View directory", err)
	}
	return filepath.ToSlash(filepath.Join(".norns", "view", name)), nil
}

func openBrowser(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return errors.New("no graphical session is available")
		}
		executable, err := exec.LookPath("xdg-open")
		if err != nil {
			return err
		}
		cmd = exec.Command(executable, path)
	}
	return cmd.Start()
}
