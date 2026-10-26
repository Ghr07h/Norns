const KEY = "well.preferences.v1";

export interface ServerPreferences {
  selectedSubjectId?: string;
  search?: string;
  stateFilter?: string;
  sort?: string;
  activePanel?: string;
}

type Preferences = Record<string, ServerPreferences>;

function read(): Preferences {
  try {
    return JSON.parse(localStorage.getItem(KEY) ?? "{}") as Preferences;
  } catch {
    return {};
  }
}

export function getServerPreferences(serverId: string): ServerPreferences {
  return read()[serverId] ?? {};
}

export function setServerPreferences(serverId: string, patch: Partial<ServerPreferences>): void {
  const all = read();
  all[serverId] = { ...all[serverId], ...patch };
  localStorage.setItem(KEY, JSON.stringify(all));
}

export function removeServerPreferences(serverId: string): void {
  const all = read();
  delete all[serverId];
  localStorage.setItem(KEY, JSON.stringify(all));
}
