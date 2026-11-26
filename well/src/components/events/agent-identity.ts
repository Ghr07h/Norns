import type { PublicEvent } from "../../api/dto";

export type AgentInfo = {
  role: string;
  name: string;
  instance_id?: string;
  tool_use_id?: string;
  task_id?: string;
  agent_id?: string;
};
export type AgentIndex = Map<string, AgentInfo>;

function record(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : null;
}
function text(value: unknown): string | undefined {
  return typeof value === "string" && value ? value : undefined;
}
export function readAgent(value: unknown): AgentInfo | null {
  const agent = record(value);
  if (!agent) return null;
  return {
    role: text(agent.role) ?? "unknown", name: text(agent.name) ?? "Agent",
    instance_id: text(agent.instance_id), tool_use_id: text(agent.tool_use_id),
    task_id: text(agent.task_id), agent_id: text(agent.agent_id),
  };
}
function keys(event: PublicEvent, agent: AgentInfo): string[] {
  const aliases = [agent.instance_id, agent.tool_use_id && `tool:${agent.tool_use_id}`, agent.task_id && `task:${agent.task_id}`, agent.agent_id && `agent:${agent.agent_id}`];
  return [...new Set(aliases.filter((id): id is string => Boolean(id)))].map((id) => `${event.runtime_id ?? ""}\u0000${id}`);
}

// Keep only attribution metadata when loading other Runtime histories.
export function agentContextEvent(event: PublicEvent): PublicEvent | null {
  const payload = record(event.payload);
  if (!payload) return null;
  if (event.type === "runtime.session_resumed") {
    return { ...event, payload: { resume_session_id: payload.resume_session_id } };
  }
  if (!event.type.startsWith("agent.")) return null;
  const context: Record<string, unknown> = {};
  for (const field of ["agent", "spawned_agent", "target_agent"]) {
    const agent = readAgent(payload[field]);
    if (agent && (agent.role === "main" || agent.instance_id || agent.tool_use_id || agent.task_id || agent.agent_id)) context[field] = agent;
  }
  if (event.type.startsWith("agent.task_")) {
    for (const field of ["task_id", "task_type"]) if (text(payload[field])) context[field] = payload[field];
  }
  return Object.keys(context).length ? { ...event, payload: context } : null;
}

// A returned agentId can arrive after the child's text. Join explicit aliases
// in the visible events without rewriting their original persisted payloads.
export function buildAgentIndex(events: PublicEvent[], context: PublicEvent[] = []): AgentIndex {
  const index: AgentIndex = new Map();
  const all = [...context, ...events].sort((a, b) => BigInt(a.seq) < BigInt(b.seq) ? -1 : BigInt(a.seq) > BigInt(b.seq) ? 1 : 0);
  for (const event of all) {
    const payload = record(event.payload);
    for (const value of [payload?.agent, payload?.spawned_agent, payload?.target_agent]) {
      const agent = readAgent(value);
      if (!agent) continue;
      const aliases = keys(event, agent);
      if (!aliases.length) continue;
      const previous = [...new Set(aliases.map((id) => index.get(id)).filter((a): a is AgentInfo => Boolean(a)))];
      const merged: AgentInfo = { role: agent.role, name: agent.name };
      for (const info of [...previous, agent]) {
        for (const [field, value] of Object.entries(info)) {
          if (value !== undefined && !(field === "name" && value === "Subagent" && merged.name !== "Subagent")) Object.assign(merged, { [field]: value });
        }
      }
      for (const [id, info] of index) if (previous.includes(info)) index.set(id, merged);
      for (const id of aliases) index.set(id, merged);
    }
  }

  // CC can emit terminal notifications for tasks inherited from a cancelled
  // session before emitting the resume confirmation. Use the explicit resume
  // lineage and task IDs, never role names or unrelated Runtime histories.
  const parents = new Map<string, Set<string>>();
  for (const event of all) {
    const parent = event.type === "runtime.session_resumed" && text(record(event.payload)?.resume_session_id);
    if (!event.runtime_id || !parent || parent === event.runtime_id) continue;
    const candidates = parents.get(event.runtime_id) ?? new Set<string>();
    candidates.add(parent);
    parents.set(event.runtime_id, candidates);
  }
  for (const event of all) {
    if (!event.type.startsWith("agent.task_") || !event.runtime_id) continue;
    const payload = record(event.payload);
    const taskId = text(payload?.task_id);
    if (!taskId) continue;
    let owner = resolveAgent(event, payload?.agent, index);
    if (!owner || owner.role === "unknown" || (owner.role === "sub" && !agentInstanceID(owner))) {
      let runtime: string | undefined = event.runtime_id;
      const visited = new Set<string>();
      while (runtime && !visited.has(runtime)) {
        visited.add(runtime);
        const known = index.get(`${runtime}\u0000task:${taskId}`);
        if (known && known.role !== "unknown") { owner = known; break; }
        const candidates = parents.get(runtime);
        runtime = candidates?.size === 1 ? candidates.values().next().value : undefined;
      }
    }
    if (owner && owner.role !== "unknown") index.set(`${event.runtime_id}\u0000task:${taskId}`, owner);
  }
  return index;
}

export function taskEventAgent(event: PublicEvent, index?: AgentIndex): AgentInfo | null {
  const taskId = text(record(event.payload)?.task_id);
  return event.type.startsWith("agent.task_") && taskId ? index?.get(`${event.runtime_id ?? ""}\u0000task:${taskId}`) ?? null : null;
}
export function resolveAgent(event: PublicEvent, value: unknown, index?: AgentIndex): AgentInfo | null {
  const agent = readAgent(value);
  if (!agent) return null;
  for (const id of keys(event, agent)) {
    const known = index?.get(id);
    if (known) return known;
  }
  return agent;
}

export function agentInstanceID(agent: AgentInfo): string | null {
  return agent.agent_id ?? agent.tool_use_id ?? agent.task_id ?? agent.instance_id ?? null;
}
