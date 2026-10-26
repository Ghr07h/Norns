import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeAll } from "vitest";
import i18n from "../../i18n";
import type { Runtime, SubjectDetail } from "../../api/dto";
import { RuntimePanel } from "./RuntimePanel";

beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function showModal() { this.setAttribute("open", ""); };
  HTMLDialogElement.prototype.close = function close() { this.removeAttribute("open"); };
});

it("submits force stop only after the in-app confirmation", async () => {
  await i18n.changeLanguage("zh");
  let invocation: { command: string; args?: Record<string, unknown> } | undefined;
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string, args?: Record<string, unknown>) => {
    if (command === "list_runtimes_page") return { runtimes: [], next_cursor: null, total_count: 0 } as T;
    invocation = { command, args };
    return { accepted: true } as T;
  } };
  const runtime: Runtime = { id: "runtime", subject_id: "subject", state: "running", instructions: [], created_at: "2026-10-16T00:00:00Z", started_at: "2026-10-16T00:00:01Z", resources: {}, closing: false, close_deadline: null, state_changed_at: "2026-10-16T00:00:01Z" };
  const subject: SubjectDetail = { id: "subject", name: null, state: "using", runtime_profile: { agent: "cc", agent_version: "2.1.199", capabilities: [] }, workspace: { materialized: true, directory_name: "subject" }, last_event_seq: 0, created_at: "2026-10-16T00:00:00Z", updated_at: "2026-10-16T00:00:01Z" };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });

  render(<QueryClientProvider client={client}><RuntimePanel serverId="server" connection={{ base_url: "http://server", password: "secret" }} subject={subject} runtime={runtime} /></QueryClientProvider>);
  fireEvent.click(screen.getByRole("button", { name: "强制终止" }));
  expect(invocation).toBeUndefined();
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "强制终止" }));

  await waitFor(() => expect(invocation).toEqual({ command: "stop_runtime", args: { input: { connection: { base_url: "http://server", password: "secret" }, subject_id: "subject", mode: "force" } } }));
});

it("submits graceful stop only after the in-app confirmation", async () => {
  await i18n.changeLanguage("zh");
  const invocations: string[] = [];
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string) => {
    invocations.push(command);
    if (command === "list_runtimes_page") return { runtimes: [], next_cursor: null, total_count: 0 } as T;
    return { accepted: true } as T;
  } };
  const runtime: Runtime = { id: "runtime", subject_id: "subject", state: "running", instructions: [], created_at: "2026-10-16T00:00:00Z", started_at: "2026-10-16T00:00:01Z", resources: {}, closing: false, close_deadline: null, state_changed_at: "2026-10-16T00:00:01Z" };
  const subject: SubjectDetail = { id: "subject", name: null, state: "using", runtime_profile: { agent: "cc", agent_version: "2.1.199", capabilities: [] }, workspace: { materialized: true, directory_name: "subject" }, last_event_seq: 0, created_at: "2026-10-16T00:00:00Z", updated_at: "2026-10-16T00:00:01Z" };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });

  render(<QueryClientProvider client={client}><RuntimePanel serverId="server" connection={{ base_url: "http://server", password: "secret" }} subject={subject} runtime={runtime} /></QueryClientProvider>);
  fireEvent.click(screen.getByRole("button", { name: "优雅终止" }));
  expect(invocations).not.toContain("stop_runtime");
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "优雅终止" }));

  await waitFor(() => expect(invocations).toContain("stop_runtime"));
});
