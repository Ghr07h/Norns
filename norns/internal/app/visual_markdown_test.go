package app

import (
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/domain"
)

func TestVisualMarkdownRendersGFMAndSanitizesHTML(t *testing.T) {
	renderer := newVisualMarkdownRenderer()
	content := "# Heading\n\n**strong** and ~~old~~\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n<script>alert(1)</script>\n\n[x](javascript:alert(2))\n\n[site](https://example.com)"
	rendered, err := renderer.Render(content)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"<h1>Heading</h1>", "<strong>strong</strong>", "<del>old</del>", "<table>", `href="https://example.com"`} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("rendered Markdown omitted %q: %s", required, rendered)
		}
	}
	for _, forbidden := range []string{"<script", "alert(1)", "javascript:", "alert(2)"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("rendered Markdown retained unsafe content %q: %s", forbidden, rendered)
		}
	}
}

func TestVisualMarkdownRendersTaskOutcomeReason(t *testing.T) {
	reason := "Closed with **verified evidence**"
	rendered, err := renderVisualMarkdown(loaded{Tasks: []domain.Task{{ID: "task_123", OutcomeReason: &reason}}})
	if err != nil {
		t.Fatal(err)
	}
	if rendered["task-reason:task_123"] != "<p>Closed with <strong>verified evidence</strong></p>\n" {
		t.Fatalf("unexpected rendered Task outcome reason: %q", rendered["task-reason:task_123"])
	}
}

func TestVisualMarkdownRendersExplorationOutcomeReason(t *testing.T) {
	reason := "Verified **evidence** with `interval arithmetic`.\n\n- Check precision\n- Reproduce bounds\n\n<script>alert(1)</script>\n\n[unsafe](javascript:alert(2))"
	rendered, err := renderVisualMarkdown(loaded{Explorations: []domain.Exploration{
		{ID: "exp_finished", Content: "Original **content**", Status: "finished", OutcomeReason: &reason},
		{ID: "exp_running", Content: "Running content", Status: "running"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if rendered["exp_finished"] != "<p>Original <strong>content</strong></p>\n" {
		t.Fatalf("unexpected rendered Exploration content: %q", rendered["exp_finished"])
	}
	result := rendered["exploration-reason:exp_finished"]
	for _, required := range []string{"<strong>evidence</strong>", "<code>interval arithmetic</code>", "<ul>", "<li>Check precision</li>"} {
		if !strings.Contains(result, required) {
			t.Fatalf("rendered Exploration outcome reason omitted %q: %s", required, result)
		}
	}
	for _, forbidden := range []string{"<script", "alert(1)", "javascript:", "alert(2)"} {
		if strings.Contains(result, forbidden) {
			t.Fatalf("rendered Exploration outcome reason retained unsafe content %q: %s", forbidden, result)
		}
	}
	if _, exists := rendered["exploration-reason:exp_running"]; exists {
		t.Fatal("Exploration without an outcome reason unexpectedly has rendered reason content")
	}
}

func TestVisualMarkdownDoesNotLoadImages(t *testing.T) {
	rendered, err := newVisualMarkdownRenderer().Render("![remote](https://example.com/image.png)")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, "<img") || strings.Contains(rendered, "image.png") {
		t.Fatalf("rendered Markdown retained a remote image: %s", rendered)
	}
}

func TestVisualMarkdownConvertsEscapedAndSoftLineBreaks(t *testing.T) {
	rendered, err := newVisualMarkdownRenderer().Render(`first\nsecond
third`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "first<br>\nsecond<br>\nthird") {
		t.Fatalf("rendered Markdown omitted hard line breaks: %s", rendered)
	}
}
