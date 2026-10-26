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
