import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeAll } from "vitest";
import i18n from "../../i18n";
import { HistoryPanel } from "./HistoryPanel";

beforeAll(() => i18n.changeLanguage("zh"));

it("loads runtime details only after a history record is expanded", async () => {
  const runtimeId = "55555555-5555-4555-8555-555555555555";
  const commands: string[] = [];
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string) => {
    commands.push(command);
    if (command === "list_runtimes_page") return { runtimes: [{ id: runtimeId, state: "ended", current: false, created_at: "2026-10-16T00:00:00Z", started_at: "2026-10-16T00:00:01Z", ended_at: "2026-10-16T00:10:00Z" }], next_cursor: null, total_count: 1 } as T;
    return { id: runtimeId, subject_id: "subject", state: "ended", agent: "cc", session_id: runtimeId, queued_at: "2026-10-16T00:00:00Z", started_at: "2026-10-16T00:00:01Z", ended_at: "2026-10-16T00:10:00Z", end_reason: "user_graceful_stop", settings_slot: 1, config_hash: null, process: null, usage: null, resources: null, error: null, artifacts: { raw_log_path: "/raw.jsonl", readable_log_path: "/readable.jsonl", transcript_path: null, transcript_status: "pending" } } as T;
  } };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  render(<QueryClientProvider client={client}><HistoryPanel serverId="server" subjectId="subject" connection={{ base_url: "http://server", password: "secret" }} /></QueryClientProvider>);
  await screen.findByText("共 1 条");
  expect(commands).not.toContain("get_runtime");
  fireEvent.click(screen.getByRole("button", { name: /运行时记录/ }));
  fireEvent.click(await screen.findByRole("button", { name: new RegExp(runtimeId) }));

  await waitFor(() => expect(commands).toContain("get_runtime"));
  expect(await screen.findByText("用户请求优雅终止")).toBeInTheDocument();
  expect(screen.getAllByText(runtimeId).length).toBeGreaterThan(1);
  expect(screen.getByText("等待运行时结束后转储")).toBeInTheDocument();
});
