import type { PublicEvent } from "../../api/dto";
import { eventAgentInfo, payloadRecord } from "./EventRow";
import { readAgent, type AgentIndex, type AgentInfo } from "./agent-identity";

const AGENT_MESSAGE_TYPES = new Set(["agent.text", "agent.tool_call", "agent.tool_result", "agent.interrupted", "agent.result"]);

export function isAgentMessageEvent(event: Pick<PublicEvent, "type">): boolean {
  return AGENT_MESSAGE_TYPES.has(event.type);
}

export type EventFilters = {
  text: string;
  runtimeId: string;
  eventType: string;
  agentRole: string;
  agentId: string;
};
export const EMPTY_EVENT_FILTERS: EventFilters = { text: "", runtimeId: "", eventType: "", agentRole: "", agentId: "" };

export function hasEventFilters(filters: EventFilters): boolean {
  return Object.values(filters).some((value) => value.trim() !== "");
}
export function namedAgentRole(agent: AgentInfo): string {
  return `name:${JSON.stringify([agent.role, agent.name])}`;
}
function includes(value: string | null | undefined, query: string): boolean {
  return (value ?? "").toLowerCase().includes(query.trim().toLowerCase());
}
export function matchesEventFilters(event: PublicEvent, filters: EventFilters, index: AgentIndex): boolean {
  if (filters.runtimeId.trim() && !includes(event.runtime_id, filters.runtimeId)) return false;
  if (filters.eventType && event.type !== filters.eventType) return false;
  const agent = eventAgentInfo(event, index);
  if (filters.agentRole && (!agent || (filters.agentRole.startsWith("kind:")
    ? `kind:${agent.role}` !== filters.agentRole : namedAgentRole(agent) !== filters.agentRole))) return false;
  if (filters.agentId.trim()) {
    // Include the original aliases as well: resume may add a new invocation ID.
    const original = readAgent(payloadRecord(event)?.agent);
    const ids = [agent, original].flatMap((info) => info ? [info.agent_id, info.instance_id, info.tool_use_id, info.task_id] : []);
    if (!ids.some((id) => id && includes(id, filters.agentId))) return false;
  }
  if (filters.text.trim() && !includes(`${event.type} ${event.source} ${event.runtime_id ?? ""} ${JSON.stringify(event.payload)}`, filters.text)) return false;
  return true;
}
