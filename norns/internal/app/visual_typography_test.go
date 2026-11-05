package app

import (
	"regexp"
	"strings"
	"testing"
)

func TestVisualTypographyKeepsReadableSizes(t *testing.T) {
	html, err := visualHTML([]byte(`{}`), "zh")
	if err != nil {
		t.Fatal(err)
	}
	style := regexp.MustCompile(`(?s)<style>(.*?)</style>`).FindSubmatch(html)
	if len(style) != 2 {
		t.Fatal("Visual omitted its embedded stylesheet")
	}
	for selector, declarations := range map[string][]string{
		"body": {"font:15px/1.65"},
		".task-block-value,.task-cognition .markdown": {"font-size:15px", "font-weight:400", "line-height:1.7"},
		".task-progress":            {"font-size:12px"},
		".metric-label":             {"font-size:12px"},
		".metric-value":             {"font-size:20px"},
		".detail-title":             {"font-size:18px"},
		".markdown":                 {"line-height:1.7"},
		".markdown code":            {"font:13px/1.6"},
		".filter-label":             {"font-size:12px"},
		".exploration-attribute dt": {"font-size:12px"},
		".exploration-attribute dd": {"font-size:13px"},
		".node-title":               {"font-size:13px"},
		".graph-body":               {"grid-template-columns:minmax(0,1fr) 360px"},
		`.graph-shell[data-graph-view="knowledge"] .node-title`:   {"display:none"},
		`.knowledge-label-copy`:                                   {"font:600 13px/1.5", "text-align:left"},
		`.knowledge-label-text`:                                   {"-webkit-line-clamp:var(--knowledge-label-lines,2)", "overflow-wrap:anywhere", "overflow:hidden"},
		`.graph-shell .node.knowledge-root .knowledge-label-copy`: {"font-size:13px", "font-weight:700"},
	} {
		rule := regexp.MustCompile(regexp.QuoteMeta(selector) + `\{([^{}]+)\}`).FindSubmatch(style[1])
		if len(rule) != 2 {
			t.Errorf("Visual omitted typography selector %q", selector)
			continue
		}
		for _, declaration := range declarations {
			if !strings.Contains(string(rule[1]), declaration) {
				t.Errorf("Visual selector %q omitted %q", selector, declaration)
			}
		}
	}
	if !strings.Contains(string(style[1]), ".graph-body{grid-template-columns:1fr;height:620px}") {
		t.Error("Visual omitted the compact-screen graph layout")
	}
}
