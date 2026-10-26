import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PublicEvent } from "../../api/dto";
import i18n from "../../i18n";
import { EventRow } from "./EventRow";

function event(type: string, source: string, payload: Record<string, unknown>, runtimeId: string | null = "runtime-12345678"): PublicEvent {
  return { seq: "7", type, source, payload, runtime_id: runtimeId, timestamp: "2026-10-22T08:30:00Z" };
}

beforeEach(async () => {
  await i18n.changeLanguage("zh");
});

describe("EventRow", () => {
  it("renders localized metadata and a semantic runtime summary", () => {
    const { container } = render(<EventRow event={event("runtime.started", "runtime", { slot: 2 })} highlighted={false} onJump={vi.fn()} />);
    const header = container.querySelector(".event-header");
    expect(header).not.toBeNull();
    expect(within(header as HTMLElement).getByText("运行时", { selector: ".event-source-badge" })).toBeInTheDocument();
    expect(within(header as HTMLElement).getByText("启动运行时")).toBeInTheDocument();
    expect(within(header as HTMLElement).getByText(/运行时 runtime/)).toBeInTheDocument();
    expect(screen.getByText("运行时已在槽位 2 启动。")).toBeInTheDocument();
    expect(screen.queryByText(/"slot"/)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "数据" }));
    expect(screen.getByText(/"slot": 2/)).toBeInTheDocument();
  });

  it("renders a resumed session as Runtime lifecycle metadata", () => {
    render(<EventRow event={event("runtime.session_resumed", "runtime", { resume_session_id: "session-old-12345678", session_id: "session-new-87654321", usage: { input_tokens: 0 } })} highlighted={false} onJump={vi.fn()} />);
    expect(screen.getByText("恢复会话")).toBeInTheDocument();
    expect(screen.getByText("已从上一会话派生新的运行时会话。")).toBeInTheDocument();
    expect(screen.getByText("来源会话")).toBeInTheDocument();
    expect(screen.getByText("新会话")).toBeInTheDocument();
    expect(screen.queryByText("智能体执行成功。")).not.toBeInTheDocument();
  });

  it("renders agent text as markdown", () => {
    const { container } = render(<EventRow event={event("agent.text", "agent", { text: "**完成**\n\n- 第一项\n- 第二项", agent: { role: "main", name: "Coordinator" } })} highlighted={false} onJump={vi.fn()} />);
    expect(screen.getByText("主智能体 · Coordinator")).toBeInTheDocument();
    expect(screen.getByText("完成").tagName).toBe("STRONG");
    expect(container.querySelectorAll(".event-markdown li")).toHaveLength(2);
  });

  it("renders tool parameters and structured result metadata", () => {
    const call = event("agent.tool_call", "agent", { name: "Read", input: { file_path: "src/main.ts", limit: 20 }, tool_call_id: "tool-1", agent: { role: "main", name: "Coordinator" } });
    const result = event("agent.tool_result", "agent", { content: "Read 20 lines", is_error: false, tool_call_id: "tool-1", agent: { role: "main", name: "Coordinator" } });
    const onJump = vi.fn();
    const { rerender } = render(<EventRow event={call} linkedResult={result} highlighted={false} onJump={onJump} />);
    expect(screen.getByText("Read", { selector: ".event-tool-name" })).toBeInTheDocument();
    expect(screen.getByText("file_path")).toBeInTheDocument();
    expect(screen.getByText('"src/main.ts"')).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /查看结果/ }));
    expect(onJump).toHaveBeenCalledWith("7");

    rerender(<EventRow event={result} linkedCall={call} highlighted={false} onJump={onJump} />);
    expect(screen.getByText("main · Coordinator")).toBeInTheDocument();
    expect(screen.getByText("成功")).toHaveClass("succeeded");
    expect(screen.getByText("Read 20 lines")).toBeInTheDocument();
  });
});
