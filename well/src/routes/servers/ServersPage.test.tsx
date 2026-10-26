import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeAll, beforeEach, expect, it } from "vitest";
import type { ClientError, ProbeSnapshot } from "../../api/dto";
import { keys } from "../../api/query-keys";
import i18n from "../../i18n";
import { cacheEvents, clearAllCaches, readLatestCachedEvents } from "../../storage/event-db";
import { bindRemoteIdentity, readRegistry, saveServer } from "../../storage/registry";
import { sessionPasswords } from "../../storage/session-passwords";
import { ServersPage } from "./ServersPage";

const OLD_ID = "550e8400-e29b-41d4-a716-446655440000";
const NEW_ID = "38b90b26-86d8-4149-a88f-91a44d36e79f";

function snapshot(serverId: string): ProbeSnapshot {
  return {
    service: {
      service: "nornsd",
      server_id: serverId,
      version: "dev",
      api_version: "v1",
      runtime_profile: { agent: "cc", agent_version: "1", capabilities: [] },
      protocol_versions: { norns: null, agent_config: 1 },
      capacity: { concurrency: 1, using: 0, queuing: 0 },
      capabilities: [],
    },
    health: { status: "ok" },
  };
}

function renderPage(client = new QueryClient({ defaultOptions: { queries: { retry: false } } })) {
  return render(<MemoryRouter><QueryClientProvider client={client}><ServersPage /></QueryClientProvider></MemoryRouter>);
}

beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function showModal() { this.setAttribute("open", ""); };
  HTMLDialogElement.prototype.close = function close() { this.removeAttribute("open"); };
});

beforeEach(async () => {
  await i18n.changeLanguage("zh");
  localStorage.clear();
  sessionPasswords.clearAll();
  await clearAllCaches();
});

it("clears local state and rebinds a changed target service", async () => {
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string, args?: Record<string, unknown>) => {
    if (command === "normalize_server_url") return String(args?.input) as T;
    if (command === "probe_server") return snapshot(NEW_ID) as T;
    throw new Error(`unexpected command: ${command}`);
  } };
  const saved = await saveServer({ name: "Changed", baseUrl: "http://changed.test", password: "secret", rememberPassword: true });
  bindRemoteIdentity(saved.server.id, OLD_ID);

  renderPage();
  fireEvent.click(await screen.findByRole("button", { name: "清除并重新绑定" }));
  const dialog = screen.getByRole("dialog");
  expect(within(dialog).getByRole("heading", { name: "目标服务已改变" })).toBeInTheDocument();
  fireEvent.click(within(dialog).getByRole("button", { name: "清除并重新绑定" }));

  await waitFor(() => expect(readRegistry().servers.find((server) => server.id === saved.server.id)?.remote_server_id).toBe(NEW_ID));
  expect(await screen.findByRole("status")).toHaveTextContent("已清除 Changed 的本地数据并绑定到新目标服务。");
});

it("shows why rebinding failed when another record already uses the target", async () => {
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string, args?: Record<string, unknown>) => {
    if (command === "normalize_server_url") return String(args?.input) as T;
    if (command === "probe_server") return snapshot(NEW_ID) as T;
    throw new Error(`unexpected command: ${command}`);
  } };
  const changed = await saveServer({ name: "Changed", baseUrl: "http://changed.test", password: "secret", rememberPassword: true });
  const existing = await saveServer({ name: "Existing", baseUrl: "http://existing.test", password: "secret", rememberPassword: true });
  bindRemoteIdentity(changed.server.id, OLD_ID);
  bindRemoteIdentity(existing.server.id, NEW_ID);
  await cacheEvents(changed.server.id, "subject", [{ seq: "1", type: "test", timestamp: "2026-10-22T00:00:00Z", runtime_id: null, source: "test", payload: null }]);

  renderPage();
  fireEvent.click(await screen.findByRole("button", { name: "清除并重新绑定" }));
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "清除并重新绑定" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("该目标服务已被另一个 Server 使用。");
  expect(readRegistry().servers.find((server) => server.id === changed.server.id)?.remote_server_id).toBe(OLD_ID);
  expect(await readLatestCachedEvents(changed.server.id, "subject", 10)).toHaveLength(1);
});

it("hides cached capacity when a server probe becomes offline", async () => {
  const offlineError: ClientError = {
    kind: "api",
    httpStatus: 503,
    code: null,
    message: "service unavailable",
    requestId: null,
    details: {},
  };
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string, args?: Record<string, unknown>) => {
    if (command === "normalize_server_url") return String(args?.input) as T;
    if (command === "probe_server") throw offlineError;
    throw new Error(`unexpected command: ${command}`);
  } };
  const saved = await saveServer({ name: "Offline", baseUrl: "http://offline.test", password: "secret", rememberPassword: true });
  const cached = snapshot(OLD_ID);
  cached.service.capacity = { concurrency: 7, using: 3, queuing: 5 };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(keys.probe(saved.server.id, saved.server.updated_at), cached);

  const view = renderPage(client);

  expect(await screen.findByText("离线")).toBeInTheDocument();
  const capacity = view.container.querySelector(".server-capacity");
  expect(capacity).toHaveTextContent("-");
  expect(capacity).not.toHaveTextContent("3/7");
  expect(capacity).not.toHaveTextContent("5");
});
