import type { PublicEvent, RuntimeState } from "../../api/dto";
import { eventAgentInfo, payloadRecord } from "./EventRow";
import { buildAgentIndex, resolveAgent, type AgentInfo } from "./agent-identity";
import { compareEventSequence } from "./use-runtime-agent-events";

export type AgentConversation = {
  key: string;
  agent: AgentInfo;
  state: "running" | "ended" | "unknown";
  firstSeen: string;
  firstSeq: string;
  events: PublicEvent[];
};
export const runtimeIsActive = (state: RuntimeState) => state === "queued" || state === "running" || state === "waiting";

function instanceKey(agent: AgentInfo): string {
  if (agent.role === "main") return "main";
  return agent.instance_id ?? (agent.agent_id ? `agent:${agent.agent_id}` : agent.tool_use_id ? `tool:${agent.tool_use_id}` : agent.task_id ? `task:${agent.task_id}` : "unknown");
}
function record(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : null;
}
const terminal = new Set(["completed", "failed", "stopped", "cancelled", "killed", "error"]);

export function buildAgentConversations(allEvents: PublicEvent[], runtimeId: string, runtimeState: RuntimeState, context: PublicEvent[] = []) {
  const events = allEvents.filter((event) => event.runtime_id === runtimeId && event.type.startsWith("agent.")).sort(compareEventSequence);
  const index = buildAgentIndex(events, context);
  const groups = new Map<string, AgentConversation>();
  const tasks = new Map<string, string>();
  groups.set("main", { key: "main", agent: { role: "main", name: "Main Agent" }, state: "running", firstSeen: "", firstSeq: "0", events: [] });
  const ensure = (agent: AgentInfo, event: PublicEvent): AgentConversation => {
    const key = instanceKey(agent);
    let group = groups.get(key);
    if (!group) {
      // Unattributed events share an explicit unknown-source bucket. They are
      // not claimed to be one subagent instance merely because names match.
      group = { key, agent: key === "unknown" ? { role: "unknown", name: "Agent" } : agent, state: "unknown", firstSeen: event.timestamp, firstSeq: event.seq, events: [] };
      groups.set(key, group);
    } else if (key !== "unknown") group.agent = agent;
    return group;
  };
  for (const event of events) {
    const payload = payloadRecord(event);
    const agent = eventAgentInfo(event, index);
    if (agent) ensure(agent, event).events.push(event);
    const child = resolveAgent(event, payload?.spawned_agent ?? payload?.target_agent, index);
    if (child) {
      const group = ensure(child, event);
      if (event.type === "agent.tool_call" && payload?.spawned_agent) group.state = "running";
      if (event.type === "agent.tool_result") {
        if (payload?.subagent_status === "running") group.state = "running";
        else if (terminal.has(String(payload?.subagent_status))) group.state = "ended";
        else if (payload?.spawned_agent && ![...tasks.values()].includes(group.key)) group.state = "unknown";
      }
    }
    if (!agent || agent.role !== "sub" || instanceKey(agent) === "unknown" || !event.type.startsWith("agent.task_")) continue;
    const taskId = typeof payload?.task_id === "string" ? payload.task_id : "";
    const group = ensure(agent, event);
    if (event.type === "agent.task_started" && taskId && (payload?.task_type === "local_agent" || (payload?.task_type !== "local_bash" && taskId === agent.task_id))) {
      tasks.set(taskId, group.key);
      group.state = "running";
    }
    // A worker's Bash may finish while the worker itself continues. Only a
    // task explicitly identified as local_agent controls the agent lifecycle.
    if ((tasks.get(taskId) === group.key || (taskId && taskId === agent.task_id)) && payload?.task_type !== "local_bash") {
      const status = payload?.status ?? record(payload?.patch)?.status;
      if (terminal.has(String(status))) { group.state = "ended"; tasks.delete(taskId); }
      else if (status === "running" || status === "in_progress") group.state = "running";
    }
  }
  if (!runtimeIsActive(runtimeState)) for (const group of groups.values()) if (group.key !== "unknown") group.state = "ended";
  const rank = (group: AgentConversation) => group.key === "main" ? -1 : group.key === "unknown" ? 3 : group.state === "running" ? 0 : group.state === "ended" ? 1 : 2;
  const agents = [...groups.values()].sort((a, b) => rank(a) - rank(b) || (Date.parse(a.firstSeen) - Date.parse(b.firstSeen) || (a.firstSeq === b.firstSeq ? 0 : BigInt(a.firstSeq) < BigInt(b.firstSeq) ? -1 : 1)));
  return { agents, index };
}
