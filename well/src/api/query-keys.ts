export const keys = {
  probe: (serverId: string, revision?: string) => ["server", serverId, "probe", ...(revision ? [revision] : [])] as const,
  subjects: (serverId: string) => ["server", serverId, "subjects"] as const,
  queue: (serverId: string) => ["server", serverId, "queue"] as const,
  subject: (serverId: string, subjectId: string) => ["server", serverId, "subject", subjectId] as const,
  currentRuntime: (serverId: string, subjectId: string) => ["server", serverId, "subject", subjectId, "runtime"] as const,
  events: (serverId: string, subjectId: string) => ["server", serverId, "subject", subjectId, "events"] as const,
  runtimes: (serverId: string, subjectId: string) => ["server", serverId, "subject", subjectId, "runtimes"] as const,
  runtime: (serverId: string, subjectId: string, runtimeId: string) => ["server", serverId, "subject", subjectId, "runtime", runtimeId] as const,
  norns: (serverId: string, subjectId: string, kind: string, parameter = "") => ["server", serverId, "subject", subjectId, "norns", kind, parameter] as const,
};
