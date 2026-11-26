import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StrictMode } from "react";
import type { PublicEvent } from "../../api/dto";
import i18n from "../../i18n";
import { clearAllCaches } from "../../storage/event-db";
import { EventsPanel } from "./EventsPanel";

beforeEach(async () => {
  await i18n.changeLanguage("en");
});

afterEach(async () => {
  delete window.__WELL_BRIDGE__;
  await clearAllCaches();
});

function event(seq: number): PublicEvent {
  return { seq: String(seq), type: "agent.text", timestamp: "2026-10-14T00:00:00Z", runtime_id: null, source: "agent", payload: { text: `event ${seq}` } };
}

it("Agent Only defaults off, combines with existing filters and preserves linked tool jumps", async () => {
  const types = ["agent.text", "agent.tool_call", "agent.tool_result", "agent.interrupted", "agent.result", "agent.status", "agent.api_retry", "agent.task_started", "runtime.finished"];
  const events = types.map((type, index) => ({ ...event(index + 1), type, payload: { text: `Output ${index + 1}`, name: "Bash", tool_call_id: "call-a", input: { command: "pwd" }, content: "Tool completed" } }));
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string) => {
    if (command === "list_events") return { events, cursor: "sync-9", next_cursor: null, has_more: false } as T;
    throw new Error(`unexpected command: ${command}`);
  } };
  const scroll = vi.fn();
  const previous = HTMLElement.prototype.scrollIntoView;
  HTMLElement.prototype.scrollIntoView = scroll;
  try {
    render(<EventsPanel serverId="server" subjectId="subject" connection={{ base_url: "http://server", password: "secret" }} runtimeActive={false} />);
    await screen.findByText("Output 1");
    const toggle = screen.getByRole("switch", { name: "Agent Only" });
    expect(toggle).not.toBeChecked();
    expect(document.querySelectorAll(".event-row")).toHaveLength(9);
    fireEvent.click(toggle);
    expect(toggle).toBeChecked();
    expect([...document.querySelectorAll(".event-row")].map((row) => row.id)).toEqual(["event-seq-1", "event-seq-2", "event-seq-3", "event-seq-4", "event-seq-5"]);
    fireEvent.change(screen.getByRole("combobox", { name: "Event type" }), { target: { value: "agent.status" } });
    expect(screen.getByText(/No matching events/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(toggle).not.toBeChecked();
    expect(document.querySelectorAll(".event-row")).toHaveLength(9);
    fireEvent.click(toggle);
    fireEvent.change(screen.getByRole("combobox", { name: "Event type" }), { target: { value: "agent.tool_result" } });
    fireEvent.click(screen.getByRole("button", { name: /View call/ }));
    await waitFor(() => expect(scroll).toHaveBeenCalled());
    expect(toggle).toBeChecked();
    expect(document.querySelectorAll(".event-row")).toHaveLength(5);
    expect(document.querySelector("#event-seq-2")).toHaveClass("event-highlighted");
  } finally {
    HTMLElement.prototype.scrollIntoView = previous;
  }
});

it("caches the initial page but renders only the latest window", async () => {
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string) => {
    if (command === "list_events") return { events: Array.from({ length: 201 }, (_, index) => event(index + 1)), cursor: "sync-201", next_cursor: null, has_more: false } as T;
    if (command === "get_event_updates") return { has_updates: false } as T;
    throw new Error(`unexpected command: ${command}`);
  } };

  render(<StrictMode><EventsPanel serverId="server" subjectId="subject" connection={{ base_url: "http://server", password: "secret" }} runtimeActive={false} /></StrictMode>);

  await screen.findByText("event 201");
  await waitFor(() => expect(document.querySelectorAll(".event-row")).toHaveLength(200));
  expect(screen.queryByText("event 1", { exact: true })).not.toBeInTheDocument();
  expect(screen.getByText("event 2", { exact: true })).toBeInTheDocument();
  expect(screen.getByText("2-201 / 201")).toBeInTheDocument();
});

it("pages backward and forward through cached event windows", async () => {
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string) => {
    if (command === "list_events") return { events: Array.from({ length: 450 }, (_, index) => event(index + 1)), cursor: "sync-450", next_cursor: null, has_more: false } as T;
    if (command === "get_event_updates") return { has_updates: false } as T;
    throw new Error(`unexpected command: ${command}`);
  } };

  render(<EventsPanel serverId="server" subjectId="subject" connection={{ base_url: "http://server", password: "secret" }} runtimeActive={false} />);

  await screen.findByText("event 450");
  expect(screen.getByText("251-450 / 450")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Next page" })).toBeDisabled();
  fireEvent.change(screen.getByRole("textbox", { name: "Search content" }), { target: { value: "event 51" } });
  expect(screen.getByText(/No matching events/)).toBeInTheDocument();

  await waitFor(() => expect(screen.getByRole("button", { name: "Previous page" })).toBeEnabled());
  fireEvent.click(screen.getByRole("button", { name: "Previous page" }));
  await screen.findByText("event 51", { exact: true }, { timeout: 5000 });
  expect(screen.getByText("51-250 / 450")).toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "Search content" })).toHaveValue("event 51");
  fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));

  await waitFor(() => expect(screen.getByRole("button", { name: "Previous page" })).toBeEnabled());
  fireEvent.click(screen.getByRole("button", { name: "Previous page" }));
  await screen.findByText("event 1", { exact: true }, { timeout: 5000 });
  expect(screen.getByText("1-50 / 450")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Previous page" })).toBeDisabled();

  await waitFor(() => expect(screen.getByRole("button", { name: "Next page" })).toBeEnabled());
  fireEvent.click(screen.getByRole("button", { name: "Next page" }));
  await screen.findByText("event 51", { exact: true }, { timeout: 5000 });
  expect(screen.getByText("51-250 / 450")).toBeInTheDocument();
});

it("combines all filter controls, clears them, and resets them on Subject change", async () => {
  const worker = { role: "sub", name: "Worker", tool_use_id: "call-a", instance_id: "tool:call-a" };
  const events: PublicEvent[] = [
    { ...event(1), runtime_id: "runtime-alpha", payload: { agent: worker, text: "Geometry discovery" } },
    { ...event(2), runtime_id: "runtime-alpha", payload: { agent: { ...worker, tool_use_id: "call-b", instance_id: "tool:call-b", agent_id: "actual-b" }, text: "Geometry second" } },
    { ...event(3), runtime_id: "runtime-alpha", type: "agent.tool_result", payload: { agent: { role: "main", name: "Coordinator" }, spawned_agent: { ...worker, agent_id: "actual-a" }, content: "Completed" } },
  ];
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string) => {
    if (command === "list_events") return { events, cursor: "sync-3", next_cursor: null, has_more: false } as T;
    throw new Error(`unexpected command: ${command}`);
  } };
  const props = { serverId: "server", subjectId: "subject", connection: { base_url: "http://server", password: "secret" }, runtimeActive: false };
  const view = render(<EventsPanel {...props} />);
  await screen.findByText("Geometry discovery");
  fireEvent.change(screen.getByRole("combobox", { name: "Runtime ID" }), { target: { value: "alpha" } });
  fireEvent.change(screen.getByRole("combobox", { name: "Event type" }), { target: { value: "agent.text" } });
  fireEvent.change(screen.getByRole("combobox", { name: "Source agent role" }), { target: { value: 'name:["sub","Worker"]' } });
  fireEvent.change(screen.getByRole("combobox", { name: "Source agent ID" }), { target: { value: "actual-a" } });
  fireEvent.change(screen.getByRole("textbox", { name: "Search content" }), { target: { value: "geometry" } });
  expect(document.querySelectorAll(".event-row")).toHaveLength(1);
  expect(screen.getByText("Geometry discovery")).toBeInTheDocument();
  expect(screen.queryByText("Geometry second")).not.toBeInTheDocument();
  fireEvent.change(screen.getByRole("textbox", { name: "Search content" }), { target: { value: "missing" } });
  expect(screen.getByText(/No matching events/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
  expect(document.querySelectorAll(".event-row")).toHaveLength(3);
  fireEvent.change(screen.getByRole("combobox", { name: "Runtime ID" }), { target: { value: "missing" } });
  view.rerender(<EventsPanel {...props} subjectId="another-subject" />);
  await screen.findByText("Geometry discovery");
  expect(screen.getByRole("combobox", { name: "Runtime ID" })).toHaveValue("");
});

it("clears filters when jumping to a linked tool event", async () => {
  const scroll = vi.fn();
  const previous = HTMLElement.prototype.scrollIntoView;
  HTMLElement.prototype.scrollIntoView = scroll;
  try {
    const events: PublicEvent[] = [
      { ...event(1), type: "agent.tool_call", payload: { name: "Bash", tool_call_id: "call-a", input: { command: "pwd" } } },
      { ...event(2), type: "agent.tool_result", payload: { tool_call_id: "call-a", content: "Completed" } },
    ];
    window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string) => {
      if (command === "list_events") return { events, cursor: "sync-2", next_cursor: null, has_more: false } as T;
      throw new Error(`unexpected command: ${command}`);
    } };
    render(<EventsPanel serverId="server" subjectId="subject" connection={{ base_url: "http://server", password: "secret" }} runtimeActive={false} />);
    await screen.findByText("Completed");
    fireEvent.change(screen.getByRole("combobox", { name: "Event type" }), { target: { value: "agent.tool_result" } });
    expect(document.querySelectorAll(".event-row")).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: /View call/ }));
    await waitFor(() => expect(scroll).toHaveBeenCalled());
    expect(screen.getByRole("combobox", { name: "Event type" })).toHaveValue("");
    expect(document.querySelectorAll(".event-row")).toHaveLength(2);
    expect(document.querySelector("#event-seq-1")).toHaveClass("event-highlighted");
  } finally {
    HTMLElement.prototype.scrollIntoView = previous;
  }
});
