import { normalizeServerUrl } from "../api/commands";
import { REGISTRY_KEY, REGISTRY_VERSION, type RegistryDocument, type RegistryReadResult, type ServerRecord } from "./registry-schema";
import { sessionPasswords } from "./session-passwords";

export const REGISTRY_CHANGED = "well:registry-changed";

function validRecord(value: unknown): value is ServerRecord {
  if (!value || typeof value !== "object") return false;
  const item = value as Partial<ServerRecord>;
  return typeof item.id === "string" && typeof item.name === "string" && item.name.trim() !== "" &&
    typeof item.base_url === "string" && typeof item.remember_password === "boolean" &&
    typeof item.created_at === "string" && typeof item.updated_at === "string" &&
    (item.last_connected_at === null || typeof item.last_connected_at === "string");
}

export function readRegistry(): RegistryReadResult {
  const raw = localStorage.getItem(REGISTRY_KEY);
  if (!raw) return { servers: [], errors: [] };
  try {
    const document = JSON.parse(raw) as Partial<RegistryDocument>;
    if (document.version !== REGISTRY_VERSION || !Array.isArray(document.servers)) {
      return { servers: [], errors: ["unsupported_registry_schema"] };
    }
    const servers = document.servers.filter(validRecord);
    return { servers, errors: servers.length === document.servers.length ? [] : ["corrupt_server_records"] };
  } catch {
    return { servers: [], errors: ["corrupt_registry"] };
  }
}

function writeRegistry(servers: ServerRecord[]): void {
  const document: RegistryDocument = { version: REGISTRY_VERSION, servers };
  localStorage.setItem(REGISTRY_KEY, JSON.stringify(document));
  window.dispatchEvent(new CustomEvent(REGISTRY_CHANGED));
}

export interface SaveServerInput {
  id?: string;
  name: string;
  baseUrl: string;
  password: string;
  rememberPassword: boolean;
}

export interface SaveServerResult {
  server: ServerRecord;
  urlChanged: boolean;
}

export async function saveServer(input: SaveServerInput): Promise<SaveServerResult> {
  const name = input.name.trim();
  if (!name) throw new Error("server_name_required");
  const baseUrl = await normalizeServerUrl(input.baseUrl);
  const current = readRegistry().servers;
  const existing = input.id ? current.find((server) => server.id === input.id) : undefined;
  if (current.some((server) => server.base_url === baseUrl && server.id !== input.id)) throw new Error("duplicate_server_url");
  const now = new Date().toISOString();
  const urlChanged = Boolean(existing && existing.base_url !== baseUrl);
  const server: ServerRecord = {
    id: existing?.id ?? crypto.randomUUID().toLowerCase(),
    name,
    base_url: baseUrl,
    remember_password: input.rememberPassword,
    created_at: existing?.created_at ?? now,
    updated_at: now,
    last_connected_at: urlChanged ? null : (existing?.last_connected_at ?? null),
    ...(urlChanged ? {} : existing?.remote_server_id ? { remote_server_id: existing.remote_server_id } : {}),
    ...(input.rememberPassword && input.password ? { password: input.password } : {}),
  };
  if (input.rememberPassword) sessionPasswords.clear(server.id);
  else if (input.password) sessionPasswords.set(server.id, input.password);
  else if (existing?.remember_password && existing.password) sessionPasswords.set(server.id, existing.password);
  writeRegistry([...current.filter((item) => item.id !== server.id), server]);
  return { server, urlChanged };
}

export function updateServer(id: string, patch: Partial<ServerRecord>): ServerRecord {
  const current = readRegistry().servers;
  const existing = current.find((server) => server.id === id);
  if (!existing) throw new Error("server_not_found");
  const server = { ...existing, ...patch, id, updated_at: new Date().toISOString() };
  writeRegistry(current.map((item) => item.id === id ? server : item));
  return server;
}

export function ensureRemoteIdentityAvailable(id: string, remoteId: string): void {
  const servers = readRegistry().servers;
  if (servers.some((server) => server.id !== id && server.remote_server_id === remoteId)) throw new Error("duplicate_remote_identity");
}

export function bindRemoteIdentity(id: string, remoteId: string): ServerRecord {
  ensureRemoteIdentityAvailable(id, remoteId);
  return updateServer(id, { remote_server_id: remoteId, last_connected_at: new Date().toISOString() });
}

export function confirmRemoteIdentity(id: string, remoteId: string): ServerRecord {
  return bindRemoteIdentity(id, remoteId);
}

export function markServerConnected(id: string): void {
  const current = readRegistry().servers;
  if (!current.some((server) => server.id === id)) return;
  const now = new Date().toISOString();
  writeRegistry(current.map((server) => server.id === id ? { ...server, last_connected_at: now } : server));
}

export function disconnectServer(id: string): void {
  sessionPasswords.clear(id);
  window.dispatchEvent(new CustomEvent(REGISTRY_CHANGED));
}

export function removeServer(id: string): void {
  writeRegistry(readRegistry().servers.filter((server) => server.id !== id));
  sessionPasswords.clear(id);
}

export function passwordFor(server: ServerRecord): string | undefined {
  return server.remember_password ? server.password : sessionPasswords.get(server.id);
}
