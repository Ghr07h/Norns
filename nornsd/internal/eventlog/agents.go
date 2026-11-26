package eventlog

import (
	"encoding/json"
	"regexp"
	"strings"
)

// An instance is scoped to one Runtime. Names describe roles, never instances.
// All aliases below come from explicit CC identifiers, not role-name matching.
type agentIdentity struct {
	Role       string `json:"role"`
	Name       string `json:"name"`
	Source     string `json:"source,omitempty"`
	InstanceID string `json:"instance_id,omitempty"`
	ToolUseID  string `json:"tool_use_id,omitempty"`
	TaskID     string `json:"task_id,omitempty"`
	AgentID    string `json:"agent_id,omitempty"`
}

type agentEnvelope struct {
	Type             string `json:"type"`
	Subtype          string `json:"subtype"`
	ParentToolUseID  string `json:"parent_tool_use_id"`
	ToolUseID        string `json:"tool_use_id"`
	TaskID           string `json:"task_id"`
	AgentID          string `json:"agent_id"`
	TaskType         string `json:"task_type"`
	AttributionAgent string `json:"attributionAgent"`
	SubagentType     string `json:"subagent_type"`
}

type agentTracker struct {
	instances  map[string]agentIdentity
	aliases    map[string]string
	toolOwners map[string]agentIdentity
	taskOwners map[string]agentIdentity
	toolNames  map[string]string
}

func newAgentTracker() *agentTracker {
	return &agentTracker{instances: map[string]agentIdentity{}, aliases: map[string]string{}, toolOwners: map[string]agentIdentity{}, taskOwners: map[string]agentIdentity{}, toolNames: map[string]string{}}
}

func (a *agentTracker) clone() *agentTracker {
	out := newAgentTracker()
	if a == nil {
		return out
	}
	for k, v := range a.instances {
		out.instances[k] = v
	}
	for k, v := range a.aliases {
		out.aliases[k] = v
	}
	for k, v := range a.toolOwners {
		out.toolOwners[k] = v
	}
	for k, v := range a.taskOwners {
		out.taskOwners[k] = v
	}
	for k, v := range a.toolNames {
		out.toolNames[k] = v
	}
	return out
}

func roleAgent(source string) agentIdentity {
	if source == "" {
		return agentIdentity{Role: "sub", Name: "Subagent"}
	}
	name := source
	if index := strings.LastIndex(name, ":"); index >= 0 {
		name = name[index+1:]
	}
	parts := strings.Split(name, "-")
	for index, part := range parts {
		if part != "" {
			parts[index] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return agentIdentity{Role: "sub", Name: strings.Join(parts, "-"), Source: source}
}

func (a *agentTracker) instance(toolID, taskID, agentID, source string) agentIdentity {
	keys := []string{}
	for _, pair := range [][2]string{{"tool:", toolID}, {"task:", taskID}, {"agent:", agentID}} {
		if pair[1] != "" {
			keys = append(keys, pair[0]+pair[1])
		}
	}
	if len(keys) == 0 {
		return roleAgent(source)
	}
	var id string
	for _, key := range keys {
		if known := a.aliases[key]; known != "" {
			id = known
			break
		}
	}
	var agent agentIdentity
	if id == "" {
		id = keys[0]
		agent = roleAgent(source)
		agent.InstanceID = id
	} else {
		agent = a.instances[id]
		if source != "" {
			role := roleAgent(source)
			agent.Role, agent.Name, agent.Source = role.Role, role.Name, role.Source
		}
	}
	// An explicit frame/result may connect aliases first seen independently.
	// Union those records; a repeated role name is never sufficient to do so.
	for _, key := range keys {
		otherID := a.aliases[key]
		if otherID == "" || otherID == id {
			continue
		}
		other := a.instances[otherID]
		if agent.Source == "" && other.Source != "" {
			agent.Name, agent.Source = other.Name, other.Source
		}
		if agent.AgentID == "" {
			agent.AgentID = other.AgentID
		}
		if agent.TaskID == "" {
			agent.TaskID = other.TaskID
		}
		if agent.ToolUseID == "" {
			agent.ToolUseID = other.ToolUseID
		}
		for alias, target := range a.aliases {
			if target == otherID {
				a.aliases[alias] = id
			}
		}
		delete(a.instances, otherID)
	}
	if toolID != "" {
		agent.ToolUseID = toolID
	}
	if taskID != "" {
		agent.TaskID = taskID
	}
	if agentID != "" {
		agent.AgentID = agentID
	}
	a.instances[id] = agent
	for _, key := range keys {
		a.aliases[key] = id
	}
	return agent
}

func (a *agentTracker) frameAgent(e agentEnvelope) agentIdentity {
	source := strings.TrimSpace(e.SubagentType)
	if source == "" {
		source = strings.TrimSpace(e.AttributionAgent)
	}
	if e.Type == "system" && strings.HasPrefix(e.Subtype, "task_") {
		// Task tool_use_id identifies the task itself; parent_tool_use_id, if
		// present, identifies its caller. Never use the latter as this instance.
		known := a.aliases["tool:"+e.ToolUseID] != "" || a.aliases["task:"+e.TaskID] != "" || a.aliases["agent:"+e.AgentID] != ""
		if source != "" || e.TaskType == "local_agent" || known {
			return a.instance(e.ToolUseID, e.TaskID, e.AgentID, source)
		}
		if owner, ok := a.toolOwners[e.ToolUseID]; ok {
			if e.TaskID != "" {
				a.taskOwners[e.TaskID] = owner
			}
			return a.current(owner)
		}
		if owner, ok := a.taskOwners[e.TaskID]; ok {
			return a.current(owner)
		}
		return agentIdentity{Role: "unknown", Name: "Background task"}
	}
	if e.ParentToolUseID != "" || e.AgentID != "" || source != "" {
		return a.instance(e.ParentToolUseID, "", e.AgentID, source)
	}
	return agentIdentity{Role: "main", Name: "Coordinator"}
}

func (a *agentTracker) current(agent agentIdentity) agentIdentity {
	id := a.aliases[agent.InstanceID]
	if known, ok := a.instances[id]; ok {
		return known
	}
	return agent
}

func (a *agentTracker) spawned(id, name string, input any) *agentIdentity {
	if id == "" || (name != "Agent" && name != "Task") {
		return nil
	}
	fields, _ := input.(map[string]any)
	source, _ := fields["subagent_type"].(string)
	resumeID, _ := fields["resume"].(string)
	agent := a.instance(id, "", resumeID, strings.TrimSpace(source))
	return &agent
}

// SendMessage can resume an ended agent in CC 2.1.199. The target must be
// an actual known ID, never an equal-looking role name or a broadcast target.
func (a *agentTracker) targeted(id, name string, input any) *agentIdentity {
	if id == "" || name != "SendMessage" {
		return nil
	}
	fields, _ := input.(map[string]any)
	target, _ := fields["to"].(string)
	if target == "" {
		target, _ = fields["recipient"].(string)
	}
	if target == "" || a.aliases["agent:"+target] == "" {
		return nil
	}
	agent := a.instance(id, "", target, "")
	return &agent
}

// CC returns agentId in structured tool_use_result metadata and, in some
// responses, as an exact agentId: line in the Agent tool's result content.
var returnedAgentID = regexp.MustCompile(`(?m)^agentId:[ \t]*([A-Za-z0-9_-]+)(?:[ \t\r]|$)`)

func (a *agentTracker) completedAgent(toolID string, metadata any, content any) *agentIdentity {
	if a.aliases["tool:"+toolID] == "" && a.toolNames[toolID] != "SendMessage" {
		return nil
	}
	var agentID string
	if fields, ok := metadata.(map[string]any); ok {
		agentID, _ = fields["agentId"].(string)
		if agentID == "" {
			agentID, _ = fields["agent_id"].(string)
		}
		if a.toolNames[toolID] == "SendMessage" {
			agentID, _ = fields["resumedAgentId"].(string)
		}
	}
	if a.toolNames[toolID] == "SendMessage" {
		if agentID == "" && a.aliases["tool:"+toolID] == "" {
			return nil
		}
		agent := a.instance(toolID, "", strings.TrimSpace(agentID), "")
		return &agent
	}
	if agentID == "" {
		var text strings.Builder
		switch value := content.(type) {
		case string:
			text.WriteString(value)
		case []any:
			for _, item := range value {
				if block, ok := item.(map[string]any); ok && block["type"] == "text" {
					if part, ok := block["text"].(string); ok {
						text.WriteString(part)
						text.WriteByte('\n')
					}
				}
			}
		}
		if match := returnedAgentID.FindStringSubmatch(text.String()); len(match) > 1 {
			agentID = match[1]
		}
	}
	agent := a.instance(toolID, "", strings.TrimSpace(agentID), "")
	return &agent
}

func agentMetadata(raw json.RawMessage) any {
	var value any
	_ = json.Unmarshal(raw, &value)
	return value
}

// The initial result of a background Agent is not its completion. Keep this
// structured distinction in public events for clients that group agents.
func subagentResultStatus(metadata any, failed bool, tool string) string {
	fields, _ := metadata.(map[string]any)
	if tool == "SendMessage" {
		if id, _ := fields["resumedAgentId"].(string); id != "" && !failed {
			return "running"
		}
		return ""
	}
	if failed {
		return "failed"
	}
	if fields["isAsync"] == true || fields["status"] == "async_launched" {
		return "running"
	}
	switch fields["status"] {
	case "completed", "failed", "stopped", "cancelled":
		return fields["status"].(string)
	}
	return ""
}
