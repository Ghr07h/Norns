import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeAll, expect, it, vi } from "vitest";
import type { ServiceInfo } from "../../api/dto";
import i18n from "../../i18n";
import { SubjectDetailPanel } from "./SubjectDetailPanel";
import { supportedApiSchema } from "../../lib/versions";

beforeAll(() => i18n.changeLanguage("en"));

it("loads the current runtime only when the runtime panel is opened", async () => {
  const commands: string[] = [];
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string) => {
    commands.push(command);
    const result = command === "get_subject" ? {
      id: "subject",
      name: "Subject",
      state: "using",
      workspace: { materialized: true, directory_name: "subject" },
      last_event_seq: 0,
      created_at: "2026-10-16T00:00:00Z",
      updated_at: "2026-10-16T00:00:01Z",
    } : command === "get_current_runtime" ? { runtime: null } : null;
    return result as T;
  } };
  const service: ServiceInfo = {
    service: "nornsd",
    version: "dev",
    api_schema: supportedApiSchema,
    protocol_versions: { norns: null, agent_config: 1 },
    capacity: { concurrency: 1, using: 1, queuing: 0 },
    capabilities: [],
  };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  render(<QueryClientProvider client={client}><SubjectDetailPanel serverId="server" subjectId="subject" subjectActive={false} connection={{ base_url: "http://server", password: "secret" }} service={service} initialPanel="overview" onPanelChange={vi.fn()} /></QueryClientProvider>);

  const runtimeTab = await screen.findByRole("button", { name: i18n.t("subject.runtime") });
  expect(within(screen.getByRole("navigation", { name: "Subject panels" })).getAllByRole("button").map((tab) => tab.textContent)).toEqual(["Subject overview", "Runtime", "Agent messages", "Event records", "Norns information"]);
  expect(commands).toContain("get_subject");
  expect(commands).not.toContain("get_current_runtime");
  const nornsTab = screen.getByRole("button", { name: "Norns information" });
  expect(nornsTab).toBeEnabled();
  fireEvent.click(nornsTab);
  expect(screen.getByRole("button", { name: "Generate" })).toBeEnabled();
  fireEvent.click(runtimeTab);
  await waitFor(() => expect(commands).toContain("get_current_runtime"));
});
