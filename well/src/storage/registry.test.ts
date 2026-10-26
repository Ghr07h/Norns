import { bindRemoteIdentity, disconnectServer, passwordFor, readRegistry, saveServer } from "./registry";
import { sessionPasswords } from "./session-passwords";
import { REGISTRY_KEY } from "./registry-schema";

beforeEach(() => {
  localStorage.clear();
  sessionPasswords.clearAll();
  window.__WELL_BRIDGE__ = { invoke: async <T,>(_command: string, args?: Record<string, unknown>) => {
    const input = String(args?.input).trim().replace(/\/$/, "").toLowerCase();
    return input as T;
  } };
});

it("keeps stable local identity and clears remote identity when URL changes", async () => {
  const first = await saveServer({ name: "Local", baseUrl: "HTTP://LOCALHOST:28081/", password: "secret", rememberPassword: true });
  bindRemoteIdentity(first.server.id, "550e8400-e29b-41d4-a716-446655440000");
  const changed = await saveServer({ id: first.server.id, name: "Local", baseUrl: "http://localhost:28082", password: "session", rememberPassword: false });
  expect(changed.server.id).toBe(first.server.id);
  expect(changed.server.remote_server_id).toBeUndefined();
  expect(changed.server.last_connected_at).toBeNull();
  expect(passwordFor(changed.server)).toBe("session");
  expect(readRegistry().servers).toHaveLength(1);
});

it("rejects duplicate canonical URLs and remote identities", async () => {
  const first = await saveServer({ name: "One", baseUrl: "http://one.test", password: "x", rememberPassword: true });
  const second = await saveServer({ name: "Two", baseUrl: "http://two.test", password: "x", rememberPassword: true });
  bindRemoteIdentity(first.server.id, "550e8400-e29b-41d4-a716-446655440000");
  expect(() => bindRemoteIdentity(second.server.id, "550e8400-e29b-41d4-a716-446655440000")).toThrow("duplicate_remote_identity");
  await expect(saveServer({ name: "Again", baseUrl: "http://one.test", password: "x", rememberPassword: true })).rejects.toThrow("duplicate_server_url");
});

it("keeps valid records when another stored record is corrupt", async () => {
  const valid = await saveServer({ name: "Valid", baseUrl: "http://valid.test", password: "x", rememberPassword: true });
  localStorage.setItem(REGISTRY_KEY, JSON.stringify({ version: 1, servers: [valid.server, { id: 42 }] }));
  const snapshot = readRegistry();
  expect(snapshot.servers).toEqual([valid.server]);
  expect(snapshot.errors).toEqual(["corrupt_server_records"]);
});

it("stores an unremembered password only for the session and disconnects it", async () => {
  const saved = await saveServer({ name: "Session", baseUrl: "http://session.test", password: "temporary", rememberPassword: false });
  expect(saved.server.password).toBeUndefined();
  expect(passwordFor(saved.server)).toBe("temporary");
  disconnectServer(saved.server.id);
  expect(passwordFor(saved.server)).toBeUndefined();
});
