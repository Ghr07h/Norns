import { expect, it } from "vitest";
import type { PublicEvent } from "../../api/dto";
import { buildAgentConversations } from "./agent-conversations";
import resumedTasks from "./testdata/resumed-task-notifications.json";

const main = { role: "main", name: "Coordinator" };
const worker = (id: string) => ({ role: "sub", name: "Worker", instance_id: `tool:${id}`, tool_use_id: id, agent_id: `agent-${id}`, task_id: `task-${id}` });
function event(seq: number, type: string, payload: PublicEvent["payload"], runtime = "runtime"): PublicEvent {
  return { seq: String(seq), type: `agent.${type}`, runtime_id: runtime, payload, timestamp: `2026-11-20T00:00:${String(seq).padStart(2, "0")}Z`, source: "agent" };
}
const spawn = (seq: number, id: string) => event(seq, "tool_call", { agent: main, name: "Agent", tool_call_id: id, spawned_agent: worker(id) });
const result = (seq: number, id: string, status?: string) => event(seq, "tool_result", { agent: main, tool_call_id: id, spawned_agent: worker(id), subagent_status: status });
const task = (seq: number, type: string, id: string, rest = {}) => event(seq, `task_${type}`, { agent: worker(id), task_id: `task-${id}`, ...rest });

it("puts main first, then running and ended agents in first-seen time order", () => {
  const events = [spawn(1, "old"), result(2, "old", "completed"), spawn(3, "a"), spawn(4, "b"), spawn(5, "c"), result(6, "c", "failed"), event(7, "text", { agent: worker("a"), text: "A output" }), event(8, "text", { agent: worker("b"), text: "other runtime" }, "other")];
  const { agents } = buildAgentConversations(events.reverse(), "runtime", "running");
  expect(agents.map((agent) => agent.key)).toEqual(["main", "tool:a", "tool:b", "tool:old", "tool:c"]);
  expect(agents.map((agent) => agent.state)).toEqual(["running", "running", "running", "ended", "ended"]);
  expect(agents[0].events.map((event) => event.seq)).toEqual(["1", "2", "3", "4", "5", "6"]);
  expect(agents[1].events.map((event) => event.seq)).toEqual(["7"]);
  expect(agents[2].events).toEqual([]);
});

it("keeps background agents running after launch and ignores their Bash completion", () => {
  const events = [spawn(1, "a"), result(2, "a", "running"), task(3, "started", "a", { task_type: "local_agent" }), task(4, "started", "a", { task_type: "local_bash", task_id: "bash-task" }), task(5, "notification", "a", { task_id: "bash-task", status: "completed" })];
  expect(buildAgentConversations(events, "runtime", "waiting").agents[1].state).toBe("running");
  events.push(task(6, "notification", "a", { status: "completed" }));
  expect(buildAgentConversations(events, "runtime", "waiting").agents[1].state).toBe("ended");
});

it("reopens the same instance after resume and handles patch status", () => {
  const events = [spawn(1, "a"), result(2, "a", "completed"), event(3, "tool_result", { agent: main, target_agent: worker("a"), subagent_status: "running" }), task(4, "started", "a", { task_type: "local_agent" })];
  expect(buildAgentConversations(events, "runtime", "running").agents[1].state).toBe("running");
  events.push(task(5, "updated", "a", { patch: { status: "failed" } }));
  const { agents } = buildAgentConversations(events, "runtime", "running");
  expect(agents).toHaveLength(2);
  expect(agents[1].state).toBe("ended");
});

it("enriches earlier IDs and preserves unknown ownership without merging same-name agents", () => {
  const events = [event(1, "text", { agent: { role: "sub", name: "Worker", instance_id: "tool:a", tool_use_id: "a" }, text: "early" }), result(2, "a", "completed"), event(3, "text", { agent: { role: "sub", name: "Worker" }, text: "unknown instance" }), event(4, "status", { agent: { role: "main", name: "Coordinator" } })];
  const { agents } = buildAgentConversations(events, "runtime", "running");
  expect(agents.map((agent) => agent.key)).toEqual(["main", "tool:a", "unknown"]);
  expect(agents[1].agent.agent_id).toBe("agent-a");
  expect(agents[1].events[0].seq).toBe("1");
  expect(agents[2].events[0].seq).toBe("3");
});

it("uses Runtime terminal state to close remaining agents, but not a main processing result", () => {
  const events = [spawn(1, "a"), event(2, "result", { agent: main, subtype: "success" })];
  expect(buildAgentConversations(events, "runtime", "waiting").agents[1].state).toBe("running");
  expect(buildAgentConversations(events, "runtime", "cancelled").agents.map((agent) => agent.state)).toEqual(["ended", "ended"]);
  const old = buildAgentConversations([spawn(1, "a"), result(2, "a")], "runtime", "running");
  expect(old.agents[1].state).toBe("unknown");
});

it("attributes observed resume cleanup notifications to three separate ended historical agents", () => {
  const current = resumedTasks.filter((event) => event.runtime_id === "resumed-runtime" && event.type.startsWith("agent."));
  const { agents } = buildAgentConversations(current, "resumed-runtime", "waiting", resumedTasks);
  expect(agents.map((agent) => agent.key)).toEqual(["main", "tool:prior-call-1", "tool:prior-call-2", "tool:prior-call-3"]);
  expect(agents.slice(1).map((agent) => agent.state)).toEqual(["ended", "ended", "ended"]);
  expect(agents.slice(1).map((agent) => agent.agent.name)).toEqual(Array(3).fill("Explorer-For-Redteam"));
  expect(agents.slice(1).map((agent) => agent.events.map((event) => event.seq))).toEqual([["1152"], ["1153"], ["1154"]]);
  expect(agents.flatMap((agent) => agent.events).every((event) => event.runtime_id === "resumed-runtime")).toBe(true);
  expect(current.every((event) => event.payload.agent?.role === "unknown")).toBe(true);
});

it("keeps inherited Bash completion separate from the lifecycle of its former owner", () => {
  const prior = [spawn(1, "a"), task(2, "started", "a", { task_type: "local_bash", task_id: "bash-task" })];
  const inherited = event(4, "task_notification", { agent: { role: "unknown", name: "Background task" }, task_id: "bash-task", status: "completed" }, "resumed");
  const resume = { ...event(5, "status", {}, "resumed"), type: "runtime.session_resumed", payload: { resume_session_id: "runtime" } };
  const { agents } = buildAgentConversations([inherited], "resumed", "running", [...prior, resume]);
  expect(agents[1].key).toBe("tool:a");
  expect(agents[1].state).toBe("unknown");
  expect(agents[1].events).toEqual([inherited]);
});
