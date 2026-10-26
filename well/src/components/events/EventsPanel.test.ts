import { describe, expect, it } from "vitest";
import type { PublicEvent } from "../../api/dto";
import { buildToolEventLinks, eventAgentLabel } from "./EventsPanel";
import { eventSourceLabel, eventTypeLabel } from "./EventRow";

function event(seq: string, type: string, payload: Record<string, unknown>): PublicEvent {
  return { seq, type, payload, source: "agent", runtime_id: "runtime-1", timestamp: "2026-10-16T00:00:00Z" };
}

describe("EventsPanel event metadata", () => {
  it("labels main and domain subagents", () => {
    expect(eventAgentLabel(event("1", "agent.text", { agent: { role: "main", name: "Coordinator" } }), "zh-CN")).toBe("主智能体 · Coordinator");
    expect(eventAgentLabel(event("2", "agent.text", { agent: { role: "sub", name: "Explorer-For-Math" } }), "zh-CN")).toBe("子智能体 · Explorer-For-Math");
    expect(eventAgentLabel(event("3", "agent.text", { agent: { role: "main", name: "Coordinator" } }), "en-US")).toBe("Main agent · Coordinator");
  });

  it("localizes known sources and event actions", () => {
    expect(eventSourceLabel("runtime", "zh-CN")).toBe("运行时");
    expect(eventSourceLabel("server", "zh-CN")).toBe("服务");
    expect(eventSourceLabel("nornsd", "zh-CN")).toBe("服务");
    expect(eventSourceLabel("custom", "zh-CN")).toBe("custom");
    expect(eventTypeLabel("subject.created", "zh-CN")).toBe("创建实体");
    expect(eventTypeLabel("runtime.session_resumed", "zh-CN")).toBe("恢复会话");
    expect(eventTypeLabel("agent.tool_call", "zh-CN")).toBe("调用工具");
    expect(eventTypeLabel("custom.event", "zh-CN")).toBe("custom.event");
  });

  it("links tool calls and results by tool_call_id", () => {
    const call = event("10", "agent.tool_call", { tool_call_id: "tool-1", name: "Bash" });
    const result = event("14", "agent.tool_result", { tool_call_id: "tool-1", content: "done" });
    const links = buildToolEventLinks([call, result]);
    expect(links.calls.get("tool-1")?.seq).toBe("10");
    expect(links.results.get("tool-1")?.seq).toBe("14");
  });
});
