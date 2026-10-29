import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
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

  fireEvent.click(screen.getByRole("button", { name: "Previous page" }));
  await screen.findByText("event 51", { exact: true });
  expect(screen.getByText("51-250 / 450")).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", { name: "Previous page" }));
  await screen.findByText("event 1", { exact: true });
  expect(screen.getByText("1-50 / 450")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Previous page" })).toBeDisabled();

  fireEvent.click(screen.getByRole("button", { name: "Next page" }));
  await screen.findByText("event 51", { exact: true });
  expect(screen.getByText("51-250 / 450")).toBeInTheDocument();
});
