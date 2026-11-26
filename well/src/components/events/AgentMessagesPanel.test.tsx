import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import type { PublicEvent } from "../../api/dto";
import i18n from "../../i18n";
import { cacheEvents, clearAllCaches, readEventCacheState } from "../../storage/event-db";
import { AgentMessagesPanel } from "./AgentMessagesPanel";
import { keys } from "../../api/query-keys";
import resumedTasks from "./testdata/resumed-task-notifications.json";

const connection = { base_url: "http://server", password: "secret" };
const main = { role: "main", name: "Coordinator" };
const worker = (id: string) => ({ role: "sub", name: "Worker", instance_id: `tool:${id}`, tool_use_id: id, agent_id: `agent-${id}`, task_id: `task-${id}` });
const runtime = (id: string, state = "running", current = true) => ({ id, state, current, created_at: "2026-11-20T00:00:00Z", started_at: "2026-11-20T00:00:01Z", ended_at: current ? null : "2026-11-20T00:10:00Z" });
function event(seq: number, type: string, payload: PublicEvent["payload"], runtimeId = "current"): PublicEvent {
  return { seq: String(seq), type, payload, runtime_id: runtimeId, source: "agent", timestamp: "2026-11-20T00:00:00Z" };
}
const spawn = (seq: number, id: string) => event(seq, "agent.tool_call", { agent: main, name: "Agent", tool_call_id: id, spawned_agent: worker(id) });
const page = (events: PublicEvent[], next: string | null = null, cursor = "sync") => ({ events, next_cursor: next, cursor, has_more: false });
let client: QueryClient;
beforeEach(async () => {
  await i18n.changeLanguage("en");
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
});
afterEach(async () => { client.clear(); delete window.__WELL_BRIDGE__; await clearAllCaches(); });
function panel(subjectId = "subject") {
  return <QueryClientProvider client={client}><AgentMessagesPanel serverId="server" subjectId={subjectId} connection={connection} subjectActive={false} /></QueryClientProvider>;
}

it.each([true, false])("attributes resume cleanup notifications using cached/backfilled history (cached=%s)", async (cached) => {
  const prior = [event(100, "agent.text", { agent: worker("a"), text: "Previous Runtime private output" }, "prior-runtime"), ...resumedTasks.filter((event) => event.runtime_id === "prior-runtime")];
  const current = [...resumedTasks.filter((event) => event.runtime_id === "resumed-runtime"), event(1157, "agent.text", { agent: main, text: "Current Runtime output" }, "resumed-runtime")];
  if (cached) await cacheEvents("server", "subject", [...prior, ...current], { syncCursor: "sync", olderCursor: null });
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string, args?: Record<string, unknown>) => {
    if (command === "list_runtimes_page") return { runtimes: [runtime("resumed-runtime")], next_cursor: null, total_count: 1 } as T;
    if (command === "get_event_updates") return { has_updates: false } as T;
    if (command === "list_events" && !cached) {
      const input = args?.input as Record<string, unknown>;
      return (input.cursor ? page(prior) : page(current, "older-runtime")) as T;
    }
    throw new Error(`unexpected command: ${command}`);
  } };
  render(panel());
  await screen.findByText("Current Runtime output");
  const roster = screen.getByRole("navigation", { name: "Agent list" });
  await waitFor(() => expect(within(roster).getAllByRole("button")).toHaveLength(4));
  expect(screen.queryByText("Unattributed")).not.toBeInTheDocument();
  for (let i = 1; i <= 3; i++) {
    const agent = within(roster).getByRole("button", { name: new RegExp(`prior-task-${i}`) });
    expect(agent).toHaveTextContent("Explorer-For-Redteam");
    expect(agent).toHaveTextContent("Ended");
    fireEvent.click(agent);
    expect(document.querySelectorAll(".event-row")).toHaveLength(1);
    expect(screen.getByText("Failed")).toBeInTheDocument();
    expect(screen.queryByText("Previous Runtime private output")).not.toBeInTheDocument();
  }
  fireEvent.click(within(roster).getByRole("button", { name: /Main agent/ }));
  expect(screen.getByText("Current Runtime output")).toBeInTheDocument();
});

it("Agent Only filters messages without hiding agents or changing their lifecycle state", async () => {
  const events = [
    event(1, "agent.text", { agent: main, text: "Main output" }),
    spawn(2, "a"),
    event(3, "agent.text", { agent: worker("a"), text: "Worker output" }),
    event(4, "agent.task_started", { agent: worker("a"), task_id: "task-a", task_type: "local_agent" }),
    event(5, "agent.tool_result", { agent: main, tool_call_id: "a", spawned_agent: worker("a"), subagent_status: "running", content: "Launched" }),
    event(6, "agent.interrupted", { agent: main, text: "Interrupted" }),
    event(7, "agent.result", { agent: main, subtype: "success", result: "Finished processing" }),
    event(8, "agent.status", { agent: main, status: "compacting" }),
    event(9, "agent.api_retry", { agent: main, attempt: 1 }),
    event(10, "agent.task_notification", { agent: worker("a"), task_id: "task-a", status: "completed" }),
    event(11, "agent.user_message", { agent: main, content: "Session input" }),
  ];
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string) => {
    if (command === "list_runtimes_page") return { runtimes: [runtime("current")], next_cursor: null, total_count: 1 } as T;
    if (command === "list_events") return page(events) as T;
    if (command === "get_event_updates") return { has_updates: false } as T;
    throw new Error(`unexpected command: ${command}`);
  } };
  render(panel());
  await screen.findByText("Main output");
  const toggle = screen.getByRole("switch", { name: "Agent Only" });
  expect(toggle).not.toBeChecked();
  expect(document.querySelectorAll(".event-row")).toHaveLength(8);
  fireEvent.click(toggle);
  expect(toggle).toBeChecked();
  expect([...document.querySelectorAll(".event-row")].map((row) => row.id)).toEqual(["event-seq-1", "event-seq-2", "event-seq-5", "event-seq-6", "event-seq-7"]);
  const roster = screen.getByRole("navigation", { name: "Agent list" });
  const child = within(roster).getByRole("button", { name: /agent-a/ });
  expect(child).toHaveTextContent("Ended");
  fireEvent.click(child);
  expect(screen.getByText("Worker output")).toBeInTheDocument();
  expect(document.querySelectorAll(".event-row")).toHaveLength(1);
  expect(child).toHaveTextContent("1 events");
  fireEvent.click(toggle);
  expect(document.querySelectorAll(".event-row")).toHaveLength(3);
  expect(child).toHaveTextContent("Ended");
});

it("resets pagination when Agent Only is enabled and pages only matching messages", async () => {
  const events = Array.from({ length: 205 }, (_, i) => [
    event(2 * i + 1, "agent.text", { agent: main, text: `Output ${i + 1}` }),
    event(2 * i + 2, "agent.status", { agent: main, status: "compacting" }),
  ]).flat();
  await cacheEvents("server", "subject", events, { syncCursor: "sync", olderCursor: null });
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string) => {
    if (command === "list_runtimes_page") return { runtimes: [runtime("current", "ended", false)], next_cursor: null, total_count: 1 } as T;
    if (command === "get_event_updates") return { has_updates: false } as T;
    throw new Error(`unexpected command: ${command}`);
  } };
  render(panel());
  await screen.findByText("Output 205");
  fireEvent.click(screen.getByRole("button", { name: "Previous page" }));
  fireEvent.click(screen.getByRole("switch", { name: "Agent Only" }));
  expect(document.querySelectorAll(".event-row")).toHaveLength(200);
  expect(screen.getByText("Output 205")).toBeInTheDocument();
  expect(screen.queryByText("Output 1", { exact: true })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Previous page" }));
  expect(document.querySelectorAll(".event-row")).toHaveLength(5);
  expect(screen.getByText("Output 1", { exact: true })).toBeInTheDocument();
});

it("backfills history, separates same-role instances and runtimes, and refreshes lifecycle", async () => {
  const events = [
    event(1, "agent.text", { agent: main, text: "Historical main" }, "old"),
    event(2, "runtime.finished", { text: "Not an agent event" }, "old"),
    spawn(3, "a"),
    event(4, "agent.text", { agent: worker("a"), text: "Ended worker output" }),
    event(5, "agent.tool_result", { agent: main, tool_call_id: "a", spawned_agent: worker("a"), subagent_status: "completed", content: "A completed" }),
    spawn(6, "b"),
    event(7, "agent.text", { agent: worker("b"), text: "Earlier worker B output" }),
    event(8, "agent.task_started", { agent: worker("b"), task_id: "task-b", task_type: "local_agent" }),
    event(9, "agent.tool_result", { agent: main, tool_call_id: "b", spawned_agent: worker("b"), subagent_status: "running", content: "B launched" }),
    event(10, "agent.text", { agent: worker("b"), text: "Later worker B output" }),
    event(11, "runtime.waiting", { text: "Hidden runtime event" }),
    event(12, "agent.text", { agent: worker("b"), text: "Old runtime worker" }, "old"),
  ];
  let updateReady = false;
  const calls: Record<string, unknown>[] = [];
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string, args?: Record<string, unknown>) => {
    const input = args?.input as Record<string, unknown>;
    if (command === "list_runtimes_page") return (input.cursor ? { runtimes: [runtime("old", "ended", false)], next_cursor: null, total_count: 2 } : { runtimes: [runtime("current")], next_cursor: "older-runtimes", total_count: 2 }) as T;
    if (command === "get_event_updates") return { has_updates: updateReady } as T;
    if (command === "list_events") {
      calls.push(input);
      if (input.after) return page([event(13, "agent.task_notification", { agent: worker("b"), task_id: "task-b", status: "completed" })], null, "sync-13") as T;
      return (input.cursor ? page(events.slice(0, 7)) : page(events.slice(7), "older-events")) as T;
    }
    throw new Error(`unexpected command: ${command}`);
  } };
  render(panel());
  const roster = await screen.findByRole("navigation", { name: "Agent list" });
  await waitFor(() => expect(within(roster).getAllByRole("button")).toHaveLength(3));
  await waitFor(() => expect(screen.getByRole("button", { name: "Refresh" })).toBeEnabled());
  const agents = within(roster).getAllByRole("button");
  expect(agents[0]).toHaveTextContent("Main agent");
  expect(agents[1]).toHaveTextContent("agent-b");
  expect(agents[1]).toHaveTextContent("In progress");
  expect(agents[2]).toHaveTextContent("agent-a");
  expect(agents[2]).toHaveTextContent("Ended");
  fireEvent.click(agents[1]);
  expect(screen.getByText("Earlier worker B output")).toBeInTheDocument();
  expect(screen.getByText("Later worker B output")).toBeInTheDocument();
  expect([...document.querySelectorAll(".event-row")].map((row) => row.id)).toEqual(["event-seq-7", "event-seq-8", "event-seq-10"]);
  expect(screen.queryByText("Ended worker output")).not.toBeInTheDocument();
  expect(screen.queryByText("Old runtime worker")).not.toBeInTheDocument();
  expect(calls.some((call) => call.cursor === "older-events")).toBe(true);
  expect(await readEventCacheState("server", "subject")).toMatchObject({ olderCursor: null, eventCount: 12 });
  updateReady = true;
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await waitFor(() => expect(within(roster).getAllByRole("button")[2]).toHaveTextContent("agent-b"));
  expect(within(roster).getAllByRole("button")[2]).toHaveTextContent("Ended");
  expect(within(roster).getAllByRole("button")[2]).toHaveAttribute("aria-pressed", "true");
  updateReady = false;
  fireEvent.click(screen.getByRole("button", { name: "Load more" }));
  await screen.findByRole("option", { name: /^old ·/ });
  fireEvent.change(screen.getByRole("combobox", { name: "Choose Runtime" }), { target: { value: "old" } });
  await screen.findByText("Historical main");
  expect(screen.queryByText("Later worker B output")).not.toBeInTheDocument();
  fireEvent.click(within(roster).getByRole("button", { name: /agent-b/ }));
  expect(screen.getByText("Old runtime worker")).toBeInTheDocument();
  expect(within(roster).getByRole("button", { name: /agent-b/ })).toHaveTextContent("Ended");
});

it("limits rendered messages to 200, preserves order, and pages to earlier messages", async () => {
  const events = Array.from({ length: 205 }, (_, i) => event(i + 1, "agent.text", { agent: main, text: `Message ${i + 1}` }));
  await cacheEvents("server", "subject", events, { syncCursor: "sync", olderCursor: null });
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string) => {
    if (command === "list_runtimes_page") return { runtimes: [runtime("current", "ended", false)], next_cursor: null, total_count: 1 } as T;
    if (command === "get_event_updates") return { has_updates: false } as T;
    throw new Error(`unexpected command: ${command}`);
  } };
  render(panel());
  await screen.findByText("Message 205");
  expect(document.querySelectorAll(".event-row")).toHaveLength(200);
  expect(screen.queryByText("Message 1", { exact: true })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Previous page" }));
  expect(document.querySelectorAll(".event-row")).toHaveLength(5);
  expect(screen.getByText("Message 1", { exact: true })).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Latest" }));
  expect(screen.getByText("Message 205")).toBeInTheDocument();
});

it("ignores a late event response after the Subject changes", async () => {
  let finish: (value: unknown) => void = () => undefined;
  let pending = false;
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string, args?: Record<string, unknown>) => {
    const input = args?.input as Record<string, unknown>;
    if (command === "list_runtimes_page") return { runtimes: [runtime("current", "ended", false)], next_cursor: null, total_count: 1 } as T;
    if (command === "list_events") {
      if (input.subject_id === "subject") { pending = true; return new Promise<unknown>((resolve) => { finish = resolve; }) as Promise<T>; }
      return page([event(2, "agent.text", { agent: main, text: "New subject" })]) as T;
    }
    throw new Error(`unexpected command: ${command}`);
  } };
  const view = render(panel());
  await waitFor(() => expect(pending).toBe(true));
  view.rerender(panel("new-subject"));
  await screen.findByText("New subject");
  await act(async () => finish(page([event(1, "agent.text", { agent: main, text: "Old subject" })])));
  expect(screen.queryByText("Old subject")).not.toBeInTheDocument();
  expect(screen.getByText("New subject")).toBeInTheDocument();
});

it("restarts an invalid cursor and offers retry when history backfill fails", async () => {
  await cacheEvents("server", "subject", [event(1, "agent.text", { agent: main, text: "Stale cache" })], { syncCursor: "invalid", olderCursor: "older" });
  let fail = true;
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string, args?: Record<string, unknown>) => {
    const input = args?.input as Record<string, unknown>;
    if (command === "list_runtimes_page") return { runtimes: [runtime("current", "ended", false)], next_cursor: null, total_count: 1 } as T;
    if (command === "get_event_updates") { if (input.cursor === "invalid") throw { code: "INVALID_CURSOR" }; return { has_updates: false } as T; }
    if (command === "list_events") {
      if (input.cursor && fail) throw new Error("History unavailable");
      return (input.cursor ? page([event(2, "agent.text", { agent: main, text: "Earlier message" })]) : page([event(3, "agent.text", { agent: main, text: "Fresh message" })], "older")) as T;
    }
    throw new Error(`unexpected command: ${command}`);
  } };
  render(panel());
  await screen.findByText("History unavailable");
  expect(screen.queryByText("Stale cache")).not.toBeInTheDocument();
  expect(screen.getByText("Fresh message")).toBeInTheDocument();
  fail = false;
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  await screen.findByText("Earlier message");
  await waitFor(() => expect(screen.queryByText("History unavailable")).not.toBeInTheDocument());
});

it("fetches the final messages when Runtime state changes to ended before the next event poll", async () => {
  let finished = false;
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string, args?: Record<string, unknown>) => {
    const input = args?.input as Record<string, unknown>;
    if (command === "list_runtimes_page") return { runtimes: [runtime("current")], next_cursor: null, total_count: 1 } as T;
    if (command === "get_event_updates") return { has_updates: finished } as T;
    if (command === "list_events") return (input.after
      ? page([event(2, "agent.text", { agent: main, text: "Final message" })], null, "sync-final")
      : page([event(1, "agent.text", { agent: main, text: "Before completion" })])) as T;
    throw new Error(`unexpected command: ${command}`);
  } };
  render(panel());
  await screen.findByText("Before completion");
  await waitFor(() => expect(screen.getByRole("button", { name: "Refresh" })).toBeEnabled());
  finished = true;
  await act(async () => { client.setQueryData(keys.runtimes("server", "subject"), { pages: [{ runtimes: [runtime("current", "ended", false)], next_cursor: null, total_count: 1 }], pageParams: [null] }); });
  await screen.findByText("Final message");
  expect(within(screen.getByRole("navigation", { name: "Agent list" })).getByRole("button")).toHaveTextContent("Ended");
});
