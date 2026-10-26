import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { StrictMode } from "react";
import type { PublicEvent } from "../../api/dto";
import { clearAllCaches } from "../../storage/event-db";
import { EventsPanel } from "./EventsPanel";

afterEach(async () => {
  delete window.__WELL_BRIDGE__;
  await clearAllCaches();
});

function event(seq: number): PublicEvent {
  return { seq: String(seq), type: "agent.text", timestamp: "2026-10-14T00:00:00Z", runtime_id: null, source: "agent", payload: { text: `event ${seq}` } };
}

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
});
