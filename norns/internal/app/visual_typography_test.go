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
		".graph-shell .graph-body":  {"grid-template-columns:minmax(0,1fr)"},
		`.graph-shell[data-graph-view="knowledge"] .node-title`:   {"display:none"},
		`.knowledge-label-copy`:                                   {"font-size:12px", "line-height:1.45", "font-weight:500"},
		`.knowledge-label-text`:                                   {"-webkit-line-clamp:var(--knowledge-label-lines,2)", "overflow-wrap:anywhere", "overflow:hidden"},
		`.graph-shell .node.knowledge-root .knowledge-label-copy`: {"font-size:13px", "font-weight:700"},
	} {
		rules := regexp.MustCompile(`(?:^|[{}])\s*`+regexp.QuoteMeta(selector)+`\{([^{}]+)\}`).FindAllSubmatch(style[1], -1)
		if len(rules) == 0 {
			t.Errorf("Visual omitted typography selector %q", selector)
			continue
		}
		properties := make(map[string]string)
		for _, rule := range rules {
			for _, declaration := range strings.Split(string(rule[1]), ";") {
				if name, value, ok := strings.Cut(declaration, ":"); ok {
					properties[name] = value
				}
			}
		}
		for _, declaration := range declarations {
			name, value, _ := strings.Cut(declaration, ":")
			if !strings.Contains(properties[name], value) {
				t.Errorf("Visual selector %q omitted %q", selector, declaration)
			}
		}
	}
	if !strings.Contains(string(style[1]), ".graph-body{grid-template-columns:1fr;height:620px}") {
		t.Error("Visual omitted the compact-screen graph layout")
	}
}
