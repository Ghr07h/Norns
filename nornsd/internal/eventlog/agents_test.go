package eventlog

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
	"github.com/Ghr07h/norns/nornsd/internal/root"
	"github.com/google/uuid"
)

func TestSameRoleInstancesKeepExplicitIdentityAcrossInterleavedOutputAndReplay(t *testing.T) {
	ctx := context.Background()
	store, sid, rid := projectionStore(t)
	path := t.TempDir()
	logger := New(store, path, sid, rid)
	frames := []string{
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call-a","name":"Agent","input":{"subagent_type":"worker"}},{"type":"tool_use","id":"call-b","name":"Agent","input":{"subagent_type":"worker"}}]}}`,
		`{"type":"assistant","parent_tool_use_id":"call-b","message":{"content":[{"type":"text","text":"B first"},{"type":"tool_use","id":"read-b","name":"Read","input":{}}]}}`,
		`{"type":"assistant","parent_tool_use_id":"call-a","message":{"content":[{"type":"text","text":"A first"},{"type":"tool_use","id":"read-a","name":"Read","input":{}}]}}`,
		// Tool results sometimes omit routing; inherit the owner of that tool call.
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"read-b","content":"B read"}]}}`,
		`{"type":"user","tool_use_result":{"agentId":"agent-b","status":"async_launched"},"message":{"content":[{"type":"tool_result","tool_use_id":"call-b","content":"launched"}]}}`,
		`{"type":"system","subtype":"task_started","task_id":"task-b","tool_use_id":"call-b","task_type":"local_agent"}`,
		`{"type":"system","subtype":"task_notification","task_id":"task-b","status":"completed"}`,
		`{"type":"assistant","agent_id":"agent-b","message":{"content":[{"type":"text","text":"B after metadata"}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"call-a","content":[{"type":"text","text":"done\nagentId: agent-a (for resuming this agent)"}]}]}}`,
		`{"type":"assistant","parent_tool_use_id":"call-a","message":{"content":[{"type":"text","text":"A after metadata"}]}}`,
		// Same role without an instance identifier must not borrow either instance.
		`{"type":"assistant","subagent_type":"worker","message":{"content":[{"type":"text","text":"unknown worker"}]}}`,
		// A Bash result containing similar text is not an Agent result.
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"bash","name":"Bash","input":{}}]}}`,
		`{"type":"user","tool_use_result":{"agentId":"not-an-agent"},"message":{"content":[{"type":"tool_result","tool_use_id":"bash","content":"agentId: not-an-agent"}]}}`,
	}
	for _, raw := range frames {
		if _, err := logger.Record(ctx, "stdout", "agent_message", json.RawMessage(raw), time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	events, err := store.ListEvents(ctx, sid, 100, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	texts := map[string]agentIdentity{}
	for _, event := range events {
		var p struct {
			Text    string         `json:"text"`
			Agent   agentIdentity  `json:"agent"`
			Spawned *agentIdentity `json:"spawned_agent"`
			ToolID  string         `json:"tool_call_id"`
			Parent  string         `json:"parent_tool_use_id"`
		}
		if err := json.Unmarshal(event.Payload, &p); err != nil {
			t.Fatal(err)
		}
		if p.Text != "" {
			texts[p.Text] = p.Agent
		}
		if p.Text == "B first" && p.Parent != "call-b" {
			t.Fatalf("lost provenance: %s", event.Payload)
		}
		if event.Type == "agent.tool_result" {
			switch p.ToolID {
			case "read-b":
				if p.Agent.InstanceID != "tool:call-b" || p.Agent.Name != "Worker" {
					t.Fatalf("wrong Read caller: %s", event.Payload)
				}
			case "call-b":
				if p.Agent.Role != "main" || p.Spawned == nil || p.Spawned.AgentID != "agent-b" {
					t.Fatalf("wrong Agent caller/child: %s", event.Payload)
				}
			case "call-a":
				if p.Spawned == nil || p.Spawned.AgentID != "agent-a" {
					t.Fatalf("lost text agentId: %s", event.Payload)
				}
			case "bash":
				if p.Spawned != nil {
					t.Fatalf("Bash impersonated an agent: %s", event.Payload)
				}
			}
		}
		if event.Type == "agent.task_notification" && (p.Agent.InstanceID != "tool:call-b" || p.Agent.AgentID != "agent-b" || p.Agent.TaskID != "task-b") {
			t.Fatalf("lost task correlation: %s", event.Payload)
		}
	}
	for text, id := range map[string]string{"B first": "tool:call-b", "A first": "tool:call-a", "B after metadata": "tool:call-b", "A after metadata": "tool:call-a"} {
		if texts[text].InstanceID != id || texts[text].Name != "Worker" {
			t.Fatalf("%s: %#v", text, texts[text])
		}
	}
	if texts["B after metadata"].AgentID != "agent-b" || texts["A after metadata"].AgentID != "agent-a" {
		t.Fatal("actual agentId not associated")
	}
	if unknown := texts["unknown worker"]; unknown.Role != "sub" || unknown.InstanceID != "" {
		t.Fatalf("guessed identity: %#v", unknown)
	}
	for n := 0; n < 2; n++ {
		if err := ReplayRaw(ctx, store, path, sid); err != nil {
			t.Fatal(err)
		}
		replayed, err := store.ListEvents(ctx, sid, 100, nil, nil)
		if err != nil || !reflect.DeepEqual(events, replayed) {
			t.Fatalf("replay changed attribution: %v", err)
		}
	}
	data, err := os.ReadFile(filepath.Join(root.SubjectDataPath(path), "logs", "readable.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var readable []domain.Event
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var event domain.Event
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		readable = append(readable, event)
	}
	if !reflect.DeepEqual(events, readable) {
		t.Fatal("readable lost instance payloads")
	}
}

func TestAgentIdentityUsesOnlyExplicitAliases(t *testing.T) {
	a := newAgentTracker()
	before := a.frameAgent(agentEnvelope{Type: "assistant", ParentToolUseID: "call", SubagentType: "worker"})
	returned := a.completedAgent("call", map[string]any{"agentId": "actual"}, nil)
	resumed := a.spawned("resume-call", "Agent", map[string]any{"resume": "actual"})
	after := a.frameAgent(agentEnvelope{Type: "assistant", ParentToolUseID: "resume-call"})
	if returned == nil || resumed == nil || before.InstanceID != after.InstanceID || after.AgentID != "actual" {
		t.Fatalf("resume lost identity: %#v %#v %#v", returned, resumed, after)
	}
	a.toolOwners["worker-bash"] = after
	bashStarted := a.frameAgent(agentEnvelope{Type: "system", Subtype: "task_started", TaskType: "local_bash", ToolUseID: "worker-bash", TaskID: "bash-task"})
	delete(a.toolOwners, "worker-bash")
	bashFinished := a.frameAgent(agentEnvelope{Type: "system", Subtype: "task_notification", TaskID: "bash-task"})
	if bashStarted.AgentID != "actual" || bashFinished.AgentID != "actual" || bashStarted.InstanceID != before.InstanceID {
		t.Fatal("Bash background task lost worker caller")
	}
	unknown := a.frameAgent(agentEnvelope{Type: "assistant", SubagentType: "worker"})
	if unknown.InstanceID != "" {
		t.Fatalf("role-name association: %#v", unknown)
	}
	task := a.frameAgent(agentEnvelope{Type: "system", Subtype: "task_notification", TaskID: "unknown-task"})
	if task.Role != "unknown" {
		t.Fatalf("unknown background task assigned to main: %#v", task)
	}
	other := newAgentTracker().frameAgent(agentEnvelope{Type: "assistant", ParentToolUseID: "call"})
	if other.AgentID != "" || other.Source != "" {
		t.Fatalf("tracker leaked identity: %#v", other)
	}
	// Independently arriving identifiers merge only when an explicit frame ties them.
	independent := a.frameAgent(agentEnvelope{Type: "assistant", AgentID: "late-agent"})
	a.frameAgent(agentEnvelope{Type: "assistant", ParentToolUseID: "late-call", SubagentType: "worker"})
	joined := a.completedAgent("late-call", map[string]any{"agentId": "late-agent"}, nil)
	if joined == nil || a.current(independent).InstanceID != joined.InstanceID {
		t.Fatal("explicit aliases were not joined")
	}
	clone := a.clone()
	clone.frameAgent(agentEnvelope{Type: "assistant", ParentToolUseID: "call", AgentID: "changed"})
	if a.current(before).AgentID != "actual" {
		t.Fatal("uncommitted attribution leaked")
	}
}

func TestRuntimeAgentIdentityDoesNotLeakAcrossRawReplay(t *testing.T) {
	ctx := context.Background()
	store, sid, rid := projectionStore(t)
	path := t.TempDir()
	first := New(store, path, sid, rid)
	raw := json.RawMessage(`{"type":"assistant","parent_tool_use_id":"shared","subagent_type":"worker","agent_id":"old-agent","message":{"content":[{"type":"text","text":"old"}]}}`)
	if _, err := first.Record(ctx, "stdout", "agent_message", raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RequestStop(ctx, sid, "test", time.Now()); err != nil {
		t.Fatal(err)
	}
	secondID := uuid.NewString()
	if _, err := store.SubmitInstruction(ctx, sid, secondID, "next", false, time.Now()); err != nil {
		t.Fatal(err)
	}
	second := New(store, path, sid, secondID)
	raw = json.RawMessage(`{"type":"assistant","parent_tool_use_id":"shared","message":{"content":[{"type":"text","text":"new"}]}}`)
	if _, err := second.Record(ctx, "stdout", "agent_message", raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListEvents(ctx, sid, 100, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReplayRaw(ctx, store, path, sid); err != nil {
		t.Fatal(err)
	}
	replayed, err := store.ListEvents(ctx, sid, 100, nil, nil)
	if err != nil || !reflect.DeepEqual(events, replayed) {
		t.Fatalf("cross-Runtime replay changed identities: %v", err)
	}
	for _, event := range replayed {
		if event.RuntimeID == nil || *event.RuntimeID != secondID || event.Type != "agent.text" {
			continue
		}
		var p struct {
			Agent agentIdentity `json:"agent"`
		}
		if err := json.Unmarshal(event.Payload, &p); err != nil {
			t.Fatal(err)
		}
		if p.Agent.AgentID != "" || p.Agent.Source != "" {
			t.Fatalf("old Runtime identity leaked: %s", event.Payload)
		}
	}
}

func TestAgentResultMetadataDoesNotApplyToMultipleToolResults(t *testing.T) {
	ctx := context.Background()
	store, sid, rid := projectionStore(t)
	logger := New(store, t.TempDir(), sid, rid)
	for _, raw := range []string{
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"one","name":"Agent","input":{"subagent_type":"worker"}},{"type":"tool_use","id":"two","name":"Agent","input":{"subagent_type":"worker"}}]}}`,
		`{"type":"user","tool_use_result":{"agentId":"ambiguous"},"message":{"content":[{"type":"tool_result","tool_use_id":"one","content":"done\nagentId: real-one (resume)"},{"type":"tool_result","tool_use_id":"two","content":"done"}]}}`,
	} {
		if _, err := logger.Record(ctx, "stdout", "agent_message", json.RawMessage(raw), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	events, err := store.ListEvents(ctx, sid, 100, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type != "agent.tool_result" {
			continue
		}
		var p struct {
			ToolID  string        `json:"tool_call_id"`
			Spawned agentIdentity `json:"spawned_agent"`
		}
		if err := json.Unmarshal(event.Payload, &p); err != nil {
			t.Fatal(err)
		}
		if p.Spawned.AgentID == "ambiguous" || p.ToolID == "one" && p.Spawned.AgentID != "real-one" || p.ToolID == "two" && p.Spawned.AgentID != "" {
			t.Fatalf("ambiguous ID attribution: %s", event.Payload)
		}
	}
}

// Frames from fresh real 2.1.199 foreground/background executions on ECS.
// Identifiers and temporary paths were replaced consistently, not fabricated.
func TestRealClaude2199AgentIdentityFramesProjectAndReplay(t *testing.T) {
	data, err := os.ReadFile("testdata/cc2199-agent-identity.json")
	if err != nil {
		t.Fatal(err)
	}
	var scenarios []struct {
		Name   string
		Frames []json.RawMessage
	}
	if err := json.Unmarshal(data, &scenarios); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			ctx := context.Background()
			store, sid, rid := projectionStore(t)
			path := t.TempDir()
			logger := New(store, path, sid, rid)
			for _, raw := range scenario.Frames {
				if _, err := logger.Record(ctx, "stdout", "agent_message", raw, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			events, err := store.ListEvents(ctx, sid, 1000, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			identities := map[string]bool{}
			childOutputs := map[string]bool{}
			resumed := false
			for _, event := range events {
				var p struct {
					Agent    agentIdentity  `json:"agent"`
					Spawned  *agentIdentity `json:"spawned_agent"`
					Target   *agentIdentity `json:"target_agent"`
					Text     string         `json:"text"`
					Parent   string         `json:"parent_tool_use_id"`
					TaskType string         `json:"task_type"`
					Status   string         `json:"subagent_status"`
				}
				if err := json.Unmarshal(event.Payload, &p); err != nil {
					t.Fatal(err)
				}
				if p.Spawned != nil && p.Spawned.AgentID != "" {
					identities[p.Spawned.AgentID] = true
					if event.Type == "agent.tool_result" {
						want := "completed"
						if scenario.Name == "background" {
							want = "running"
						}
						if p.Status != want {
							t.Fatalf("real %s result status = %q, want %q: %s", scenario.Name, p.Status, want, event.Payload)
						}
					}
				}
				if event.Type == "agent.text" && p.Parent != "" {
					if p.Agent.Role != "sub" || p.Agent.Name != "Identity-Worker" || p.Agent.InstanceID == "" {
						t.Fatalf("misattributed child output: %s", event.Payload)
					}
					childOutputs[p.Agent.InstanceID] = true
				}
				if p.TaskType == "local_bash" && (p.Agent.Role != "sub" || p.Agent.Name != "Identity-Worker") {
					t.Fatalf("lost worker Bash task owner: %s", event.Payload)
				}
				if p.Text == "IDENTITY_RESUMED" {
					if p.Agent.AgentID != "agent-1" {
						t.Fatalf("resumed worker identity changed: %s", event.Payload)
					}
					resumed = true
				}
				if p.Target != nil && p.Target.AgentID != "agent-1" {
					t.Fatalf("SendMessage target was not original worker: %s", event.Payload)
				}
				if p.Target != nil && event.Type == "agent.tool_result" && p.Status != "running" {
					t.Fatalf("real resume status = %q", p.Status)
				}
			}
			if len(identities) != 2 || len(childOutputs) != 2 {
				t.Fatalf("expected exactly two distinct workers: ids=%v outputs=%v", identities, childOutputs)
			}
			if scenario.Name == "foreground" && !resumed {
				t.Fatal("missing resumed worker output")
			}
			for n := 0; n < 2; n++ {
				if err := ReplayRaw(ctx, store, path, sid); err != nil {
					t.Fatal(err)
				}
				replayed, err := store.ListEvents(ctx, sid, 1000, nil, nil)
				if err != nil || !reflect.DeepEqual(events, replayed) {
					t.Fatalf("real frame replay changed attribution: %v", err)
				}
			}
		})
	}
}

func TestSubagentResultLifecycleIsPreserved(t *testing.T) {
	store, sid, rid := projectionStore(t)
	logger := New(store, t.TempDir(), sid, rid)
	frames := []string{
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"sync","name":"Agent","input":{"subagent_type":"worker"}},{"type":"tool_use","id":"async","name":"Agent","input":{"subagent_type":"worker"}},{"type":"tool_use","id":"failed","name":"Agent","input":{"subagent_type":"worker"}},{"type":"tool_use","id":"bash","name":"Bash","input":{}}]}}`,
		`{"type":"user","tool_use_result":{"agentId":"a-sync","status":"completed"},"message":{"content":[{"type":"tool_result","tool_use_id":"sync","content":"finished"}]}}`,
		`{"type":"user","tool_use_result":{"agentId":"a-async","status":"async_launched","isAsync":true},"message":{"content":[{"type":"tool_result","tool_use_id":"async","content":"launched"}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"failed","is_error":true,"content":"failed"}]}}`,
		`{"type":"user","tool_use_result":{"status":"completed"},"message":{"content":[{"type":"tool_result","tool_use_id":"bash","content":"done"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"resume","name":"SendMessage","input":{"to":"a-sync","message":"continue"}}]}}`,
		`{"type":"user","tool_use_result":{"resumedAgentId":"a-sync","success":true},"message":{"content":[{"type":"tool_result","tool_use_id":"resume","content":"resumed"}]}}`,
	}
	for _, frame := range frames {
		if _, err := logger.Record(context.Background(), "stdout", "agent_message", json.RawMessage(frame), time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	events, err := store.ListEvents(context.Background(), sid, 100, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, event := range events {
		if event.Type != "agent.tool_result" {
			continue
		}
		var p struct {
			Tool   string `json:"tool_call_id"`
			Status string `json:"subagent_status"`
		}
		if err := json.Unmarshal(event.Payload, &p); err != nil {
			t.Fatal(err)
		}
		got[p.Tool] = p.Status
	}
	want := map[string]string{"sync": "completed", "async": "running", "failed": "failed", "bash": "", "resume": "running"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lifecycle = %#v, want %#v", got, want)
	}
}
