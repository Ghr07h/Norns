import { describe, expect, it } from "vitest";
import type { PublicEvent } from "../../api/dto";
import { agentContextEvent, agentInstanceID, buildAgentIndex, resolveAgent } from "./agent-identity";
import { eventAgentLabel } from "./EventRow";

function event(runtime: string, payload: Record<string, unknown>): PublicEvent {
  return { seq: "1", type: "agent.text", payload, source: "agent", runtime_id: runtime, timestamp: "2026-11-20T00:00:00Z" };
}
const first = { role: "sub", name: "Worker", instance_id: "tool:call-a", tool_use_id: "call-a" };
const second = { role: "sub", name: "Worker", instance_id: "tool:call-b", tool_use_id: "call-b" };

describe("Subagent instance identity", () => {
  it("uses the returned agentId for earlier interleaved messages of the same role", () => {
    const a = event("runtime", { agent: first });
    const b = event("runtime", { agent: second });
    const returned = event("runtime", { agent: { role: "main", name: "Coordinator" }, spawned_agent: { ...first, agent_id: "actual-a" } });
    const index = buildAgentIndex([a, b, returned]);
    expect(eventAgentLabel(a, "en", index)).toBe("Sub-agent · Worker · actual-a");
    expect(eventAgentLabel(b, "en", index)).toBe("Sub-agent · Worker · call-b");
    expect(eventAgentLabel(returned, "en", index)).toBe("Main agent · Coordinator");
  });

  it("joins independently observed aliases only when an explicit message connects them", () => {
    const a = event("runtime", { agent: first });
    const independent = event("runtime", { agent: { role: "sub", name: "Subagent", instance_id: "agent:actual-a", agent_id: "actual-a" } });
    const bridge = event("runtime", { spawned_agent: { ...first, agent_id: "actual-a" } });
    const index = buildAgentIndex([a, independent, bridge]);
    expect(eventAgentLabel(independent, "zh", index)).toBe("子智能体 · Worker · actual-a");
    expect(agentInstanceID(resolveAgent(a, first, index)!)).toBe("actual-a");
  });

  it("does not mix identical identifiers across Runtime scopes or guess from role names", () => {
    const prior = event("prior", { agent: { ...first, agent_id: "old-agent" } });
    const current = event("current", { agent: first });
    const unknown = event("current", { agent: { role: "sub", name: "Worker" } });
    const index = buildAgentIndex([prior, current, unknown]);
    expect(eventAgentLabel(current, "en", index)).toBe("Sub-agent · Worker · call-a");
    expect(eventAgentLabel(unknown, "zh", index)).toBe("子智能体 · Worker · 实例未知");
    expect(eventAgentLabel({ ...unknown, type: "agent.task_notification", payload: { task_id: "missing" } }, "en")).toBe("Unknown origin · Background task");
  });
});

it("recovers task ownership only through an explicit, unambiguous resume lineage", () => {
  const prior = { ...event("prior", { agent: { ...first, task_id: "task-a", agent_id: "actual-a" } }), seq: "1" };
  const notification = { ...event("current", { agent: { role: "unknown", name: "Background task" }, task_id: "task-a" }), type: "agent.task_notification", seq: "2" };
  const resume = { ...event("current", { resume_session_id: "prior" }), type: "runtime.session_resumed", seq: "3" };
  expect(eventAgentLabel(notification, "en", buildAgentIndex([notification], [prior]))).toBe("Unknown origin · Background task");
  expect(eventAgentLabel(notification, "en", buildAgentIndex([notification], [prior, resume]))).toBe("Sub-agent · Worker · actual-a");
  const unrelated = { ...resume, payload: { resume_session_id: "unrelated" } };
  expect(eventAgentLabel(notification, "en", buildAgentIndex([notification], [prior, unrelated]))).toBe("Unknown origin · Background task");
  expect(eventAgentLabel(notification, "en", buildAgentIndex([notification], [prior, resume, unrelated]))).toBe("Unknown origin · Background task");
  const middle = { ...resume, runtime_id: "middle" };
  const chain = { ...resume, payload: { resume_session_id: "middle" } };
  expect(eventAgentLabel(notification, "en", buildAgentIndex([notification], [prior, middle, chain]))).toBe("Sub-agent · Worker · actual-a");
  const cycle = { ...resume, runtime_id: "middle", payload: { resume_session_id: "current" } };
  expect(eventAgentLabel(notification, "en", buildAgentIndex([notification], [prior, cycle, chain]))).toBe("Unknown origin · Background task");
});

it("keeps current Runtime task ownership ahead of historical tasks with the same ID", () => {
  const prior = event("prior", { agent: { ...first, task_id: "same-task", agent_id: "old-agent" } });
  const own = event("current", { agent: { ...second, task_id: "same-task", agent_id: "current-agent" } });
  const notification = { ...event("current", { task_id: "same-task" }), type: "agent.task_notification" };
  const resume = { ...event("current", { resume_session_id: "prior" }), type: "runtime.session_resumed" };
  expect(eventAgentLabel(notification, "en", buildAgentIndex([own, notification], [prior, resume]))).toBe("Sub-agent · Worker · current-agent");
});

it("retains compact attribution metadata without other Runtime message bodies", () => {
  const source = event("prior", { agent: { ...first, task_id: "task-a" }, text: "private message", content: "private tool result", input: { prompt: "private instructions" }, usage: { input_tokens: 12 } });
  const compact = agentContextEvent(source);
  expect(compact?.payload).toEqual({ agent: { ...first, task_id: "task-a" } });
  expect(JSON.stringify(compact)).not.toContain("private");
  expect(agentContextEvent({ ...source, type: "runtime.finished" })).toBeNull();
});

it("keeps SendMessage's caller separate from the explicitly resumed target", () => {
  const output = event("runtime", { agent: first });
  const resumed = event("runtime", { agent: { role: "main", name: "Coordinator" }, target_agent: { ...first, agent_id: "resumed-actual-id" } });
  const index = buildAgentIndex([output, resumed]);
  expect(eventAgentLabel(output, "en", index)).toBe("Sub-agent · Worker · resumed-actual-id");
  expect(eventAgentLabel(resumed, "en", index)).toBe("Main agent · Coordinator");
});
