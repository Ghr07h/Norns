package app

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Ghr07h/norns/norns/internal/domain"
)

// ViewFormat selects the representation of the same protocol projection.
type ViewFormat string

const (
	ViewYAML     ViewFormat = "yaml"
	ViewJSON     ViewFormat = "json"
	ViewMarkdown ViewFormat = "md"
)

func (f ViewFormat) Validate() error {
	switch f {
	case ViewYAML, ViewJSON, ViewMarkdown:
		return nil
	default:
		return domain.NewError("INVALID_ARGUMENT", "View format must be yaml, json or md")
	}
}

func encodeMarkdownView(view any, name string, all loaded) ([]byte, error) {
	// YAML nodes retain the projection's field order, inline fields and omissions.
	var root yaml.Node
	if err := root.Encode(view); err != nil {
		return nil, domain.Wrap("INTERNAL_ERROR", "cannot encode Markdown View", err)
	}
	context := newMarkdownContext(all)
	var out strings.Builder
	fmt.Fprintf(&out, "# %s View\n\n", name)
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i].Value, root.Content[i+1]
		heading := markdownLabel(key)
		if key == "graph" || key == "related_subgraph" {
			heading = name
		}
		markdownHeading(&out, 2, heading)
		switch key {
		case "task":
			markdownTask(&out, value)
		case "tasks":
			if len(value.Content) == 0 {
				out.WriteString("`[]`\n\n")
			}
			for _, task := range value.Content {
				markdownTask(&out, task)
			}
		case "graph", "related_subgraph":
			context.graph(&out, value)
		case "global_hints":
			if len(value.Content) == 0 {
				out.WriteString("`[]`\n\n")
			}
			for _, cog := range value.Content {
				context.cognition(&out, cog, 3)
			}
		case "todo_explorations":
			markdownTable(&out, value, []string{"id", "title"})
		case "running_explorations":
			columns := []string{"id", "title", "duration_seconds"}
			if name == "Trace" {
				columns = append(columns, "explorer_id")
			}
			markdownTable(&out, value, columns)
		case "recent_top10_derived_cognitions":
			context.recent(&out, value)
		default:
			markdownValue(&out, value, 3)
		}
		if i == 0 {
			graphKey := "graph"
			if name == "Trace" {
				graphKey = "related_subgraph"
			}
			context.graphSummary(&out, markdownField(&root, graphKey))
		}
	}
	return []byte(out.String()), nil
}

// The lookup contains only data from the same validated snapshot as the projection.
// It adds presentation labels without expanding the projection's node membership.
type markdownContext struct {
	cognitions   map[string]domain.Cognition
	explorations map[string]domain.Exploration
}

func newMarkdownContext(all loaded) markdownContext {
	context := markdownContext{cognitions: map[string]domain.Cognition{}, explorations: map[string]domain.Exploration{}}
	for _, cognition := range all.Cognitions {
		context.cognitions[cognition.ID] = cognition
	}
	for _, exploration := range all.Explorations {
		context.explorations[exploration.ID] = exploration
	}
	return context
}

func markdownLabel(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func markdownField(node *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return &yaml.Node{Kind: yaml.ScalarNode}
}

func markdownWithout(node *yaml.Node, keys ...string) *yaml.Node {
	copy := *node
	copy.Content = nil
	for i := 0; i < len(node.Content); i += 2 {
		skip := false
		for _, key := range keys {
			if node.Content[i].Value == key {
				skip = true
				break
			}
		}
		if !skip {
			copy.Content = append(copy.Content, node.Content[i], node.Content[i+1])
		}
	}
	return &copy
}

func markdownAppend(node *yaml.Node, key, value string) {
	node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Value: value})
}

func markdownTask(out *strings.Builder, node *yaml.Node) {
	fmt.Fprintf(out, "> %s\n\n", markdownCode(markdownField(node, "id").Value))
	out.WriteString("**Purpose**\n\n")
	markdownProse(out, markdownField(node, "purpose").Value)
	markdownMetadata(out, markdownWithout(node, "id", "purpose"))
}

func markdownMetadata(out *strings.Builder, node *yaml.Node) {
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		text := markdownCode(value.Value)
		if value.Kind == yaml.SequenceNode {
			text = markdownIDs(value)
		}
		fmt.Fprintf(out, "- **%s:** %s\n", markdownLabel(key), text)
	}
	if len(node.Content) > 0 {
		out.WriteString("\n")
	}
}

func markdownEntityHeading(out *strings.Builder, level int, id, title string) {
	text := markdownCode(id)
	if title != "" {
		text += " - " + markdownEscape(strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(title))
	}
	markdownHeading(out, level, text)
}

func (context markdownContext) cognition(out *strings.Builder, node *yaml.Node, level int) {
	id := markdownField(node, "id").Value
	cognition := context.cognitions[id]
	markdownEntityHeading(out, level, id, cognition.Title)
	metadata := markdownWithout(node, "id", "content")
	if cognition.Origin != "" {
		markdownAppend(metadata, "origin", cognition.Origin)
	}
	markdownMetadata(out, metadata)
	out.WriteString("**Content**\n\n")
	markdownProse(out, markdownField(node, "content").Value)
}

func (context markdownContext) taskCognitions(out *strings.Builder, graph *yaml.Node) bool {
	var roots []*yaml.Node
	for _, cognition := range markdownField(graph, "cognitions").Content {
		id := markdownField(cognition, "id").Value
		if context.cognitions[id].Origin == "task" {
			roots = append(roots, cognition)
		}
	}
	if len(roots) == 0 {
		return false
	}
	markdownHeading(out, 3, "Task-Init Cognitions")
	for _, cognition := range roots {
		context.cognition(out, cognition, 4)
	}
	return true
}

func (context markdownContext) graphSummary(out *strings.Builder, graph *yaml.Node) {
	markdownHeading(out, 2, "Graph")
	markdownHeading(out, 3, "Cognitions")
	var rows strings.Builder
	for _, node := range markdownField(graph, "cognitions").Content {
		id := markdownField(node, "id").Value
		cognition := context.cognitions[id]
		fmt.Fprintf(&rows, "%s: %s", id, markdownSingleLine(cognition.Title))
		if cognition.Origin == "exploration" {
			fmt.Fprintf(&rows, " <- %s", value(cognition.OriginRef))
		}
		rows.WriteString("\n")
	}
	markdownTextBlock(out, rows.String())
	markdownHeading(out, 3, "Explorations")
	rows.Reset()
	for _, node := range markdownField(graph, "explorations").Content {
		id := markdownField(node, "id").Value
		fmt.Fprintf(&rows, "%s: (%s) %s <- [", id, markdownField(node, "status").Value, markdownSingleLine(context.explorations[id].Title))
		for i, source := range markdownField(node, "sources").Content {
			if i > 0 {
				rows.WriteString(", ")
			}
			fmt.Fprintf(&rows, "'%s'", source.Value)
		}
		rows.WriteString("]\n")
	}
	markdownTextBlock(out, rows.String())
}

func markdownSingleLine(value string) string {
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ").Replace(value)
}

func markdownTextBlock(out *strings.Builder, content string) {
	if content == "" {
		content = "[]\n"
	}
	fence := "```"
	for strings.Contains(content, fence) {
		fence += "`"
	}
	fmt.Fprintf(out, "%stext\n%s%s\n\n", fence, content, fence)
}

func (context markdownContext) graph(out *strings.Builder, node *yaml.Node) {
	hasTaskCognitions := context.taskCognitions(out, node)
	outputs := map[string][]*yaml.Node{}
	for _, cognition := range markdownField(node, "cognitions").Content {
		id := markdownField(cognition, "id").Value
		if source := context.cognitions[id]; source.Origin == "exploration" {
			outputs[value(source.OriginRef)] = append(outputs[value(source.OriginRef)], cognition)
		}
	}
	explorations := markdownField(node, "explorations")
	if len(explorations.Content) == 0 && !hasTaskCognitions {
		out.WriteString("`[]`\n\n")
	}
	for _, exploration := range explorations.Content {
		id := markdownField(exploration, "id").Value
		entity := context.explorations[id]
		markdownEntityHeading(out, 3, id, entity.Title)
		metadata := markdownWithout(exploration, "id", "content", "hints")
		if entity.CreatedAt != "" {
			markdownAppend(metadata, "created_at", entity.CreatedAt)
		}
		markdownMetadata(out, metadata)
		out.WriteString("**Content**\n\n")
		markdownProse(out, markdownField(exploration, "content").Value)
		for _, cognition := range outputs[id] {
			context.cognition(out, cognition, 4)
		}
		for _, hint := range markdownField(exploration, "hints").Content {
			context.cognition(out, hint, 4)
		}
	}
}

func (context markdownContext) recent(out *strings.Builder, ids *yaml.Node) {
	rows := &yaml.Node{Kind: yaml.SequenceNode}
	for _, id := range ids.Content {
		cognition := context.cognitions[id.Value]
		row := &yaml.Node{Kind: yaml.MappingNode}
		markdownAppend(row, "id", id.Value)
		markdownAppend(row, "title", cognition.Title)
		markdownAppend(row, "created_at", cognition.CreatedAt)
		rows.Content = append(rows.Content, row)
	}
	markdownTable(out, rows, []string{"id", "title", "created_at"})
}

func markdownHeading(out *strings.Builder, level int, title string) {
	fmt.Fprintf(out, "%s %s\n\n", strings.Repeat("#", level), title)
}

func markdownValue(out *strings.Builder, node *yaml.Node, level int) {
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i].Value, node.Content[i+1]
			if value.Kind == yaml.ScalarNode {
				if key == "content" || key == "purpose" || key == "note" || key == "outcome_reason" || strings.ContainsAny(value.Value, "\r\n") {
					fmt.Fprintf(out, "**%s**\n\n", markdownLabel(key))
					markdownProse(out, value.Value)
				} else {
					fmt.Fprintf(out, "- **%s:** %s\n", markdownLabel(key), markdownCode(value.Value))
					// Keep consecutive metadata fields in a single list.
					if i+2 == len(node.Content) || isMarkdownBlock(node.Content[i+2].Value, node.Content[i+3]) {
						out.WriteString("\n")
					}
				}
			} else if isMarkdownScalarList(value) {
				fmt.Fprintf(out, "- **%s:** %s\n\n", markdownLabel(key), markdownIDs(value))
			} else {
				markdownHeading(out, level, key)
				markdownValue(out, value, level+1)
			}
		}
	case yaml.SequenceNode:
		if len(node.Content) == 0 {
			out.WriteString("`[]`\n\n")
			return
		}
		if isMarkdownScalarList(node) {
			out.WriteString(markdownIDs(node) + "\n\n")
			return
		}
		for _, item := range node.Content {
			title := ""
			body := *item
			body.Content = nil
			for i := 0; i < len(item.Content); i += 2 {
				if item.Content[i].Value == "id" {
					title = markdownCode(item.Content[i+1].Value)
				} else {
					body.Content = append(body.Content, item.Content[i], item.Content[i+1])
				}
			}
			if title != "" {
				markdownHeading(out, level, title)
			}
			markdownValue(out, &body, level+1)
		}
	default:
		out.WriteString(markdownCode(node.Value) + "\n\n")
	}
}

func isMarkdownBlock(key string, value *yaml.Node) bool {
	return value.Kind != yaml.ScalarNode || key == "content" || key == "purpose" || key == "note" || key == "outcome_reason" || strings.ContainsAny(value.Value, "\r\n")
}

func isMarkdownScalarList(node *yaml.Node) bool {
	if node.Kind != yaml.SequenceNode {
		return false
	}
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode {
			return false
		}
	}
	return true
}

func markdownIDs(node *yaml.Node) string {
	if len(node.Content) == 0 {
		return "`[]`"
	}
	items := make([]string, len(node.Content))
	for i, item := range node.Content {
		items[i] = markdownCode(item.Value)
	}
	return strings.Join(items, ", ")
}

func markdownTable(out *strings.Builder, list *yaml.Node, columns []string) {
	for _, column := range columns {
		fmt.Fprintf(out, "| %s ", column)
	}
	out.WriteString("|\n")
	for range columns {
		out.WriteString("| --- ")
	}
	out.WriteString("|\n")
	for _, item := range list.Content {
		for _, column := range columns {
			cell := markdownCode(markdownField(item, column).Value)
			fmt.Fprintf(out, "| %s ", strings.ReplaceAll(cell, "|", `\|`))
		}
		out.WriteString("|\n")
	}
	out.WriteString("\n")
}

func markdownCode(value string) string {
	value = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(value)
	fence := "`"
	for strings.Contains(value, fence) {
		fence += "`"
	}
	if strings.HasPrefix(value, "`") || strings.HasSuffix(value, "`") || strings.HasPrefix(value, " ") || strings.HasSuffix(value, " ") {
		value = " " + value + " "
	}
	return fence + value + fence
}

func markdownProse(out *strings.Builder, value string) {
	value = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(value)
	// Treat entity prose as text; its own headings or HTML must not change the View layout.

	lines := strings.Split(value, "\n")
	for i, line := range lines {
		if line == "" {
			out.WriteString(">\n")
		} else {
			fmt.Fprintf(out, "> %s", markdownEscape(line))
			if i+1 < len(lines) && lines[i+1] != "" {
				out.WriteString(`\`)
			}
			out.WriteString("\n")
		}
	}
	out.WriteString("\n")
}

func markdownEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "#", "\\#", "~", "\\~").Replace(value)
}
