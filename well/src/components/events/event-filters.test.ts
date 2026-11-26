import { expect, it } from "vitest";
import type { PublicEvent } from "../../api/dto";
import { buildAgentIndex } from "./agent-identity";
import { EMPTY_EVENT_FILTERS, hasEventFilters, isAgentMessageEvent, matchesEventFilters, namedAgentRole, type EventFilters } from "./event-filters";

const worker = { role: "sub", name: "Worker", instance_id: "tool:call-a", tool_use_id: "call-a" };
function event(seq: number, payload: PublicEvent["payload"], runtime = "runtime-alpha", type = "agent.text"): PublicEvent {
  return { seq: String(seq), type, runtime_id: runtime, source: "agent", timestamp: "2026-11-20T00:00:00Z", payload };
}
const output = event(1, { agent: worker, text: "Geometry discovered" });
const result = event(2, { agent: { role: "main", name: "Coordinator" }, spawned_agent: { ...worker, agent_id: "actual-worker-a" } }, "runtime-alpha", "agent.tool_result");
const events = [output, result];
const index = buildAgentIndex(events);
const matches = (item: PublicEvent, filters: Partial<EventFilters>) => matchesEventFilters(item, { ...EMPTY_EVENT_FILTERS, ...filters }, index);

it("Agent Only includes exactly the five message types, for both main and subagents", () => {
  for (const type of ["agent.text", "agent.tool_call", "agent.tool_result", "agent.interrupted", "agent.result"]) {
    for (const agent of [worker, { role: "main", name: "Coordinator" }]) {
      expect(isAgentMessageEvent(event(1, { agent }, "runtime-alpha", type))).toBe(true);
    }
  }
  for (const type of ["agent.status", "agent.api_retry", "agent.user_message", "agent.task_started", "agent.task_progress", "agent.task_updated", "agent.task_notification", "agent.future_event", "runtime.finished", "instruction.dispatched", "norns.lifecycle", "diagnostic"]) {
    expect(isAgentMessageEvent({ type })).toBe(false);
  }
});

it("combines content, runtime, type, named role and agent ID with AND", () => {
  const filters = { text: " GEOMETRY ", runtimeId: " ALPHA ", eventType: "agent.text", agentRole: namedAgentRole(worker), agentId: " WORKER-A " };
  expect(matches(output, filters)).toBe(true);
  for (const [key, value] of Object.entries({ text: "missing", runtimeId: "beta", eventType: "agent.tool_result", agentRole: "kind:main", agentId: "worker-b" })) {
    expect(matches(output, { ...filters, [key]: value })).toBe(false);
  }
  expect(matches(output, { eventType: "agent.tex" })).toBe(false);
});

it("filters named roles separately from broad main/sub kinds", () => {
  expect(matches(output, { agentRole: "kind:sub" })).toBe(true);
  expect(matches(result, { agentRole: "kind:main" })).toBe(true);
  expect(matches(output, { agentRole: namedAgentRole({ role: "sub", name: "Reviewer" }) })).toBe(false);
  expect(matches(event(3, {}), { agentRole: "kind:main" })).toBe(true);
  expect(matches(event(4, {}, "runtime-alpha", "agent.task_started"), { agentRole: "kind:unknown" })).toBe(true);
  expect(matches(event(5, {}, "runtime-alpha", "runtime.started"), { agentRole: "kind:main" })).toBe(false);
});

it("resolves late actual IDs without attributing a child to its caller or another runtime", () => {
  expect(matches(output, { agentId: "actual-worker-a" })).toBe(true);
  expect(matches(output, { agentId: "call-a" })).toBe(true);
  expect(matches(result, { agentId: "actual-worker-a" })).toBe(false);
  expect(matches(result, { agentRole: namedAgentRole(worker) })).toBe(false);
  expect(matches(event(3, { agent: worker }, "runtime-beta"), { agentId: "actual-worker-a" })).toBe(false);
  const message = event(4, { agent: { role: "main", name: "Coordinator" }, target_agent: { ...worker, agent_id: "actual-worker-a" } }, "runtime-alpha", "agent.tool_call");
  expect(matches(message, { agentId: "actual-worker-a" })).toBe(false);
});

it("retains original invocation IDs after an instance resumes", () => {
  const resumed = event(3, { agent: { ...worker, tool_use_id: "resume-call", agent_id: "actual-worker-a", task_id: "worker-task" } });
  const resumedIndex = buildAgentIndex([...events, resumed]);
  expect(matchesEventFilters(output, { ...EMPTY_EVENT_FILTERS, agentId: "call-a" }, resumedIndex)).toBe(true);
  expect(matchesEventFilters(output, { ...EMPTY_EVENT_FILTERS, agentId: "worker-task" }, resumedIndex)).toBe(true);
});

it("treats blank inputs as inactive filters", () => {
  const blank = { ...EMPTY_EVENT_FILTERS, text: "  ", runtimeId: " ", agentId: "\t" };
  expect(hasEventFilters(blank)).toBe(false);
  expect(matchesEventFilters(output, blank, index)).toBe(true);
  expect(hasEventFilters({ ...blank, eventType: "agent.text" })).toBe(true);
});
