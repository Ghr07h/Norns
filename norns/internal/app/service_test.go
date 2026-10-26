package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ghr07h/norns/norns/internal/domain"
	storesqlite "github.com/Ghr07h/norns/norns/internal/store/sqlite"
	"github.com/Ghr07h/norns/norns/internal/workspace"
)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type testIDs struct {
	mu    sync.Mutex
	task  int
	cog   int
	exp   int
	token int
}

func (g *testIDs) next(prefix string, counter *int) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	*counter++
	return fmt.Sprintf("%s_%08x-0000-4000-8000-%012x", prefix, *counter, *counter), nil
}

func (g *testIDs) TaskID() (string, error)        { return g.next("task", &g.task) }
func (g *testIDs) CognitionID() (string, error)   { return g.next("cog", &g.cog) }
func (g *testIDs) ExplorationID() (string, error) { return g.next("exp", &g.exp) }
func (g *testIDs) Token() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.token++
	return fmt.Sprintf("%016x", g.token), nil
}

func newTestService(t *testing.T) (*Service, *testClock) {
	t.Helper()
	paths, err := workspace.Resolve(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ids := &testIDs{}
	clock := &testClock{now: time.Date(2026, 10, 12, 8, 30, 0, 0, time.UTC)}
	return &Service{
		Paths: paths,
		Store: storesqlite.New(paths, ids, clock),
		Lock:  workspace.NewLock(paths),
		IDs:   ids,
		Clock: clock,
		Files: workspace.OSFileOps{},
	}, clock
}

func resultID(t *testing.T, result Result) string {
	t.Helper()
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result data type %T", result.Data)
	}
	id, ok := data["id"].(string)
	if !ok {
		t.Fatalf("result has no string id: %#v", data)
	}
	return id
}

func TestProtocolWorkflowAndViews(t *testing.T) {
	ctx := context.Background()
	service, clock := newTestService(t)
	info := "Subject information"

	opened, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Investigate", Concurrency: 1, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	taskID := resultID(t, opened)
	originID := opened.Data.(map[string]any)["init_cognition_ref"].(string)

	global, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Global", Content: "Shared context", Origin: "user"})
	if err != nil {
		t.Fatal(err)
	}
	globalID := resultID(t, global)

	exploration, err := service.AddExploration(ctx, AddExplorationRequest{Title: "Check", Content: "Check the evidence", Sources: []string{originID, globalID}})
	if err != nil {
		t.Fatal(err)
	}
	explorationID := resultID(t, exploration)

	output, err := service.AddCognition(ctx, AddCognitionRequest{Title: "Finding", Content: "Evidence found", Origin: "exploration", ExplorationID: &explorationID, Relevant: true})
	if err != nil {
		t.Fatal(err)
	}
	outputID := resultID(t, output)

	traceBytes, tracePath, err := service.TraceView(ctx, explorationID, true)
	if err != nil {
		t.Fatalf("trace for an Exploration with outputs failed: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(tracePath), "trace-") {
		t.Fatalf("unexpected Trace filename: %s", tracePath)
	}
	if disk, err := os.ReadFile(filepath.Join(service.Paths.Root, tracePath)); err != nil || string(disk) != string(traceBytes) {
		t.Fatalf("published Trace differs from stdout bytes: err=%v", err)
	}
	var trace traceView
	if err := json.Unmarshal(traceBytes, &trace); err != nil {
		t.Fatal(err)
	}
	if trace.Current.ID != explorationID || trace.OriginTask.Purpose != "Investigate" {
		t.Fatalf("unexpected Trace context: %#v", trace)
	}
	if !containsCognition(trace.Related.Cognitions, outputID) {
		t.Fatalf("Trace omitted current output Cognition %s", outputID)
	}

	clock.Advance(2 * time.Second)
	if _, err := service.AssignExploration(ctx, explorationID, "executor-1", "output://one"); err != nil {
		t.Fatal(err)
	}
	clock.Advance(3 * time.Second)
	statusBytes, _, err := service.StatusView(ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	var status statusView
	if err := json.Unmarshal(statusBytes, &status); err != nil {
		t.Fatal(err)
	}
	if status.Task == nil || status.Task.ID != taskID || len(status.Running) != 1 || status.Running[0].DurationSeconds != 3 {
		t.Fatalf("unexpected running Status: %#v", status)
	}

	if _, err := service.FinishExploration(ctx, explorationID, "succeeded", "Done"); err != nil {
		t.Fatal(err)
	}
	retry, err := service.RetryExploration(ctx, explorationID)
	if err != nil {
		t.Fatal(err)
	}
	retryID := resultID(t, retry)
	traceBytes, _, err = service.TraceView(ctx, retryID, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(traceBytes, &trace); err != nil {
		t.Fatal(err)
	}
	if !containsExploration(trace.Related.Explorations, explorationID) || !containsExploration(trace.Related.Explorations, retryID) || !containsCognition(trace.Related.Cognitions, outputID) {
		t.Fatalf("retry Trace is incomplete: %#v", trace.Related)
	}

	closed, err := service.CloseTask(ctx, "succeeded", "Task complete")
	if err != nil {
		t.Fatal(err)
	}
	if closed.Data.(map[string]any)["cancelled_todo"] != int64(1) {
		t.Fatalf("close did not cancel retry todo: %#v", closed.Data)
	}

	clock.Advance(time.Second)
	second, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Follow up", Concurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	if second.Data.(map[string]any)["reused_init_cognition"] != true || second.Data.(map[string]any)["init_cognition_ref"] != originID {
		t.Fatalf("second Task did not reuse the latest closed Task init Cognition: %#v", second.Data)
	}
	if _, err := service.CloseTask(ctx, "cancelled", "Reset Subject"); err != nil {
		t.Fatal(err)
	}

	reset, err := service.Reset(ctx)
	if err != nil {
		t.Fatal(err)
	}
	archive := reset.Data.(map[string]any)["archive_path"].(string)
	if _, err := os.Stat(filepath.Join(service.Paths.Root, filepath.FromSlash(archive), "norns.sqlite")); err != nil {
		t.Fatalf("reset archive is incomplete: %v", err)
	}
	empty, _, err := service.StatusView(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(empty) == "" || !containsText(string(empty), "tasks: []") {
		t.Fatalf("empty Workspace Status must contain tasks: []:\n%s", empty)
	}
}

func TestInitIsIdempotentAndTodoRequiresOpenTask(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	first, err := service.Init(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Data.(map[string]any)["initialized"] != true {
		t.Fatalf("unexpected init result: %#v", first.Data)
	}
	second, err := service.Init(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.Data.(map[string]any)["existing"] != true {
		t.Fatalf("unexpected idempotent init result: %#v", second.Data)
	}
	if _, err := service.ListExplorations(ctx, "todo"); errorCode(err) != "NO_OPEN_TASK" {
		t.Fatalf("todo without open Task returned %v", err)
	}
}

func TestInitAcceptsOrphanWorkspaceDirectories(t *testing.T) {
	for _, layout := range []string{"norns-only", "history"} {
		t.Run(layout, func(t *testing.T) {
			service, _ := newTestService(t)
			if err := os.MkdirAll(service.Paths.Norns, 0o700); err != nil {
				t.Fatal(err)
			}
			marker := ""
			if layout == "history" {
				if err := os.MkdirAll(service.Paths.History, 0o700); err != nil {
					t.Fatal(err)
				}
				marker = filepath.Join(service.Paths.History, "keep")
				if err := os.WriteFile(marker, []byte("existing history"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if service.Store.Initialized() {
				t.Fatal("directory without norns.sqlite was treated as initialized")
			}
			result, err := service.Init(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if result.Data.(map[string]any)["initialized"] != true || !service.Store.Initialized() {
				t.Fatalf("orphan directory was not initialized: %#v", result.Data)
			}
			if marker != "" {
				content, err := os.ReadFile(marker)
				if err != nil || string(content) != "existing history" {
					t.Fatalf("existing history changed during init: %q %v", content, err)
				}
			}
		})
	}
}

func TestStructuredViewFieldOrderAndConditionalOmission(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	info := "yes"
	opened, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Order", Concurrency: 1, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	origin := opened.Data.(map[string]any)["init_cognition_ref"].(string)
	exploration, err := service.AddExploration(ctx, AddExplorationRequest{Title: "Order", Content: "on", Sources: []string{origin}})
	if err != nil {
		t.Fatal(err)
	}
	explorationID := resultID(t, exploration)

	status, _, err := service.StatusView(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	statusText := string(status)
	assertOrdered(t, statusText, "task:", "graph:", "global_cognitions:", "todo_explorations:", "running_explorations:", "generated_at:")
	if strings.Contains(statusText, "null") || strings.Contains(statusText, "task_purpose_relevant: false") || !strings.Contains(statusText, `content: "yes"`) {
		t.Fatalf("Status conditional fields or YAML string scalars are invalid:\n%s", statusText)
	}

	trace, _, err := service.TraceView(ctx, explorationID, false)
	if err != nil {
		t.Fatal(err)
	}
	traceText := string(trace)
	assertOrdered(t, traceText, "current_exploration:", "origin_task:", "related_subgraph:", "global_cognitions:", "generated_at:")
	for _, forbidden := range []string{"todo_explorations:", "running_explorations:", "task_purpose_relevant:", "null"} {
		if strings.Contains(traceText, forbidden) {
			t.Fatalf("Trace contains forbidden field %q:\n%s", forbidden, traceText)
		}
	}
}

func TestConcurrencyWarningDoesNotRejectAssign(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	info := "Subject"
	opened, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Run", Concurrency: 1, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	origin := opened.Data.(map[string]any)["init_cognition_ref"].(string)
	first, err := service.AddExploration(ctx, AddExplorationRequest{Title: "One", Content: "One", Sources: []string{origin}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.AddExploration(ctx, AddExplorationRequest{Title: "Two", Content: "Two", Sources: []string{origin}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AssignExploration(ctx, resultID(t, first), "one", "one-ref"); err != nil {
		t.Fatal(err)
	}
	assigned, err := service.AssignExploration(ctx, resultID(t, second), "two", "two-ref")
	if err != nil {
		t.Fatal(err)
	}
	if len(assigned.Warnings) != 1 || assigned.Warnings[0].Code != "CONCURRENCY_LIMIT_EXCEEDED" {
		t.Fatalf("expected concurrency warning, got %#v", assigned.Warnings)
	}
}

func TestAssignExplorationResolvesOutputSymlink(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	info := "Subject"
	opened, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Resolve output", Concurrency: 3, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	origin := opened.Data.(map[string]any)["init_cognition_ref"].(string)
	add := func(title string) string {
		t.Helper()
		result, addErr := service.AddExploration(ctx, AddExplorationRequest{Title: title, Content: title, Sources: []string{origin}})
		if addErr != nil {
			t.Fatal(addErr)
		}
		return resultID(t, result)
	}

	targetDir := filepath.Join(service.Paths.Root, "sessions")
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(targetDir, "real.output")
	if err := os.WriteFile(target, []byte("session"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkName := "session.output"
	link := filepath.Join(service.Paths.Root, linkName)
	if err := os.Symlink(filepath.Join("sessions", "real.output"), link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	assigned, err := service.AssignExploration(ctx, add("Linked"), "executor", linkName)
	if err != nil {
		t.Fatal(err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := assigned.Data.(map[string]any)["output_ref"]; got != resolvedTarget {
		t.Fatalf("resolved output_ref = %v, want %s", got, resolvedTarget)
	}

	opaque := "output://opaque"
	assigned, err = service.AssignExploration(ctx, add("Opaque"), "executor", opaque)
	if err != nil {
		t.Fatal(err)
	}
	if got := assigned.Data.(map[string]any)["output_ref"]; got != opaque {
		t.Fatalf("opaque output_ref changed to %v", got)
	}

	brokenName := "missing.output"
	if err := os.Symlink("does-not-exist.output", filepath.Join(service.Paths.Root, brokenName)); err != nil {
		t.Fatal(err)
	}
	assigned, err = service.AssignExploration(ctx, add("Broken"), "executor", brokenName)
	if err != nil {
		t.Fatal(err)
	}
	if got := assigned.Data.(map[string]any)["output_ref"]; got != brokenName {
		t.Fatalf("broken symlink output_ref changed to %v", got)
	}
}

func TestWriteRefusesToRepairPreexistingUntrustedState(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	info := "Subject"
	if _, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Open", Concurrency: 1, Info: &info}); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", service.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA ignore_check_constraints=ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE workspace_meta SET runtime_occupied=0`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := service.CloseTask(ctx, "failed", "Would otherwise repair occupancy"); errorCode(err) != "WORKSPACE_STATE_UNTRUSTED" || !strings.Contains(err.Error(), "manual Workspace inspection") {
		t.Fatalf("close should reject preexisting corruption with manual-recovery guidance, got %v", err)
	}
	db, err = sql.Open("sqlite", service.Paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var status string
	var runtime int
	if err := db.QueryRow(`SELECT status FROM tasks`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT runtime_occupied FROM workspace_meta`).Scan(&runtime); err != nil {
		t.Fatal(err)
	}
	if status != "open" || runtime != 0 {
		t.Fatalf("failed write modified untrusted state: status=%s runtime=%d", status, runtime)
	}
}

func TestVisualViewIsSelfContainedAndEscapesUserContent(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	malicious := `Subject </script><script>globalThis.injected=true</script>`
	opened, err := service.OpenTask(ctx, OpenTaskRequest{Purpose: "Visual", Concurrency: 1, Info: &malicious})
	if err != nil {
		t.Fatal(err)
	}
	origin := opened.Data.(map[string]any)["init_cognition_ref"].(string)
	if _, err := service.AddExploration(ctx, AddExplorationRequest{Title: "Inspect", Content: "Render graph", Sources: []string{origin}}); err != nil {
		t.Fatal(err)
	}
	result, err := service.VisualView(ctx, false, "en")
	if err != nil {
		t.Fatal(err)
	}
	data := result.Data.(map[string]any)
	if data["opened"] != false {
		t.Fatalf("Visual must not open by default: %#v", data)
	}
	if !strings.HasPrefix(filepath.Base(data["path"].(string)), "visual-") {
		t.Fatalf("unexpected Visual filename: %s", data["path"])
	}
	html, err := os.ReadFile(filepath.Join(service.Paths.Root, data["path"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(html)
	if !strings.Contains(text, `<html lang="en">`) || !strings.Contains(text, `"language":"en"`) {
		t.Fatalf("Visual did not preserve the requested language: %s", text[:min(len(text), 500)])
	}
	for _, required := range []string{`id="norns-data"`, `"kind":"source"`, `"language":"en"`, `runtime_occupied`, `rendered_content`, `Norns Visual Snapshot`, `data-i18n="brand"`, `messages.zh.brand='Norns 可视化快照'`, `id="exploration-task-filter"`, `id="search-toggle"`, `id="viewport"`, `id="task-list"`, `id="exploration-list"`, `id="zoom-value"`, `id="fullscreen"`, `nodeElements=new Map()`, `marker-end`, `Task-Related`, `Output Cognitions`, `tasksTitle:'任务记录'`, `graphTitle:'图记录'`, `explorationsTitle:'探索记录'`, `unachievable:'无法达成'`, `outcomeBadge(task.outcome)`, `function explorationStateLabel`, `outcomeName(item.outcome)`, `String(a.created_at).localeCompare(String(b.created_at))||a.id.localeCompare(b.id)`, `function closeExplorationPanel`, `closeExplorationPanel()`, `foreignObject`, `cognition-bronze`, `node-status-ring`, `graphNodeHeight`, `alignGraphRank`, `edge.kind==='extends'`, `rowGap=72`, `planGraphCurves`, `planSameColumnLanes`, `graphColumnShift`, `graphRoutes`, `graphSplinePath`, `kind:'gutter'`, `kind:'same-column'`, `cognitionChain`, `toSide=cognitionChain?'in'`, `from.y+from.height/2+startOffset`, `Math.min(16,64/(entries.length-1))`, `shoulder=66+lane*18`, `graphEndpointGap=3`, `refX:'10'`, `stroke-width:1.55`, `arrow-relation`, `stroke:#a5aeaa`} {
		if !strings.Contains(text, required) {
			t.Fatalf("Visual omitted %q", required)
		}
	}
	for _, removed := range []string{`planGraphTracks`, `class:'edge-label'`, `class="legend"`, `id="visible-count"`, `{scroll:true}`, `rowGap=42`, `toSide=sameColumn?'out'`, `stroke-width:1.35`, `rightX=sx+64`, `const order={running:0,todo:1,finished:2}`} {
		if strings.Contains(text, removed) {
			t.Fatalf("Visual unexpectedly retained %q", removed)
		}
	}
	if strings.Contains(text, malicious) || strings.Contains(text, `<script>globalThis.injected=true</script>`) {
		t.Fatal("Visual embedded user content as executable markup")
	}
	if strings.Contains(text, `src="http`) || strings.Contains(text, `href="http`) {
		t.Fatal("Visual unexpectedly contains a remote resource URL")
	}
}

func TestVisualViewLanguageSelection(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	if _, err := service.VisualView(ctx, false, "fr"); errorCode(err) != "INVALID_ARGUMENT" {
		t.Fatalf("invalid language returned %q, want INVALID_ARGUMENT: %v", errorCode(err), err)
	}
	if service.Store.Initialized() {
		t.Fatal("language validation accessed or initialized the Workspace")
	}
	if _, err := service.Init(ctx); err != nil {
		t.Fatal(err)
	}

	readLanguage := func(language string) string {
		t.Helper()
		result, err := service.VisualView(ctx, false, language)
		if err != nil {
			t.Fatal(err)
		}
		path := result.Data.(map[string]any)["path"].(string)
		content, err := os.ReadFile(filepath.Join(service.Paths.Root, path))
		if err != nil {
			t.Fatal(err)
		}
		return string(content)
	}

	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	english := readLanguage("en")
	if !strings.Contains(english, `<html lang="en">`) || !strings.Contains(english, `"language":"en"`) {
		t.Fatal("explicit English did not override the operating system locale")
	}
	chinese := readLanguage("")
	if !strings.Contains(chinese, `<html lang="zh-CN">`) || !strings.Contains(chinese, `"language":"zh"`) {
		t.Fatal("omitted language did not follow the operating system locale")
	}
}

func TestResetRecoveryPreparedRollsBack(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	createClosedTask(t, service)
	j, newRoot := stageReset(t, service, "prepared", "00000000000000a1")

	if _, err := service.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(newRoot); !os.IsNotExist(err) {
		t.Fatalf("prepared staging root still exists: %v", err)
	}
	if _, err := os.Stat(service.Paths.ResetLog); !os.IsNotExist(err) {
		t.Fatalf("prepared reset journal still exists: %v", err)
	}
	view, _, err := service.StatusView(ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	var status statusView
	if err := json.Unmarshal(view, &status); err != nil {
		t.Fatal(err)
	}
	if status.Tasks == nil || len(*status.Tasks) != 1 {
		t.Fatalf("prepared recovery did not retain the old Workspace: %#v (journal %#v)", status, j)
	}
}

func TestResetRecoveryOldRenamedRollsForward(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	createClosedTask(t, service)
	j, _ := stageReset(t, service, "prepared", "00000000000000b2")
	oldRoot := filepath.Join(service.Paths.Root, j.OldRoot)
	if err := os.Rename(service.Paths.Norns, oldRoot); err != nil {
		t.Fatal(err)
	}
	j.Phase = "old_renamed"
	if err := service.writeResetJournal(j); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Init(ctx); err != nil {
		t.Fatal(err)
	}
	view, _, err := service.StatusView(ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	var status statusView
	if err := json.Unmarshal(view, &status); err != nil {
		t.Fatal(err)
	}
	if status.Tasks == nil || len(*status.Tasks) != 0 {
		t.Fatalf("rolled-forward Workspace is not empty: %#v", status)
	}
	archiveDB := filepath.Join(service.Paths.History, j.ArchiveName, "norns.sqlite")
	if _, err := os.Stat(archiveDB); err != nil {
		t.Fatalf("old Workspace was not archived: %v", err)
	}
	if _, err := os.Stat(service.Paths.ResetLog); !os.IsNotExist(err) {
		t.Fatalf("roll-forward reset journal still exists: %v", err)
	}
}

func TestResetRecoveryNewPublishedAndArchived(t *testing.T) {
	for _, phase := range []string{"new_published", "archived"} {
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			service, _ := newTestService(t)
			createClosedTask(t, service)
			j, newRoot := stageReset(t, service, "prepared", "00000000000000c3")
			oldRoot := filepath.Join(service.Paths.Root, j.OldRoot)
			if err := os.Rename(service.Paths.Norns, oldRoot); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(newRoot, service.Paths.Norns); err != nil {
				t.Fatal(err)
			}
			j.Phase = "new_published"
			if phase == "archived" {
				if _, err := service.finishReset(j); err != nil {
					t.Fatal(err)
				}
				j.Phase = "archived"
			}
			if err := service.writeResetJournal(j); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Init(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(service.Paths.History, j.ArchiveName, "norns.sqlite")); err != nil {
				t.Fatalf("archive missing after %s recovery: %v", phase, err)
			}
			if _, err := os.Stat(service.Paths.ResetLog); !os.IsNotExist(err) {
				t.Fatalf("journal remains after %s recovery: %v", phase, err)
			}
		})
	}
}

func TestResetRecoveryRejectsForgedJournalPaths(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	if _, err := service.Init(ctx); err != nil {
		t.Fatal(err)
	}
	cases := []resetJournal{
		{Version: 1, Token: "../../../../tmp/x", Phase: "prepared", OldRoot: ".norns.old.../../../../tmp/x", NewRoot: ".norns.new.../../../../tmp/x", ArchiveName: "20261012T083000.000000000Z-00000000", CreatedAt: "2026-10-12T08:30:00Z"},
		{Version: 1, Token: "00000000000000d4", Phase: "prepared", OldRoot: ".norns.old.00000000000000d4", NewRoot: ".norns.new.00000000000000d4", ArchiveName: "..", CreatedAt: "2026-10-12T08:30:00Z"},
		{Version: 1, Token: "00000000000000d4", Phase: "unknown", OldRoot: ".norns.old.00000000000000d4", NewRoot: ".norns.new.00000000000000d4", ArchiveName: "20261012T083000.000000000Z-00000000", CreatedAt: "2026-10-12T08:30:00Z"},
	}
	for index, journal := range cases {
		payload, err := json.Marshal(journal)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(service.Paths.ResetLog, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Init(ctx); errorCode(err) != "RESET_RECOVERY_FAILED" {
			t.Fatalf("forged journal %d was accepted: %v", index, err)
		}
		if _, err := os.Stat(service.Paths.Database); err != nil {
			t.Fatalf("forged journal %d modified the Workspace: %v", index, err)
		}
		if err := os.Remove(service.Paths.ResetLog); err != nil {
			t.Fatal(err)
		}
	}
}

func createClosedTask(t *testing.T, service *Service) {
	t.Helper()
	info := "Subject"
	if _, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Old", Concurrency: 1, Info: &info}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CloseTask(context.Background(), "succeeded", "Complete"); err != nil {
		t.Fatal(err)
	}
}

func stageReset(t *testing.T, service *Service, phase, token string) (resetJournal, string) {
	t.Helper()
	j := resetJournal{
		Version:     1,
		Token:       token,
		Phase:       phase,
		OldRoot:     ".norns.old." + token,
		NewRoot:     ".norns.new." + token,
		ArchiveName: "20261012T083000.000000000Z-" + token[:8],
		CreatedAt:   domain.TimeText(service.Clock.Now()),
	}
	newRoot := filepath.Join(service.Paths.Root, j.NewRoot)
	paths := service.Paths
	paths.Norns = newRoot
	paths.Database = filepath.Join(newRoot, "norns.sqlite")
	paths.View = filepath.Join(newRoot, "view")
	paths.History = filepath.Join(newRoot, "history")
	if err := storesqlite.New(paths, service.IDs, service.Clock).Initialize(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := service.writeResetJournal(j); err != nil {
		t.Fatal(err)
	}
	return j, newRoot
}

func containsCognition(items []cognitionProjection, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func containsExploration(items []explorationProjection, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func containsText(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}

func assertOrdered(t *testing.T, text string, fields ...string) {
	t.Helper()
	last := -1
	for _, field := range fields {
		index := strings.Index(text, field)
		if index <= last {
			t.Fatalf("field %q is absent or out of order:\n%s", field, text)
		}
		last = index
	}
}

func errorCode(err error) string {
	if typed, ok := err.(*domain.Error); ok {
		return typed.Code
	}
	return ""
}
