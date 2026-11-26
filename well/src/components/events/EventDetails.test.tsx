import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PublicEvent } from "../../api/dto";
import i18n from "../../i18n";
import { formatTime } from "../../lib/format";
import { EventRow } from "./EventRow";

// Shapes observed through the running ECS nornsd API; identifiers and prose
// have been replaced. Tests do not require a live server or saved credentials.
const worker = { role: "sub", name: "Explorer", agent_id: "worker-1", instance_id: "tool:call-1", tool_use_id: "call-1", task_id: "task-1" };
function show(type: string, payload: PublicEvent["payload"]) {
  const event: PublicEvent = { seq: "7", type, source: "agent", payload, runtime_id: "runtime-1", timestamp: "2026-11-25T06:00:00Z" };
  return render(<EventRow event={event} highlighted={false} onJump={vi.fn()} />);
}
function fact(label: string) {
  return within(screen.getByText(label, { selector: ".event-fact > span" }).parentElement as HTMLElement);
}

beforeEach(async () => { await i18n.changeLanguage("zh"); });

describe("Readable event projections", () => {
  it("renders observed task progress with ownership, actions and reported usage", () => {
    const { container } = show("agent.task_progress", {
      agent: worker, task_id: "task-1", description: "Running local check", last_tool_name: "Bash",
      subagent_type: "norns:explorer", usage: { duration_ms: 1057, total_tokens: 15210, tool_uses: 1 },
    });
    expect(screen.getByText("子智能体 · Explorer · worker-1")).toBeInTheDocument();
    expect(fact("任务 ID").getByText("task-1")).toBeInTheDocument();
    expect(fact("当前动作").getByText("Running local check")).toBeInTheDocument();
    expect(fact("最近工具").getByText("Bash")).toBeInTheDocument();
    expect(fact("已用时间").getByText("1.057 秒")).toBeInTheDocument();
    expect(fact("已报告 Token 数").getByText("15210")).toBeInTheDocument();
    expect(container.querySelector(".event-content pre")).toBeNull();
    expect(screen.queryByText(/"duration_ms"/)).not.toBeInTheDocument();
  });

  it.each([["local_agent", "子智能体任务"], ["local_bash", "Bash 后台任务"]])("identifies task_started %s without inventing completion", (type, label) => {
    show("agent.task_started", { task_id: "task-1", task_type: type, description: "Run check", agent: worker });
    expect(fact("任务类型").getByText(label)).toBeInTheDocument();
    expect(screen.queryByText("已完成")).not.toBeInTheDocument();
  });

  it("reads status/end_time and background changes from patch, preserving false", () => {
    const end = 1795587408394;
    show("agent.task_updated", { agent: worker, task_id: "task-1", patch: { status: "killed", end_time: end, is_backgrounded: false } });
    expect(fact("状态").getByText("已终止")).toBeInTheDocument();
    expect(fact("结束时间").getByText(formatTime(new Date(end).toISOString(), "zh"))).toBeInTheDocument();
    expect(fact("后台执行").getByText("否")).toBeInTheDocument();
    expect(screen.queryByText(/"patch"/)).not.toBeInTheDocument();
  });

  it.each([["completed", "已完成"], ["stopped", "已停止"], ["failed", "失败"]])("renders a %s task notification and summary", (status, label) => {
    show("agent.task_notification", { agent: worker, task_id: "task-1", status, summary: "**Task summary**\n\n- Reported outcome", output_file: "/tmp/task.output", usage: { duration_ms: 3000, tool_uses: 0, total_tokens: 12 } });
    expect(fact("状态").getByText(label)).toBeInTheDocument();
    expect(screen.getByText("Task summary").tagName).toBe("STRONG");
    expect(fact("输出文件").getByText("/tmp/task.output")).toBeInTheDocument();
    expect(fact("工具调用次数").getByText("0")).toBeInTheDocument();
  });

  it("distinguishes native JSON message bodies from raw event payloads", () => {
    const { container } = show("agent.task_notification", {
      agent: worker, task_id: "task-1", status: "completed", summary: '{"state":"explore","explorations":[{"id":"exp_1"}]}',
    });
    expect(container.querySelector(".event-body-json")?.textContent).toBe(JSON.stringify({ state: "explore", explorations: [{ id: "exp_1" }] }, null, 2));
    expect(container.querySelector(".event-payload")).toBeNull();
    expect(container.querySelector(".event-body-json")?.textContent).not.toContain("task_id");
  });

  it.each([["requesting", "正在请求模型响应"], [null, "状态标记已清除"], ["new_status", "new_status"]])("renders status %s without inferring Runtime state", (status, label) => {
    show("agent.status", { status });
    expect(fact("状态").getByText(label)).toBeInTheDocument();
    expect(screen.queryByText("运行时正在等待后续指令。")).not.toBeInTheDocument();
  });

  it("renders replay, receipt and settlement as separate input lifecycle stages", () => {
    let view = show("instruction.replayed", { input_uuid: "input-1" });
    expect(fact("输入 ID").getByText("input-1")).toBeInTheDocument();
    expect(screen.getByText(/Claude Code 已回显输入/)).toBeInTheDocument();
    view.unmount();
    view = show("instruction.accepted", { input_uuid: "input-1", instruction_sequences: [1, 2] });
    expect(fact("指令编号").getByText("1, 2")).toBeInTheDocument();
    expect(screen.getByText("Claude Code 已确认接收以下指令。")).toBeInTheDocument();
    view.unmount();
    show("instruction.settled", { result_id: "result-1", instruction_sequences: [1, 2], is_error: true, origin: null });
    expect(fact("结果 ID").getByText("result-1")).toBeInTheDocument();
    expect(fact("执行状态").getByText("失败")).toBeInTheDocument();
    expect(screen.queryByText("运行时已结束。")).not.toBeInTheDocument();
  });

  it("renders injected session context as message content, without labeling it human input", () => {
    show("agent.user_message", { agent: worker, text: "# Skill context\n\n**Instructions**" });
    expect(screen.getByRole("heading", { name: "Skill context" })).toBeInTheDocument();
    expect(screen.getByText("记录了一条会话输入或注入上下文。")).toBeInTheDocument();
    expect(screen.queryByText(/"text"/)).not.toBeInTheDocument();
  });

  it.each([["[Request interrupted by user]", "当前处理已中断。"], ["[Request interrupted by user for tool use]", "当前处理在工具使用期间中断。"]])("interprets interruption marker %s", (text, message) => {
    show("agent.interrupted", { agent: worker, text });
    expect(screen.getByText(message)).toBeInTheDocument();
    expect(screen.queryByText(text)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "数据" }));
    expect(screen.getByText(new RegExp(text.replace(/[\[\]]/g, "\\$&")))).toBeInTheDocument();
  });

  it("renders API retry timing and a 429 error independently from Runtime failure", () => {
    show("agent.api_retry", { attempt: 1, max_retries: 10, retry_delay_ms: 509.9832782375545, error_status: 429, error: "rate_limit" });
    expect(fact("重试次数").getByText("1")).toBeInTheDocument();
    expect(fact("重试次数上限").getByText("10")).toBeInTheDocument();
    expect(fact("重试等待时间").getByText("0.51 秒")).toBeInTheDocument();
    expect(fact("HTTP 状态码").getByText("429")).toBeInTheDocument();
    expect(screen.getByText("请求受到限流")).toBeInTheDocument();
    expect(screen.queryByText("运行时已结束。")).not.toBeInTheDocument();
  });

  it("renders processing errors, reasons and usage while separating cumulative snapshots", () => {
    const { container } = show("agent.result", {
      is_error: true, subtype: "error_during_execution", terminal_reason: "aborted_streaming", stop_reason: "tool_use",
      num_turns: 21, errors: ["[ede_diagnostic] result_type=user"],
      usage: { input_tokens: 25468, output_tokens: 2699, cache_read_input_tokens: 180480, cache_creation_input_tokens: 0 },
      model_usage: { model: { inputTokens: 123390, outputTokens: 32963, cacheCreationInputTokens: 123 } }, total_cost_usd: 2.071489,
    });
    expect(fact("本次处理结束原因").getByText("接收响应期间中断")).toBeInTheDocument();
    expect(fact("停止原因").getByText("调用工具")).toBeInTheDocument();
    expect(fact("处理轮数").getByText("21")).toBeInTheDocument();
    expect(screen.getByText("[ede_diagnostic] result_type=user")).toBeInTheDocument();
    expect(fact("本事件报告的用量").getByText("25468")).toBeInTheDocument();
    expect(screen.queryByText("进程累计费用（USD）")).not.toBeInTheDocument();
    expect(screen.queryByText("2.071489")).not.toBeInTheDocument();
    expect(screen.queryByText("缓存写入 Token")).not.toBeInTheDocument();
    expect(screen.queryByText("123", { exact: true })).not.toBeInTheDocument();
    expect(screen.getByText("按模型查看进程累计用量").tagName).toBe("SUMMARY");
    expect(screen.queryByText(/"errors"/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "数据" }));
    expect(container.querySelector(".event-payload")?.textContent).toContain('"total_cost_usd": 2.071489');
    expect(container.querySelector(".event-payload")?.textContent).toContain('"cache_creation_input_tokens": 0');
    expect(container.querySelector(".event-payload")?.textContent).toContain('"cacheCreationInputTokens": 123');
  });

  it("renders notification result content and keeps zero turns and token counters", () => {
    show("agent.result", { is_error: false, subtype: "success", origin_kind: "task-notification", terminal_reason: "completed", result: "**Notification handled**", num_turns: 0, usage: { input_tokens: 0, output_tokens: 0 }, total_cost_usd: 0 });
    expect(screen.getByText("Notification handled").tagName).toBe("STRONG");
    expect(fact("结果来源").getByText("后台任务通知")).toBeInTheDocument();
    expect(fact("处理轮数").getByText("0")).toBeInTheDocument();
    expect(fact("输入 Token").getByText("0")).toBeInTheDocument();
    expect(fact("输出 Token").getByText("0")).toBeInTheDocument();
    expect(screen.queryByText("进程累计费用（USD）")).not.toBeInTheDocument();
  });

  it("distinguishes a successful background launch from completed child work", () => {
    show("agent.tool_result", { agent: { role: "main", name: "Coordinator" }, is_error: false, subagent_status: "running", spawned_agent: worker, content: "Agent launched" });
    expect(fact("执行状态").getByText("成功")).toBeInTheDocument();
    expect(fact("子智能体状态").getByText("运行中")).toBeInTheDocument();
    expect(screen.queryByText("已完成")).not.toBeInTheDocument();
  });

  it("preserves mixed tool output verbatim and formats a native TSec JSON result", () => {
    const mixed = 'output before JSON\n{"correct":true}\noutput after JSON';
    const view = show("agent.tool_result", { content: [{ type: "text", text: mixed }] });
    expect(view.container.querySelector(".event-result-content pre")?.textContent).toBe(mixed);
    view.unmount();
    const formatted = show("agent.tool_result", { content: '{"correct":true,"correct_flag_count":1,"total_flag_count":2}' });
    expect(formatted.container.querySelector(".event-body-json")?.textContent).toBe(JSON.stringify({ correct: true, correct_flag_count: 1, total_flag_count: 2 }, null, 2));
  });

  it("displays synthetic assistant errors and structured error messages", () => {
    show("agent.text", { text: "API Error: 401 No API-key provided.", model: "<synthetic>", error: { type: "authentication_error", message: "No API-key provided." } });
    expect(fact("模型").getByText("<synthetic>")).toBeInTheDocument();
    expect(screen.getByText("认证失败")).toBeInTheDocument();
    expect(screen.getByText("No API-key provided.")).toBeInTheDocument();
  });

  it.each(["runtime.error", "diagnostic"])("renders %s messages and preserves raw unknown fields", (type) => {
    const { container } = show(type, { code: "SERVER_INTERRUPTED", message: "Process cleanup needs attention", recovery_blocked: true, future_field: { value: "inspectable" } });
    expect(screen.getByText("SERVER_INTERRUPTED")).toBeInTheDocument();
    expect(screen.getByText("Process cleanup needs attention")).toBeInTheDocument();
    expect(fact("恢复受阻").getByText("是")).toBeInTheDocument();
    expect(container.querySelector(".event-payload")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "数据" }));
    expect(container.querySelector(".event-payload")?.textContent).toContain('"future_field"');
  });

  it.each(["instruction.replayed", "instruction.accepted", "instruction.settled", "agent.api_retry", "agent.status", "agent.task_started", "agent.task_progress", "agent.task_updated", "agent.task_notification", "agent.user_message", "agent.interrupted", "agent.result", "runtime.error", "diagnostic", "future.event"])("tolerates an old or sparse %s payload", (type) => {
    const { container } = show(type, {});
    expect(container.querySelector(".event-content")).not.toBeEmptyDOMElement();
    expect(container.textContent).not.toContain("events.details.");
    expect(container.textContent).not.toContain("undefined");
  });

  it("localizes the same projection in English", async () => {
    await i18n.changeLanguage("en");
    show("agent.task_updated", { task_id: "task-1", patch: { status: "completed", is_backgrounded: true } });
    expect(screen.getByText("Task properties changed.")).toBeInTheDocument();
    expect(fact("State").getByText("Completed")).toBeInTheDocument();
    expect(fact("Background execution").getByText("Yes")).toBeInTheDocument();
  });
});
