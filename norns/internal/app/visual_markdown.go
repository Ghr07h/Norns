package app

import (
	"bytes"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	goldmarkext "github.com/yuin/goldmark/extension"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
)

type visualMarkdownRenderer struct {
	markdown goldmark.Markdown
	policy   *bluemonday.Policy
}

func newVisualMarkdownRenderer() visualMarkdownRenderer {
	policy := bluemonday.NewPolicy()
	policy.AllowElements(
		"p", "br", "strong", "em", "s", "del", "blockquote",
		"ul", "ol", "li", "pre", "code", "hr",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"table", "thead", "tbody", "tr", "th", "td",
		"a",
	)
	policy.AllowStandardURLs()
	policy.RequireParseableURLs(true)
	policy.AllowAttrs("href", "title").OnElements("a")
	policy.RequireNoFollowOnLinks(true)
	policy.RequireNoReferrerOnLinks(true)
	policy.AddTargetBlankToFullyQualifiedLinks(true)
	return visualMarkdownRenderer{
		markdown: goldmark.New(
			goldmark.WithExtensions(goldmarkext.GFM),
			goldmark.WithRendererOptions(goldmarkhtml.WithHardWraps()),
		),
		policy: policy,
	}
}

func (r visualMarkdownRenderer) Render(source string) (string, error) {
	var rendered bytes.Buffer
	source = strings.ReplaceAll(source, `\n`, "\n")
	if err := r.markdown.Convert([]byte(source), &rendered); err != nil {
		return "", err
	}
	return r.policy.Sanitize(rendered.String()), nil
}

func renderVisualMarkdown(all loaded) (map[string]string, error) {
	renderer := newVisualMarkdownRenderer()
	result := make(map[string]string, len(all.Tasks)+len(all.Cognitions)+len(all.Explorations))
	for _, task := range all.Tasks {
		if task.OutcomeReason == nil {
			continue
		}
		rendered, err := renderer.Render(*task.OutcomeReason)
		if err != nil {
			return nil, err
		}
		result["task-reason:"+task.ID] = rendered
	}
	for _, cognition := range all.Cognitions {
		rendered, err := renderer.Render(cognition.Content)
		if err != nil {
			return nil, err
		}
		result[cognition.ID] = rendered
	}
	for _, exploration := range all.Explorations {
		rendered, err := renderer.Render(exploration.Content)
		if err != nil {
			return nil, err
		}
		result[exploration.ID] = rendered
	}
	return result, nil
}
