import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeAll, expect, it, vi } from "vitest";
import type { ServiceInfo } from "../../api/dto";
import i18n from "../../i18n";
import { SubjectDetailPanel } from "./SubjectDetailPanel";

beforeAll(() => i18n.changeLanguage("en"));

it("loads the current runtime only when the runtime panel is opened", async () => {
  const commands: string[] = [];
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string) => {
    commands.push(command);
    const result = command === "get_subject" ? {
      id: "subject",
      name: "Subject",
      state: "using",
      runtime_profile: { agent: "cc", agent_version: "2.1.199", capabilities: [] },
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
    api_version: "v1",
    runtime_profile: { agent: "cc", agent_version: "2.1.199", capabilities: [] },
    protocol_versions: { norns: null, agent_config: 1 },
    capacity: { concurrency: 1, using: 1, queuing: 0 },
    capabilities: [],
  };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

  render(<QueryClientProvider client={client}><SubjectDetailPanel serverId="server" subjectId="subject" subjectActive={false} connection={{ base_url: "http://server", password: "secret" }} service={service} initialPanel="overview" onPanelChange={vi.fn()} /></QueryClientProvider>);

  const runtimeTab = await screen.findByRole("button", { name: i18n.t("subject.runtime") });
  expect(commands).toContain("get_subject");
  expect(commands).not.toContain("get_current_runtime");
  fireEvent.click(runtimeTab);
  await waitFor(() => expect(commands).toContain("get_current_runtime"));
});
