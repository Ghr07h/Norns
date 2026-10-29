import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { getCurrentRuntime, getSubject, listQueuePage, listSubjectsPage, probeServer } from "./commands";
import { collectCursorPages } from "./pagination";
import { keys } from "./query-keys";
import type { ClientError, ConnectionInput, ProbeSnapshot, QueueEntry, SubjectSummary } from "./dto";
import type { ServerRecord } from "../storage/registry-schema";
import { passwordFor, readRegistry } from "../storage/registry";
import { useAppFocused, usePollingInterval } from "../polling/focus";

const probeWaiters: Array<() => void> = [];
let activeProbes = 0;
async function withProbePermit<T>(operation: () => Promise<T>): Promise<T> {
  if (activeProbes >= 4) await new Promise<void>((resolve) => probeWaiters.push(resolve));
  activeProbes += 1;
  try { return await operation(); }
  finally { activeProbes -= 1; probeWaiters.shift()?.(); }
}

export type ConnectionState = "unknown" | "checking" | "retrying" | "online" | "offline" | "unauthorized" | "incompatible" | "identity_changed";

export interface ProbeResult {
  state: ConnectionState;
  snapshot?: ProbeSnapshot;
  currentRemoteId?: string;
  duplicateRoot?: boolean;
}

export function connectionFor(server: ServerRecord): ConnectionInput | null {
  const password = passwordFor(server);
  return password === undefined ? null : { base_url: server.base_url, password };
}

function probeState(server: ServerRecord, snapshot: ProbeSnapshot): ProbeResult {
  const remoteId = snapshot.service.server_id;
  if (server.remote_server_id && remoteId && server.remote_server_id !== remoteId) return { state: "identity_changed", snapshot, currentRemoteId: remoteId };
  const duplicateRoot = Boolean(remoteId && readRegistry().servers.some((item) => item.id !== server.id && item.remote_server_id === remoteId));
  return { state: "online", snapshot, currentRemoteId: remoteId, duplicateRoot };
}

function classifyError(error: ClientError | null): ConnectionState {
  if (!error) return "offline";
  if (error.kind === "authentication" || error.httpStatus === 401) return "unauthorized";
  if (error.kind === "protocol") return "incompatible";
  return "offline";
}

export function retryServerProbe(failureCount: number, error: ClientError): boolean {
  return failureCount < 2 && (error.kind === "connection" || error.kind === "timeout");
}

export function useServerProbe(server: ServerRecord | undefined, enabled = true) {
  const connection = server ? connectionFor(server) : null;
  const interval = usePollingInterval(enabled);
  const query = useQuery({
    queryKey: keys.probe(server?.id ?? "", server?.updated_at),
    queryFn: () => withProbePermit(() => probeServer(connection!)),
    enabled: enabled && server !== undefined && connection !== null,
    retry: retryServerProbe,
    retryDelay: 1_000,
    refetchInterval: (current) => current.state.status === "success" ? interval : false,
    refetchOnMount: "always",
    meta: { serverProbe: true },
  });
  const result: ProbeResult = !connection ? { state: "unknown" }
    : query.isFetching && query.failureCount > 0 ? { state: "retrying", snapshot: query.data }
    : query.isFetching && query.status !== "success" ? { state: "checking", snapshot: query.data }
    : query.isPending ? { state: "checking" }
    : query.error ? { state: classifyError(query.error as unknown as ClientError), snapshot: query.data }
    : query.data && server ? probeState(server, query.data)
    : { state: "offline" };
  return { ...query, connection, connectionResult: result };
}

export function useSubjects(serverId: string, connection: ConnectionInput | null, enabled = true, active = false) {
  const focused = useAppFocused();
  const wasFocused = useRef(focused);
  const interval = usePollingInterval(enabled, active ? 10_000 : 30_000);
  const query = useQuery<SubjectSummary[]>({
    queryKey: keys.subjects(serverId),
    queryFn: () => collectCursorPages(async (cursor) => {
      const page = await listSubjectsPage(connection!, cursor);
      return { items: page.subjects, nextCursor: page.next_cursor };
    }),
    enabled: enabled && connection !== null,
    refetchInterval: interval,
    placeholderData: keepPreviousData,
  });
  useEffect(() => {
    if (enabled && focused && !wasFocused.current) void query.refetch();
    wasFocused.current = focused;
  }, [enabled, focused, query.refetch]);
  return query;
}

export function useQueue(serverId: string, connection: ConnectionInput | null, enabled = true) {
  const interval = usePollingInterval(enabled);
  return useQuery<QueueEntry[]>({
    queryKey: keys.queue(serverId),
    queryFn: () => collectCursorPages(async (cursor) => {
      const page = await listQueuePage(connection!, cursor);
      return { items: page.runtimes, nextCursor: page.next_cursor };
    }),
    enabled: enabled && connection !== null,
    refetchInterval: interval,
    placeholderData: keepPreviousData,
  });
}

export function useSubject(serverId: string, subjectId: string | null, connection: ConnectionInput | null) {
  return useQuery({ queryKey: keys.subject(serverId, subjectId ?? ""), queryFn: () => getSubject(connection!, subjectId!), enabled: Boolean(connection && subjectId) });
}

export function useCurrentRuntime(serverId: string, subjectId: string | null, connection: ConnectionInput | null, enabled = true) {
  const active = Boolean(connection && subjectId && enabled);
  const interval = usePollingInterval(active);
  return useQuery({ queryKey: keys.currentRuntime(serverId, subjectId ?? ""), queryFn: () => getCurrentRuntime(connection!, subjectId!), enabled: active, refetchInterval: interval });
}
