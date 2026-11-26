package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ghr07h/norns/norns/internal/domain"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

func TestMarkdownViewsPreserveProjectionAndPublish(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	s, exp := goldenViewWorkspace(t)
	for _, tc := range []struct {
		name     string
		load     func() ([]byte, string, error)
		sections []string
	}{
		{"Status", func() ([]byte, string, error) { return s.StatusView(context.Background(), nil, ViewMarkdown) }, []string{"Task", "Graph", "Status", "Global_hints", "Todo_explorations", "Running_explorations", "Recent_top10_derived_cognitions", "Generated_at"}},
		{"Trace", func() ([]byte, string, error) { return s.TraceView(context.Background(), exp, ViewMarkdown) }, []string{"Current_exploration", "Graph", "Parent_task", "Trace", "Global_hints", "Running_explorations", "Generated_at"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content, path, err := tc.load()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(string(content), "# "+tc.name+" View\n\n") || !strings.HasSuffix(path, ".md") {
				t.Fatalf("invalid Markdown artifact: %s\n%s", path, content)
			}
			file, err := os.ReadFile(filepath.Join(s.Paths.Root, path))
			if err != nil || !bytes.Equal(file, content) {
				t.Fatalf("published Markdown differs: %v", err)
			}
			var sections []string
			for _, line := range strings.Split(string(content), "\n") {
				if strings.HasPrefix(line, "## ") {
					sections = append(sections, strings.TrimPrefix(line, "## "))
				}
			}
			if strings.Join(sections, ",") != strings.Join(tc.sections, ",") {
				t.Fatalf("wrong top-level headings: %v", sections)
			}
			if strings.Contains(string(content), "meta") || (tc.name == "Trace" && strings.Contains(string(content), "task_purpose_relevant")) {
				t.Fatal("Markdown exposed fields omitted from the projection")
			}
			for _, want := range []string{"### Task-Init Cognitions", " - Inspect", " - Finding", " - User hint", "Check the original evidence", "Shared context", "Evidence found", "| duration_seconds ", "**Content**", "- **Origin:** `user`"} {
				if !strings.Contains(string(content), want) {
					t.Fatalf("missing %q in %s", want, content)
				}
			}
		})
	}
}

func TestMarkdownEmptyViewsAndInvalidFormat(t *testing.T) {
	s, _ := newTestService(t)
	if _, err := s.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	content, _, err := s.StatusView(context.Background(), nil, ViewMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Tasks\n\n`[]`", "## Graph\n\n### Cognitions\n\n```text\n[]\n```\n\n### Explorations\n\n```text\n[]\n```", "## Global_hints\n\n`[]`", "## Running_explorations\n\n| id | title | duration_seconds |\n| --- | --- | --- |"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("missing empty list %q: %s", want, content)
		}
	}
	if strings.Contains(string(content), "recent_top10") || strings.Contains(string(content), "## Task\n") {
		t.Fatal("empty View invented conditional fields")
	}
	if _, _, err := s.StatusView(context.Background(), nil, ViewFormat("xml")); errorCode(err) != "INVALID_ARGUMENT" {
		t.Fatalf("invalid format accepted: %v", err)
	}
	if _, _, err := s.TraceView(context.Background(), "", ViewFormat("xml")); errorCode(err) != "INVALID_ARGUMENT" {
		t.Fatalf("invalid format accepted: %v", err)
	}
}

func TestMarkdownProseAndTableEscaping(t *testing.T) {
	expID := "exp_one"
	view := traceView{
		Current: currentExploration{ID: "exp_one", Title: "Inspect `code`", Content: "# Fake title\n\n```md\n## Fake section\n```\n<script>alert(1)</script>\n**literal** & 中文"},
		Running: []traceRunningProjection{{runningProjection: runningProjection{ID: "exp_one", Title: "a | b `c`\nnext", DurationSeconds: 12}, ExplorerID: "worker`|<b>"}},
	}
	view.Related = graphProjection{
		Cognitions:   []cognitionProjection{{ID: "cog_one", Content: "Cognition text", OriginRef: &expID}},
		Explorations: []explorationProjection{{ID: expID, Content: "Exploration text", Status: "todo", Sources: []string{"cog_root"}}},
	}
	all := loaded{
		Cognitions:   []domain.Cognition{{ID: "cog_one", Title: "Finding\n# forged **heading** <script>", Origin: "exploration", OriginRef: &expID}},
		Explorations: []domain.Exploration{{ID: expID, Title: "Direction `code`\n## forged section"}},
	}
	content, err := encodeMarkdownView(view, "Trace", all)
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	if err := goldmark.New(goldmark.WithExtensions(extension.GFM)).Convert(content, &rendered); err != nil {
		t.Fatal(err)
	}
	html := rendered.String()
	if strings.Count(html, "<h1>") != 1 || strings.Count(html, "<h2>") != 7 || strings.Count(html, "<h3>") != 3 || strings.Count(html, "<h4>") != 1 || strings.Contains(html, "<script>") || strings.Contains(html, "<b>") {
		t.Fatalf("entity text broke the document structure:\n%s\n%s", content, html)
	}
	if strings.Count(html, "<td>") != 4 || !strings.Contains(html, "a | b `c` next") || !strings.Contains(html, "&lt;script&gt;") || !strings.Contains(html, "**literal** &amp; 中文") {
		t.Fatalf("text or table values changed:\n%s", html)
	}
}

func TestMarkdownExplorationLayoutAndEnrichedSummaries(t *testing.T) {
	ctx := context.Background()
	s, task, root := protocolWorkspace(t)
	clock := s.Clock.(*testClock)
	first := addExplorationForTest(t, s, root, "First direction")
	hint := addUserHint(t, s, first, false)
	clock.Advance(time.Second)
	finding := addFinding(t, s, first)
	clock.Advance(time.Second)
	unrelated := addFinding(t, s, first)
	clock.Advance(time.Second)
	second, err := s.AddExploration(ctx, AddExplorationRequest{Title: "Second direction", Content: "Follow the selected finding", Sources: []string{finding}})
	if err != nil {
		t.Fatal(err)
	}
	secondID := resultID(t, second)
	clock.Advance(time.Second)
	last := addFinding(t, s, secondID)
	status, _, err := s.StatusView(ctx, nil, ViewMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	text := string(status)
	if !strings.Contains(text, "## Task\n\n> `"+task+"`\n\n**Purpose**\n\n> Investigate") {
		t.Fatalf("wrong Task layout: %s", text)
	}
	assertOrdered(t, text, "## Status\n", "### `"+first+"` - First direction", "#### `"+finding+"` - Finding", "#### `"+unrelated+"` - Finding", "#### `"+hint+"` - User input", "### `"+secondID+"` - Second direction", "#### `"+last+"` - Finding")
	for _, id := range []string{first, secondID} {
		start := strings.Index(text, "### `"+id+"`")
		if start < 0 {
			t.Fatal("missing exploration heading")
		}
		assertOrdered(t, text[start:], "- **Status:**", "- **Sources:**", "**Content**")
	}
	recent := text[strings.Index(text, "## Recent_top10_derived_cognitions\n"):]
	if !strings.Contains(recent, "| id | title | created_at |") || !strings.Contains(recent, "| `"+last+"` | `Finding` | `"+domain.TimeText(clock.Now())+"` |") {
		t.Fatalf("recent summary missing labels or timestamps: %s", recent)
	}
	assertOrdered(t, recent, last, unrelated, finding)
	trace, _, err := s.TraceView(ctx, secondID, ViewMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	assertOrdered(t, string(trace), "## Trace\n", "### `"+first+"` - First direction", "#### `"+finding+"` - Finding", "#### `"+hint+"` - User input", "### `"+secondID+"` - Second direction", "#### `"+last+"` - Finding")
	if strings.Contains(string(trace), unrelated) || strings.Contains(string(trace), "Recent_top10") {
		t.Fatalf("Markdown expanded the Trace scope: %s", trace)
	}
	if strings.Contains(text, "##### ") || strings.Contains(text, "### cognitions") {
		t.Fatal("old graph layout remains")
	}
	yaml, _, err := s.StatusView(ctx, nil, ViewYAML)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(yaml), "title: Finding") || !strings.Contains(string(yaml), "recent_top10_derived_cognitions: ["+last+", "+unrelated+", "+finding+"]") {
		t.Fatalf("Markdown enrichment changed YAML: %s", yaml)
	}
}

func TestMarkdownTaskInitialCognitionsStayInViewScope(t *testing.T) {
	ctx := context.Background()
	s, _, firstRoot := protocolWorkspace(t)
	firstExp := addExplorationForTest(t, s, firstRoot, "Historical direction")
	if _, err := s.CloseTask(ctx, "cancelled", "Next Task"); err != nil {
		t.Fatal(err)
	}
	newInfo := "Independent initial knowledge"
	opened, err := s.OpenTask(ctx, OpenTaskRequest{Purpose: "New Task", Concurrency: 1, Info: &newInfo})
	if err != nil {
		t.Fatal(err)
	}
	secondRoot := opened.Data.(map[string]any)["init_cognition_ref"].(string)
	status, _, err := s.StatusView(ctx, nil, ViewMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	assertOrdered(t, string(status), "## Status\n", "### Task-Init Cognitions\n", "#### `"+firstRoot+"` - ", "#### `"+secondRoot+"` - ", "### `"+firstExp+"` - Historical direction")
	if strings.Contains(string(status), "## Task_cognitions") || strings.Count(string(status), "### Task-Init Cognitions\n") != 1 {
		t.Fatalf("wrong Task-init section: %s", status)
	}
	trace, _, err := s.TraceView(ctx, firstExp, ViewMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	assertOrdered(t, string(trace), "## Trace\n", "### Task-Init Cognitions\n", "#### `"+firstRoot+"` - ", "### `"+firstExp+"` - Historical direction")
	if strings.Contains(string(trace), secondRoot) || strings.Contains(string(trace), newInfo) {
		t.Fatalf("Trace included unrelated Task-init data: %s", trace)
	}
}

func TestMarkdownGraphSummaryMatchesProjection(t *testing.T) {
	s, exp := goldenViewWorkspace(t)
	for _, name := range []string{"Status", "Trace"} {
		t.Run(name, func(t *testing.T) {
			var content []byte
			var err error
			if name == "Status" {
				content, _, err = s.StatusView(context.Background(), nil, ViewMarkdown)
			} else {
				content, _, err = s.TraceView(context.Background(), exp, ViewMarkdown)
			}
			if err != nil {
				t.Fatal(err)
			}
			text := string(content)
			var headings []string
			for _, line := range strings.Split(text, "\n") {
				if strings.HasPrefix(line, "## ") {
					headings = append(headings, line)
				}
			}
			if len(headings) < 2 || headings[1] != "## Graph" {
				t.Fatalf("Graph is not the second section: %v", headings)
			}
			start := strings.Index(text, "## Graph\n")
			end := strings.Index(text[start+len("## Graph\n"):], "\n## ")
			if end < 0 {
				t.Fatal("Graph replaced the detail sections")
			}
			summary := text[start : start+len("## Graph\n")+end]
			for _, want := range []string{"### Cognitions\n", "cog_1: Subject information\n", "cog_4: Finding <- exp_1\n", "### Explorations\n", "exp_1: (running) Inspect <- ['cog_1', 'cog_2']\n"} {
				if !strings.Contains(summary, want) {
					t.Fatalf("Graph missing %q: %s", want, summary)
				}
			}
			for _, forbidden := range []string{"cog_2:", "cog_3:", "Inspect evidence", "Evidence found", "created_at", "<- task_"} {
				if strings.Contains(summary, forbidden) {
					t.Fatalf("Graph included %q: %s", forbidden, summary)
				}
			}
			if !strings.Contains(text, "## "+name+"\n") || !strings.Contains(text, "#### `cog_3` - User hint") {
				t.Fatal("Graph changed the detailed content")
			}
		})
	}
}

func TestMarkdownGraphSummaryEscapesMultilineTitlesAndFences(t *testing.T) {
	expID := "exp_1"
	view := statusView{Graph: graphProjection{Cognitions: []cognitionProjection{{ID: "cog_1", OriginRef: &expID}}, Explorations: []explorationProjection{{ID: expID, Status: "running", Sources: []string{"cog_1"}}}}}
	all := loaded{Cognitions: []domain.Cognition{{ID: "cog_1", Title: "A ```\n## forged", Origin: "exploration", OriginRef: &expID}}, Explorations: []domain.Exploration{{ID: expID, Title: "B\r\n<script>"}}}
	content, err := encodeMarkdownView(view, "Status", all)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "````text\ncog_1: A ``` ## forged <- exp_1\n````") || !strings.Contains(string(content), "exp_1: (running) B <script> <- ['cog_1']") {
		t.Fatalf("summary did not preserve single-line titles: %s", content)
	}
	var rendered bytes.Buffer
	if err := goldmark.New().Convert(content, &rendered); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered.String(), "<script>") || strings.Contains(rendered.String(), "<h2>forged") {
		t.Fatalf("title escaped its code block: %s", rendered.String())
	}
}
